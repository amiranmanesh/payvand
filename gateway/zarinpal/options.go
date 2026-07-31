package zarinpal

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the Zarinpal specific options. Everything in it is optional:
// leaving a field untouched means the parameter is simply not sent.
type config struct {
	// currency overrides the unit sent to Zarinpal.
	currency core.Currency
	// currencySet tells whether the caller pinned a currency.
	currencySet bool
	// wages is the split settlement definition.
	wages []Wage
	// defaultDescription is used when the request carries none, because
	// Zarinpal rejects a request without a description.
	defaultDescription string
}

// settings returns the Zarinpal option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithCurrency pins the unit sent to Zarinpal. Terminals are configured either
// in Rial or in Toman; use this when yours is not the default Rial.
func WithCurrency(c core.Currency) core.Option {
	return func(o *core.Options) {
		s := settings(o)
		s.currency = c
		s.currencySet = true
	}
}

// WithWages enables split settlement: the payment is divided between the given
// IBANs at settlement time. Omit it and no wages are sent at all.
func WithWages(wages ...Wage) core.Option {
	return func(o *core.Options) { settings(o).wages = wages }
}

// WithDefaultDescription sets the description used when a purchase request
// carries none. Zarinpal requires a non-empty description.
func WithDefaultDescription(description string) core.Option {
	return func(o *core.Options) { settings(o).defaultDescription = description }
}
