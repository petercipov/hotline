package invoice

import (
	"errors"
	"regexp"
)

// ErrUnknownOrder and ErrUnknownTemplate mean the job names something the
// worker doesn't have. The message is well formed, so Hotline's check passes;
// only the worker can tell.
var (
	ErrUnknownOrder    = errors.New("unknown order")
	ErrUnknownTemplate = errors.New("unknown template")
)

type line struct {
	item     string
	quantity int
	cents    int
}

type order struct {
	customer string
	currency string
	lines    []line
}

// sampleOrder is any other A order, such as A-20261101221714: it has
// A-1001's lines, so a run can pay a new order each time.
var sampleOrder = regexp.MustCompile(`^A-[0-9]+$`)

// findOrder looks an order up in the shop's order store.
func findOrder(id string) (order, bool) {
	if o, ok := orders()[id]; ok {
		return o, true
	}
	if sampleOrder.MatchString(id) {
		return orders()["A-1001"], true
	}
	return order{}, false
}

// orders stands in for the shop's own order store, so the example needs no
// database.
func orders() map[string]order {
	return map[string]order{
		"A-1001": {customer: "Jana Novak", currency: "EUR", lines: []line{
			{item: "Coffee beans 1 kg", quantity: 2, cents: 2499},
			{item: "Paper filters", quantity: 3, cents: 533},
		}},
		"A-1002": {customer: "Max Muller", currency: "EUR", lines: []line{
			{item: "Mug", quantity: 1, cents: 1200},
		}},
	}
}

// labels are the invoice's fixed words in each language, kept to plain
// letters so the PDF's built-in font can show them.
func labels(language string) struct{ invoice, customer, total string } {
	return map[string]struct{ invoice, customer, total string }{
		"en": {"Invoice", "Customer", "Total"},
		"de": {"Rechnung", "Kunde", "Summe"},
		"sk": {"Faktura", "Zakaznik", "Spolu"},
		"cs": {"Faktura", "Zakaznik", "Celkem"},
	}[language]
}
