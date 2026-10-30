package invoice_test

import (
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"hotline-example/aws-sqs-invoices/invoice"
)

const validJob = `{"schemaVersion":1,"eventId":"5f0c1b6e-3a52-4f7e-9d0e-2b8f6f1f2a10","orderId":"A-1001","templateId":"invoice-standard-v2","language":"en","requestedAt":"2026-10-30T19:30:00Z"}`

var _ = Describe("The invoice job message", func() {
	It("is made with a fresh event ID and the time it was asked for", func() {
		now := time.Date(2026, 10, 30, 19, 30, 0, 0, time.UTC)
		a := invoice.NewJob("A-1001", "invoice-standard-v2", "en", "", now)
		b := invoice.NewJob("A-1001", "invoice-standard-v2", "en", "", now)

		Expect(a.EventID).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`))
		Expect(a.EventID).NotTo(Equal(b.EventID))
		Expect(a.SchemaVersion).To(Equal(1))
		Expect(a.RequestedAt).To(Equal(now))
	})

	It("passes its own check once written as JSON", func() {
		job := invoice.NewJob("A-1001", "invoice-standard-v2", "de", "customer@example.com", time.Now())
		body, err := json.Marshal(job)
		Expect(err).NotTo(HaveOccurred())

		read, err := invoice.Check(body)
		Expect(err).NotTo(HaveOccurred())
		Expect(read.OrderID).To(Equal("A-1001"))
		Expect(read.SendTo).To(Equal("customer@example.com"))
	})

	It("leaves out sendTo when there is none", func() {
		body, _ := json.Marshal(invoice.NewJob("A-1001", "invoice-standard-v2", "en", "", time.Now()))
		Expect(string(body)).NotTo(ContainSubstring("sendTo"))
	})

	It("accepts a valid job", func() {
		job, err := invoice.Check([]byte(validJob))
		Expect(err).NotTo(HaveOccurred())
		Expect(job.TemplateID).To(Equal("invoice-standard-v2"))
	})

	DescribeTable("refuses a broken job",
		func(change func(map[string]any), want string) {
			var m map[string]any
			Expect(json.Unmarshal([]byte(validJob), &m)).To(Succeed())
			change(m)
			body, _ := json.Marshal(m)
			_, err := invoice.Check(body)
			Expect(err).To(MatchError(ContainSubstring(want)))
		},
		Entry("no orderId", func(m map[string]any) { delete(m, "orderId") }, "orderId"),
		Entry("no templateId", func(m map[string]any) { delete(m, "templateId") }, "templateId"),
		Entry("orderId as a number", func(m map[string]any) { m["orderId"] = 1001 }, "orderId"),
		Entry("orderId in the wrong shape", func(m map[string]any) { m["orderId"] = "a-1001" }, "orderId"),
		Entry("templateId with spaces", func(m map[string]any) { m["templateId"] = "Invoice Standard" }, "templateId"),
		Entry("eventId not a uuid", func(m map[string]any) { m["eventId"] = "42" }, "eventId"),
		Entry("unknown language", func(m map[string]any) { m["language"] = "fr" }, "language"),
		Entry("schemaVersion as text", func(m map[string]any) { m["schemaVersion"] = "1" }, "schemaVersion"),
		Entry("requestedAt not a time", func(m map[string]any) { m["requestedAt"] = "yesterday" }, "requestedAt"),
		Entry("sendTo not an email", func(m map[string]any) { m["sendTo"] = "nobody" }, "sendTo"),
		Entry("an extra field", func(m map[string]any) { m["priority"] = "high" }, "priority"),
	)

	It("refuses text that is not JSON", func() {
		_, err := invoice.Check([]byte("order 42 paid"))
		Expect(err).To(MatchError(ContainSubstring("not JSON")))
	})

	It("publishes its schema, so Hotline's overlay can use the same one", func() {
		var schema map[string]any
		Expect(json.Unmarshal(invoice.Schema, &schema)).To(Succeed())
		Expect(schema["required"]).To(ContainElements("schemaVersion", "eventId", "orderId", "templateId", "language", "requestedAt"))
	})
})
