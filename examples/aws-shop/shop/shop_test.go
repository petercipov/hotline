package shop_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"hotline-example/aws-shop/internal/fakeaws"
	"hotline-example/aws-shop/internal/fakes3"
	"hotline-example/aws-shop/internal/fakesqs"
	"hotline-example/aws-shop/invoice"
	"hotline-example/aws-shop/shop"
)

func testAWS() []func(*config.LoadOptions) error {
	return []func(*config.LoadOptions) error{
		config.WithRegion("eu-west-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("AKIDEXAMPLE", "secret", "")),
	}
}

var _ = Describe("The shop's four apps", func() {
	var (
		ctx   context.Context
		sqs   *fakesqs.SQS
		aws   *fakeaws.AWS
		files *fakes3.S3
		names shop.Names
		now   time.Time
		logs  *bytes.Buffer
	)

	BeforeEach(func() {
		ctx = context.Background()
		sqs = fakesqs.Start()
		aws = fakeaws.New(sqs)
		files = fakes3.Start("shop-invoices", aws)
		names = shop.DefaultNames()
		names.Bucket = "shop-invoices"
		names.LogGroup = "" // the log group tests turn it on
		aws.Secrets[names.PaymentSecret] = "pk_test_123"
		aws.Parameters[names.TemplateParameter] = "invoice-compact-v1"
		now = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
		logs = &bytes.Buffer{}
	})

	// app gives the clients one app uses, through the fake proxy, with the
	// app's name as the integration ID.
	app := func(name string) shop.App {
		a, err := shop.NewApp(ctx, name, shop.Settings{Proxy: "http://" + files.Addr}, names, testAWS()...)
		Expect(err).NotTo(HaveOccurred())
		a.Log = slog.New(slog.NewJSONHandler(logs, nil))
		return a
	}

	pay := func() {
		_, err := app("checkout").Pay(ctx, shop.Payment{OrderID: "A-1001", Language: "en", Template: "invoice-standard-v2", SendTo: "jana@example.com"}, now)
		Expect(err).NotTo(HaveOccurred())
	}

	Describe("checkout", func() {
		It("reads the payment key, saves the order, announces it, and asks for the invoice", func() {
			pay()
			Expect(aws.Targets()).To(Equal([]string{
				"secretsmanager.GetSecretValue",
				"DynamoDB_20120810.PutItem",
				"Kinesis_20131202.PutRecord",
			}))
			Expect(aws.Item("orders", "A-1001")).To(Equal(map[string]any{
				"orderId":  map[string]any{"S": "A-1001"},
				"total":    map[string]any{"N": "65.97"},
				"currency": map[string]any{"S": "EUR"},
				"language": map[string]any{"S": "en"},
				"paidAt":   map[string]any{"S": "2026-11-01T10:00:00Z"},
			}))
			Expect(aws.Records()).To(Equal([]string{`{"type":"order-paid","orderId":"A-1001","at":"2026-11-01T10:00:00Z","total":"65.97"}`}))
			Expect(sqs.WaitingIn(names.InvoicesQueue)).To(Equal(1))
		})

		It("calls through Hotline as the integration checkout, on AWS's own host names", func() {
			pay()
			for _, c := range aws.Calls() {
				Expect(c.ProxyAuth).To(Equal("Basic " + base64.StdEncoding.EncodeToString([]byte("checkout:x"))))
				Expect(c.Authorization).To(HavePrefix("AWS4-HMAC-SHA256 "))
			}
			Expect(aws.Calls()[1].Host).To(Equal("dynamodb.eu-west-1.amazonaws.com"))
			Expect(aws.Calls()[2].Host).To(Equal("kinesis.eu-west-1.amazonaws.com"))
		})

		It("stops before saving anything when the payment key is missing", func() {
			delete(aws.Secrets, names.PaymentSecret)
			_, err := app("checkout").Pay(ctx, shop.Payment{OrderID: "A-1001", Language: "en", Template: "default"}, now)
			Expect(err).To(HaveOccurred())
			Expect(aws.Item("orders", "A-1001")).To(BeNil())
		})

		It("refuses an order it doesn't know", func() {
			_, err := app("checkout").Pay(ctx, shop.Payment{OrderID: "A-9999", Language: "en", Template: "default"}, now)
			Expect(err).To(MatchError(invoice.ErrUnknownOrder))
		})
	})

	Describe("invoices", func() {
		work := func() {
			Expect(app("invoices").Work(ctx, true, "")).To(Succeed())
		}

		It("makes the PDF, signs it, stores it, saves its row and announces it", func() {
			pay()
			work()
			Expect(files.File("A-1001.pdf")).To(HavePrefix("%PDF-1.4"))
			Expect(aws.Targets()).To(ContainElements(
				"AmazonSSM.GetParameter",
				"TrentService.Sign",
				"DynamoDB_20120810.PutItem",
				"AWSEvents.PutEvents",
			))
			row := aws.Item("invoices", "A-1001")
			Expect(row).To(HaveKeyWithValue("file", map[string]any{"S": "A-1001.pdf"}))
			Expect(row).To(HaveKeyWithValue("pages", map[string]any{"N": "1"}))
			Expect(row).To(HaveKeyWithValue("templateId", map[string]any{"S": "invoice-standard-v2"}))
			Expect(row).To(HaveKey("signature"))

			event := aws.Events()[0]
			Expect(event).To(HaveKeyWithValue("EventBusName", "shop"))
			Expect(event).To(HaveKeyWithValue("Source", "shop.invoices"))
			Expect(event).To(HaveKeyWithValue("DetailType", "InvoiceReady"))
			var detail map[string]any
			Expect(json.Unmarshal([]byte(event["Detail"].(string)), &detail)).To(Succeed())
			Expect(detail).To(HaveKeyWithValue("file", "A-1001.pdf"))
			Expect(detail).To(HaveKeyWithValue("sendTo", "jana@example.com"))

			Expect(aws.Records()).To(HaveLen(2))
			Expect(aws.Records()[1]).To(HavePrefix(`{"type":"invoice-made","orderId":"A-1001"`))
			Expect(sqs.WaitingIn(names.InvoicesQueue)).To(Equal(0))
		})

		It("signs the PDF's SHA-256 fingerprint with the invoice key", func() {
			pay()
			work()
			for _, c := range aws.Calls() {
				if c.Target == "TrentService.Sign" {
					Expect(c.Body).To(HaveKeyWithValue("KeyId", "alias/invoice-signing"))
					Expect(c.Body).To(HaveKeyWithValue("MessageType", "DIGEST"))
					Expect(c.Body).To(HaveKeyWithValue("SigningAlgorithm", "ECDSA_SHA_256"))
				}
			}
		})

		It("uses the template from the shop's settings for a job asking for the default", func() {
			_, err := app("checkout").Pay(ctx, shop.Payment{OrderID: "A-1001", Language: "en", Template: "default"}, now)
			Expect(err).NotTo(HaveOccurred())
			work()
			Expect(aws.Item("invoices", "A-1001")).To(HaveKeyWithValue("templateId", map[string]any{"S": "invoice-compact-v1"}))
		})

		It("doesn't announce an invoice twice when its row is already there", func() {
			pay()
			work()
			pay()
			work()
			Expect(aws.Events()).To(HaveLen(1))
			Expect(sqs.WaitingIn(names.InvoicesQueue)).To(Equal(0))
		})

		It("tells an invoice's status from its row", func() {
			pay()
			work()
			status, err := app("invoices").Status(ctx, "A-1001")
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(ContainSubstring("A-1001.pdf"))
			_, err = app("invoices").Status(ctx, "A-1002")
			Expect(err).To(MatchError(invoice.ErrNoInvoice))
		})
	})

	Describe("mailer", func() {
		It("mails the ready invoice, here as a file, and deletes the message", func() {
			pay()
			Expect(app("invoices").Work(ctx, true, "")).To(Succeed())
			// The EventBridge rule on the bus puts the event on the mail queue.
			event := map[string]any{"detail-type": "InvoiceReady", "source": "shop.invoices"}
			var detail map[string]any
			Expect(json.Unmarshal([]byte(aws.Events()[0]["Detail"].(string)), &detail)).To(Succeed())
			event["detail"] = detail
			body, _ := json.Marshal(event)
			sqs.Put(names.MailQueue, string(body))

			out := GinkgoT().TempDir()
			Expect(app("mailer").Mail(ctx, true, out)).To(Succeed())
			mail, err := os.ReadFile(filepath.Join(out, "A-1001.eml")) //nolint:gosec // the test's own file
			Expect(err).NotTo(HaveOccurred())
			Expect(string(mail)).To(ContainSubstring("To: jana@example.com"))
			Expect(string(mail)).To(ContainSubstring("Content-Type: application/pdf"))
			Expect(sqs.WaitingIn(names.MailQueue)).To(Equal(0))
		})

		It("leaves a message it can't read on the queue", func() {
			sqs.Put(names.MailQueue, "not an event")
			Expect(app("mailer").Mail(ctx, true, GinkgoT().TempDir())).To(Succeed())
			Expect(sqs.WaitingIn(names.MailQueue)).To(Equal(1))
			Expect(logs.String()).To(ContainSubstring("message refused"))
		})
	})

	Describe("refunds", func() {
		It("reads the order and starts the credit note workflow", func() {
			pay()
			run, err := app("refunds").Refund(ctx, "A-1001", "10.00", "returned")
			Expect(err).NotTo(HaveOccurred())
			Expect(run).To(HaveSuffix(":run-1"))
			started := aws.Runs()[0]
			Expect(started).To(HaveKeyWithValue("stateMachineArn", names.CreditNoteWorkflow))
			Expect(started["input"]).To(MatchJSON(`{"orderId":"A-1001","amount":"10.00","currency":"EUR","reason":"returned"}`))
		})

		It("refuses a refund for an order that was never paid", func() {
			_, err := app("refunds").Refund(ctx, "A-1002", "10.00", "returned")
			Expect(err).To(MatchError(shop.ErrNoOrder))
			Expect(aws.Runs()).To(BeEmpty())
		})
	})

	Describe("the log group", func() {
		It("gets each app's log lines, in the app's own log stream", func() {
			names.LogGroup = "/shop/aws-shop"
			pay()
			var streams []string
			for _, c := range aws.Calls() {
				if strings.HasPrefix(c.Target, "Logs_") {
					streams = append(streams, c.Body["logStreamName"].(string))
				}
			}
			Expect(streams).To(HaveEach("checkout"))
			Expect(aws.Logs()).To(ContainElement(ContainSubstring("order A-1001 paid")))
		})

		It("is left alone without a log group", func() {
			names.LogGroup = ""
			pay()
			Expect(aws.Logs()).To(BeEmpty())
		})
	})
})
