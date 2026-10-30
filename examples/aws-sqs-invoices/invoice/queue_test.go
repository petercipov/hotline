package invoice_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"hotline-example/aws-sqs-invoices/internal/fakesqs"
	"hotline-example/aws-sqs-invoices/invoice"
)

const queueURL = "https://sqs.eu-west-1.amazonaws.com/192325781350/invoices"

func testAWS() []func(*config.LoadOptions) error {
	return []func(*config.LoadOptions) error{
		config.WithRegion("eu-west-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("AKIDEXAMPLE", "secret", "")),
	}
}

func jobBody(order, template string) string {
	b, err := json.Marshal(invoice.NewJob(order, template, "en", "", time.Now()))
	Expect(err).NotTo(HaveOccurred())
	return string(b)
}

var _ = Describe("Sending and receiving invoice jobs over SQS", func() {
	var (
		fake   *fakesqs.SQS
		client *sqs.Client
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		fake = fakesqs.Start()
		var err error
		client, err = invoice.NewClient(ctx, invoice.Settings{ProxyURL: "http://invoices:x@" + fake.Addr}, testAWS()...)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("the SQS client set up for Hotline", func() {
		It("sends to the forward proxy over http, with the integration ID added after signing", func() {
			id, err := invoice.Send(ctx, client, queueURL, jobBody("A-1001", "invoice-standard-v2"))
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal("m-1"))

			c := fake.Calls()[0]
			Expect(c.RequestURI).To(Equal("http://sqs.eu-west-1.amazonaws.com/"))
			Expect(c.Target).To(Equal("AmazonSQS.SendMessage"))
			Expect(c.ProxyAuth).To(Equal("Basic " + base64.StdEncoding.EncodeToString([]byte("invoices:x"))))
			Expect(c.Authorization).To(HavePrefix("AWS4-HMAC-SHA256 "))
			Expect(c.Authorization).NotTo(ContainSubstring("proxy-authorization"))
			Expect(c.Body["QueueUrl"]).To(Equal(queueURL))
		})

		It("goes straight to an endpoint when no proxy is set, as for a local SQS", func() {
			direct, err := invoice.NewClient(ctx, invoice.Settings{Endpoint: "http://" + fake.Addr}, testAWS()...)
			Expect(err).NotTo(HaveOccurred())

			_, err = invoice.Send(ctx, direct, queueURL, "hello")
			Expect(err).NotTo(HaveOccurred())
			Expect(fake.Calls()[0].RequestURI).To(Equal("/"))
			Expect(fake.Calls()[0].ProxyAuth).To(BeEmpty())
		})

		It("refuses a proxy address it can't read", func() {
			_, err := invoice.NewClient(ctx, invoice.Settings{ProxyURL: "://nope"}, testAWS()...)
			Expect(err).To(MatchError(ContainSubstring("proxy")))
		})
	})

	Describe("the worker", func() {
		var (
			out    string
			logs   *bytes.Buffer
			worker *invoice.Worker
		)

		BeforeEach(func() {
			out = GinkgoT().TempDir()
			logs = &bytes.Buffer{}
			worker = &invoice.Worker{
				Client:   client,
				QueueURL: queueURL,
				OutDir:   out,
				Log:      slog.New(slog.NewJSONHandler(logs, nil)),
			}
		})

		It("writes the invoice PDF and deletes the message", func() {
			_, _ = invoice.Send(ctx, client, queueURL, jobBody("A-1001", "invoice-standard-v2"))

			Expect(worker.Run(ctx, true)).To(Succeed())

			pdf, err := os.ReadFile(filepath.Join(out, "invoice-A-1001.pdf")) //nolint:gosec // the test chooses its own files
			Expect(err).NotTo(HaveOccurred())
			Expect(pdf).To(HavePrefix("%PDF-1.4"))
			Expect(fake.Waiting()).To(Equal(0))

			var targets []string
			for _, c := range fake.Calls() {
				targets = append(targets, c.Target)
			}
			Expect(targets).To(ContainElements("AmazonSQS.ReceiveMessage", "AmazonSQS.DeleteMessage"))
			Expect(fake.Calls()[1].RequestURI).To(Equal("http://sqs.eu-west-1.amazonaws.com/"))
		})

		It("leaves a broken message on the queue, so SQS moves it to the dead letter queue", func() {
			_, _ = invoice.Send(ctx, client, queueURL, `{"schemaVersion":1}`)

			Expect(worker.Run(ctx, true)).To(Succeed())

			Expect(fake.Waiting()).To(Equal(1))
			Expect(logs.String()).To(ContainSubstring("message refused"))
			Expect(logs.String()).To(ContainSubstring("$.orderId required"))
			entries, _ := os.ReadDir(out)
			Expect(entries).To(BeEmpty())
		})

		It("leaves a job for a template it doesn't have on the queue", func() {
			_, _ = invoice.Send(ctx, client, queueURL, jobBody("A-1001", "invoice-fancy-v9"))

			Expect(worker.Run(ctx, true)).To(Succeed())

			Expect(fake.Waiting()).To(Equal(1))
			Expect(logs.String()).To(ContainSubstring("unknown template"))
		})

		It("handles an event only once when SQS delivers it twice", func() {
			body := jobBody("A-1002", "invoice-compact-v1")
			_, _ = invoice.Send(ctx, client, queueURL, body)
			_, _ = invoice.Send(ctx, client, queueURL, body)

			Expect(worker.Run(ctx, true)).To(Succeed())

			Expect(fake.Waiting()).To(Equal(0))
			Expect(logs.String()).To(ContainSubstring("already handled"))
		})

		It("stops when asked, while waiting for messages", func() {
			stop, cancel := context.WithCancel(ctx)
			cancel()
			Expect(worker.Run(stop, false)).To(Succeed())
		})

		It("creates a missing output folder", func() {
			worker.OutDir = filepath.Join(out, "missing", "folder")
			_, _ = invoice.Send(ctx, client, queueURL, jobBody("A-1001", "invoice-standard-v2"))

			Expect(worker.Run(ctx, true)).To(Succeed())
			Expect(filepath.Join(out, "missing", "folder", "invoice-A-1001.pdf")).To(BeAnExistingFile())
		})

		It("reports an output folder it can't write to", func() {
			blocker := filepath.Join(out, "a-file")
			Expect(os.WriteFile(blocker, nil, 0o600)).To(Succeed())
			worker.OutDir = filepath.Join(blocker, "folder")
			_, _ = invoice.Send(ctx, client, queueURL, jobBody("A-1001", "invoice-standard-v2"))

			Expect(worker.Run(ctx, true)).To(MatchError(ContainSubstring("invoice-A-1001.pdf")))
			Expect(fake.Waiting()).To(Equal(1))
		})
	})
})
