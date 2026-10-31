package invoice_test

import (
	"context"
	"encoding/base64"
	"log/slog"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"hotline-example/aws-sqs-invoices/internal/fakes3"
	"hotline-example/aws-sqs-invoices/internal/fakesqs"
	"hotline-example/aws-sqs-invoices/invoice"
)

const bucket = "shop-invoices"

var _ = Describe("Keeping invoice PDFs in S3", func() {
	var (
		fake  *fakes3.S3
		files invoice.Files
		ctx   context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		fake = fakes3.Start(bucket, nil)
		client, err := invoice.NewFilesClient(ctx, invoice.Settings{ProxyURL: "http://invoices:x@" + fake.Addr}, testAWS()...)
		Expect(err).NotTo(HaveOccurred())
		files = invoice.Files{Client: client, Bucket: bucket}
	})

	Describe("the S3 client set up for Hotline", func() {
		It("sends to the forward proxy over http, with the bucket in the host name and the integration ID added after signing", func() {
			Expect(files.Store(ctx, "A-1001", []byte("%PDF-1.4 a"))).To(Succeed())

			c := fake.Calls()[0]
			Expect(c.Method).To(Equal("PUT"))
			Expect(c.RequestURI).To(HavePrefix("http://shop-invoices.s3.eu-west-1.amazonaws.com/A-1001.pdf?"))
			Expect(c.RequestURI).To(ContainSubstring("x-id=PutObject"))
			Expect(c.ProxyAuth).To(Equal("Basic " + base64.StdEncoding.EncodeToString([]byte("invoices:x"))))
			Expect(c.Authorization).To(HavePrefix("AWS4-HMAC-SHA256 "))
			Expect(c.ContentType).To(Equal("application/pdf"))
		})

		It("goes straight to an endpoint when no proxy is set, as for a local S3, with the bucket in the path", func() {
			client, err := invoice.NewFilesClient(ctx, invoice.Settings{Endpoint: "http://" + fake.Addr}, testAWS()...)
			Expect(err).NotTo(HaveOccurred())
			direct := invoice.Files{Client: client, Bucket: bucket}

			Expect(direct.Store(ctx, "A-1001", []byte("%PDF-1.4 a"))).To(Succeed())
			Expect(fake.Calls()[0].RequestURI).To(HavePrefix("/shop-invoices/A-1001.pdf"))
			Expect(fake.Calls()[0].ProxyAuth).To(BeEmpty())
		})

		It("refuses a proxy address it can't read", func() {
			_, err := invoice.NewFilesClient(ctx, invoice.Settings{ProxyURL: "://nope"}, testAWS()...)
			Expect(err).To(MatchError(ContainSubstring("proxy")))
		})
	})

	It("stores a PDF under its order and fetches it back", func() {
		Expect(files.Store(ctx, "A-1001", []byte("%PDF-1.4 invoice"))).To(Succeed())
		Expect(fake.File("A-1001.pdf")).To(Equal([]byte("%PDF-1.4 invoice")))

		pdf, err := files.Fetch(ctx, "A-1001")
		Expect(err).NotTo(HaveOccurred())
		Expect(pdf).To(Equal([]byte("%PDF-1.4 invoice")))
	})

	It("says when an invoice isn't there", func() {
		_, err := files.Fetch(ctx, "A-9999")
		Expect(err).To(MatchError(invoice.ErrNoInvoice))
	})

	It("lists the stored invoices", func() {
		fake.Put("A-1001.pdf", []byte("12345"), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
		fake.Put("A-1002.pdf", []byte("123"), time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))

		list, err := files.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(list).To(Equal([]invoice.File{
			{Key: "A-1001.pdf", Size: 5, Modified: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
			{Key: "A-1002.pdf", Size: 3, Modified: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)},
		}))
	})

	It("deletes the invoices stored before a day, in one call", func() {
		fake.Put("A-1001.pdf", []byte("a"), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
		fake.Put("A-1002.pdf", []byte("b"), time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC))
		fake.Put("A-1003.pdf", []byte("c"), time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC))

		deleted, err := files.Cleanup(ctx, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
		Expect(err).NotTo(HaveOccurred())
		Expect(deleted).To(Equal([]string{"A-1001.pdf", "A-1002.pdf"}))
		Expect(fake.Keys()).To(Equal([]string{"A-1003.pdf"}))

		var deletes int
		for _, c := range fake.Calls() {
			if c.Method == http.MethodPost {
				deletes++
				Expect(c.ContentType).To(Equal("application/xml"))
			}
		}
		Expect(deletes).To(Equal(1))
	})

	It("deletes nothing, and makes no delete call, when nothing is old enough", func() {
		fake.Put("A-1003.pdf", []byte("c"), time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC))

		deleted, err := files.Cleanup(ctx, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
		Expect(err).NotTo(HaveOccurred())
		Expect(deleted).To(BeEmpty())
		Expect(fake.Calls()).To(HaveLen(1))
	})

	It("sends a PDF in AWS's chunks, with its checksum after the body", func() {
		Expect(files.Store(ctx, "A-1001", []byte("%PDF-1.4 invoice"))).To(Succeed())
		Expect(fake.Calls()[0].Chunked).To(BeTrue())
		Expect(fake.Calls()[0].ChecksumOK).To(BeTrue())
	})
})

var _ = Describe("The worker with a bucket", func() {
	It("stores the invoice PDF in the bucket instead of a folder", func() {
		ctx := context.Background()
		sqsFake := fakesqs.Start()
		fake := fakes3.Start(bucket, sqsFake)
		settings := invoice.Settings{ProxyURL: "http://invoices:x@" + fake.Addr}
		queue, err := invoice.NewClient(ctx, settings, testAWS()...)
		Expect(err).NotTo(HaveOccurred())
		client, err := invoice.NewFilesClient(ctx, settings, testAWS()...)
		Expect(err).NotTo(HaveOccurred())
		_, _ = invoice.Send(ctx, queue, queueURL, jobBody("A-1001", "invoice-standard-v2"))

		worker := &invoice.Worker{
			Client: queue, QueueURL: queueURL,
			Files: &invoice.Files{Client: client, Bucket: bucket},
			Log:   slog.New(slog.DiscardHandler),
		}
		Expect(worker.Run(ctx, true)).To(Succeed())

		Expect(fake.File("A-1001.pdf")).To(HavePrefix("%PDF-1.4"))
		Expect(sqsFake.Waiting()).To(Equal(0))
	})

	It("leaves the message on the queue when the PDF can't be stored", func() {
		ctx := context.Background()
		sqsFake := fakesqs.Start()
		fake := fakes3.Start("another-bucket", sqsFake)
		settings := invoice.Settings{ProxyURL: "http://invoices:x@" + fake.Addr}
		queue, _ := invoice.NewClient(ctx, settings, testAWS()...)
		client, _ := invoice.NewFilesClient(ctx, settings, testAWS()...)
		_, _ = invoice.Send(ctx, queue, queueURL, jobBody("A-1001", "invoice-standard-v2"))

		worker := &invoice.Worker{
			Client: queue, QueueURL: queueURL,
			Files: &invoice.Files{Client: client, Bucket: bucket},
			Log:   slog.New(slog.DiscardHandler),
		}
		Expect(worker.Run(ctx, true)).To(MatchError(ContainSubstring("A-1001.pdf")))
		Expect(sqsFake.Waiting()).To(Equal(1))
	})
})
