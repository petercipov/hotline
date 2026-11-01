package invoice_test

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"hotline-example/aws-shop/invoice"
)

var _ = Describe("Rendering the invoice PDF", func() {
	job := func(order, template, language string) invoice.Job {
		return invoice.NewJob(order, template, language, "", time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))
	}

	It("writes a one-page PDF naming the order, its lines and the total", func() {
		pdf, err := invoice.Render(job("A-1001", "invoice-standard-v2", "en"))
		Expect(err).NotTo(HaveOccurred())

		Expect(pdf).To(HavePrefix("%PDF-1.4"))
		Expect(bytes.TrimSpace(pdf)).To(HaveSuffix("%%EOF"))
		Expect(string(pdf)).To(ContainSubstring("/Count 1"))
		Expect(string(pdf)).To(ContainSubstring("(Invoice A-1001)"))
		Expect(string(pdf)).To(ContainSubstring("Total: 65.97 EUR"))
	})

	It("gives an order's total and currency, for the shop's own records", func() {
		total, currency, err := invoice.OrderTotal("A-1001")
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal("65.97"))
		Expect(currency).To(Equal("EUR"))
		_, _, err = invoice.OrderTotal("A-9999")
		Expect(err).To(MatchError(invoice.ErrUnknownOrder))
	})

	It("uses the job's language", func() {
		pdf, err := invoice.Render(job("A-1001", "invoice-standard-v2", "de"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(pdf)).To(ContainSubstring("(Rechnung A-1001)"))
	})

	It("uses the job's template", func() {
		standard, _ := invoice.Render(job("A-1002", "invoice-standard-v2", "en"))
		compact, err := invoice.Render(job("A-1002", "invoice-compact-v1", "en"))
		Expect(err).NotTo(HaveOccurred())
		Expect(compact).NotTo(Equal(standard))
		Expect(string(compact)).To(ContainSubstring("Total: 12.00 EUR"))
	})

	It("keeps the cross-reference table pointing at each object", func() {
		pdf, _ := invoice.Render(job("A-1001", "invoice-standard-v2", "en"))
		xref := bytes.LastIndex(pdf, []byte("\nxref\n"))
		Expect(xref).To(BeNumerically(">", 0))
		var first, count int
		rest := pdf[xref+len("\nxref\n"):]
		_, err := fmt.Sscanf(string(rest), "%d %d", &first, &count)
		Expect(err).NotTo(HaveOccurred())
		lines := strings.Split(string(rest), "\n")[1 : 1+count]
		for i, line := range lines[1:] {
			var offset int
			_, err := fmt.Sscanf(line, "%010d", &offset)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(pdf[offset:])).To(HavePrefix(fmt.Sprintf("%d 0 obj", i+1)))
		}
		Expect(string(pdf)).To(ContainSubstring(fmt.Sprintf("startxref\n%d\n", xref+1)))
	})

	It("refuses an order it doesn't know", func() {
		_, err := invoice.Render(job("Z-1", "invoice-standard-v2", "en"))
		Expect(err).To(MatchError(invoice.ErrUnknownOrder))
	})

	It("refuses a template it doesn't know", func() {
		_, err := invoice.Render(job("A-1001", "invoice-fancy-v9", "en"))
		Expect(err).To(MatchError(invoice.ErrUnknownTemplate))
	})
})
