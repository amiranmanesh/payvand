package tara

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// InvoiceItem is one line of the invoice Tara is shown. Tara pays from a club
// credit that is granted per product group, so the group of every line decides
// whether the payer may buy it at all.
type InvoiceItem struct {
	// Name is the product name shown to the payer.
	Name string `json:"name"`
	// Code is the merchant side product code.
	Code string `json:"code"`
	// Count is the quantity.
	Count int `json:"count"`
	// Unit is Tara's unit of measure code; 5 is "piece".
	Unit int `json:"unit"`
	// Fee is the unit price in Rial.
	Fee int64 `json:"fee"`
	// Group is the Tara club group code of the product.
	Group string `json:"group"`
	// GroupTitle is the human readable name of the group.
	GroupTitle string `json:"groupTitle"`
	// Data is a free text kept with the line.
	Data string `json:"data"`
}

// InvoiceBuilder turns a purchase request into the invoice lines sent to Tara.
// Set one with [WithInvoiceBuilder] when the real basket matters; the default
// builder sends a single line covering the whole order.
type InvoiceBuilder func(req core.PurchaseRequest) []InvoiceItem

// config holds the Tara specific options.
type config struct {
	// serviceID routes the amount to a service of the merchant, which is how
	// Tara separates several shops under one contract.
	serviceID string
	// invoiceBuilder produces the invoice lines of a purchase.
	invoiceBuilder InvoiceBuilder
	// group and groupTitle are used by the default invoice builder.
	group      string
	groupTitle string
	// unit is the unit of measure used by the default invoice builder.
	unit int
	// clientIP is sent when the request carries none.
	clientIP string
}

// settings returns the Tara option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithServiceID routes the amount to one of the merchant's Tara services.
func WithServiceID(serviceID string) core.Option {
	return func(o *core.Options) { settings(o).serviceID = serviceID }
}

// WithInvoiceBuilder installs the function that turns a purchase request into
// the invoice lines Tara is shown.
func WithInvoiceBuilder(builder InvoiceBuilder) core.Option {
	return func(o *core.Options) { settings(o).invoiceBuilder = builder }
}

// WithInvoiceItems sends a fixed invoice with every purchase.
func WithInvoiceItems(items ...InvoiceItem) core.Option {
	return func(o *core.Options) {
		settings(o).invoiceBuilder = func(core.PurchaseRequest) []InvoiceItem { return items }
	}
}

// WithDefaultGroup sets the club group the default invoice builder puts every
// line in. Ask Tara for the group codes your contract may sell.
func WithDefaultGroup(code, title string) core.Option {
	return func(o *core.Options) {
		c := settings(o)
		c.group, c.groupTitle = code, title
	}
}

// WithDefaultUnit sets the unit of measure code the default invoice builder
// uses. Tara's default, 5, means "piece".
func WithDefaultUnit(unit int) core.Option {
	return func(o *core.Options) { settings(o).unit = unit }
}

// WithClientIP sets the payer IP address sent when a request carries none.
// Tara uses it for its own fraud checks.
func WithClientIP(ip string) core.Option {
	return func(o *core.Options) { settings(o).clientIP = ip }
}
