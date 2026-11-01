package invoice

import (
	"bytes"
	"fmt"
	"strings"
)

// templates turn an order into the lines of the invoice page.
func templates() map[string]func(Job, order) []string {
	return map[string]func(Job, order) []string{
		"invoice-standard-v2": func(job Job, o order) []string {
			l := labels(job.Language)
			out := []string{
				l.invoice + " " + job.OrderID,
				l.customer + ": " + o.customer,
				job.RequestedAt.Format("2006-01-02"),
				"",
			}
			for _, li := range o.lines {
				out = append(out, fmt.Sprintf("%d x %s  %s %s", li.quantity, li.item, money(li.quantity*li.cents), o.currency))
			}
			return append(out, "", l.total+": "+money(total(o))+" "+o.currency)
		},
		"invoice-compact-v1": func(job Job, o order) []string {
			l := labels(job.Language)
			return []string{
				l.invoice + " " + job.OrderID + ", " + o.customer,
				l.total + ": " + money(total(o)) + " " + o.currency,
			}
		},
	}
}

func total(o order) int {
	sum := 0
	for _, li := range o.lines {
		sum += li.quantity * li.cents
	}
	return sum
}

func money(cents int) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

// OrderTotal is an order's total, such as 65.97, and its currency.
func OrderTotal(orderID string) (string, string, error) {
	o, ok := orders()[orderID]
	if !ok {
		return "", "", ErrUnknownOrder
	}
	return money(total(o)), o.currency, nil
}

// Render makes the one-page invoice PDF the job asks for.
func Render(job Job) ([]byte, error) {
	o, ok := orders()[job.OrderID]
	if !ok {
		return nil, ErrUnknownOrder
	}
	template, ok := templates()[job.TemplateID]
	if !ok {
		return nil, ErrUnknownTemplate
	}
	return page(template(job, o)), nil
}

// page writes a minimal PDF: one A4 page of text lines in Helvetica. It is
// small enough to need no PDF library.
func page(lines []string) []byte {
	var text strings.Builder
	text.WriteString("BT /F1 12 Tf 16 TL 56 780 Td\n")
	for _, l := range lines {
		fmt.Fprintf(&text, "(%s) Tj T*\n", escape(l))
	}
	text.WriteString("ET")

	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", text.Len(), text.String()),
	}

	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, o := range objects {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return b.Bytes()
}

func escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`).Replace(s)
}
