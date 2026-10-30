package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"hotline-example/aws-sqs-invoices/internal/fakesqs"
)

const queue = "https://sqs.eu-west-1.amazonaws.com/192325781350/invoices"

var _ = Describe("The example program", func() {
	var (
		fake           *fakesqs.SQS
		stdout, stderr *bytes.Buffer
	)

	BeforeEach(func() {
		fake = fakesqs.Start()
		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
		GinkgoT().Setenv("AWS_REGION", "eu-west-1")
		GinkgoT().Setenv("AWS_ACCESS_KEY_ID", "AKIDEXAMPLE")
		GinkgoT().Setenv("AWS_SECRET_ACCESS_KEY", "secret")
		GinkgoT().Setenv("INVOICES_QUEUE_URL", queue)
		GinkgoT().Setenv("HOTLINE_PROXY", "http://invoices:x@"+fake.Addr)
	})

	run := func(args ...string) int {
		return run(context.Background(), args, stdout, stderr)
	}

	It("requests an invoice, then the worker writes it", func() {
		Expect(run("request", "--order", "A-1001", "--template", "invoice-standard-v2", "--language", "sk")).To(Equal(0))
		Expect(stdout.String()).To(ContainSubstring("sent m-1"))

		var job map[string]any
		Expect(json.Unmarshal([]byte(fake.Calls()[0].Body["MessageBody"].(string)), &job)).To(Succeed())
		Expect(job).To(HaveKeyWithValue("language", "sk"))
		Expect(job).NotTo(HaveKey("sendTo"))

		out := filepath.Join(GinkgoT().TempDir(), "invoices")
		Expect(run("work", "--out", out, "--once")).To(Equal(0))
		Expect(filepath.Join(out, "invoice-A-1001.pdf")).To(BeAnExistingFile())
		Expect(fake.Waiting()).To(Equal(0))
	})

	It("takes the queue and proxy from flags too", func() {
		GinkgoT().Setenv("INVOICES_QUEUE_URL", "")
		GinkgoT().Setenv("HOTLINE_PROXY", "")
		Expect(run("send", "--queue", queue, "--proxy", "http://invoices:x@"+fake.Addr, "--body", "anything")).To(Equal(0))
		Expect(fake.Calls()[0].RequestURI).To(Equal("http://sqs.eu-west-1.amazonaws.com/"))
	})

	It("sends any text unchanged with send, to try Hotline's checks", func() {
		Expect(run("send", "--body", `{"schemaVersion":1}`)).To(Equal(0))
		Expect(fake.Calls()[0].Body["MessageBody"]).To(Equal(`{"schemaVersion":1}`))
	})

	It("can reach a local SQS without Hotline", func() {
		GinkgoT().Setenv("HOTLINE_PROXY", "")
		Expect(run("send", "--endpoint", "http://"+fake.Addr, "--body", "x")).To(Equal(0))
		Expect(fake.Calls()[0].RequestURI).To(Equal("/"))
	})

	It("refuses to request a job that would fail its own check", func() {
		Expect(run("request", "--order", "a-1", "--template", "invoice-standard-v2", "--language", "en")).To(Equal(1))
		Expect(stderr.String()).To(ContainSubstring("$.orderId pattern"))
		Expect(fake.Calls()).To(BeEmpty())
	})

	It("reports a failed send", func() {
		GinkgoT().Setenv("HOTLINE_PROXY", "http://invoices:x@127.0.0.1:1")
		Expect(run("send", "--body", "x")).To(Equal(1))
		Expect(stderr.String()).NotTo(BeEmpty())
	})

	DescribeTable("refuses a wrong command line with exit code 2",
		func(args ...string) {
			Expect(run(args...)).To(Equal(2))
			Expect(stderr.String()).NotTo(BeEmpty())
		},
		Entry("no command"),
		Entry("an unknown command", "fly"),
		Entry("an unknown flag", "work", "--fast"),
		Entry("request without an order", "request", "--template", "t", "--language", "en"),
		Entry("send without a body", "send"),
		Entry("work without a folder", "work"),
	)

	It("needs a queue", func() {
		GinkgoT().Setenv("INVOICES_QUEUE_URL", "")
		Expect(run("send", "--body", "x")).To(Equal(2))
		Expect(stderr.String()).To(ContainSubstring("queue"))
	})
})
