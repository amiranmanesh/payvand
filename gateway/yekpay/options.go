package yekpay

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// ISO 4217 numeric currency codes YekPay works with. The gateway is
// multi-currency, which is why the codes are numbers rather than the
// [core.Currency] used elsewhere.
const (
	// CurrencyIRR is the Iranian Rial.
	CurrencyIRR = 364
	// CurrencyEUR is the Euro.
	CurrencyEUR = 978
	// CurrencyUSD is the US Dollar.
	CurrencyUSD = 840
	// CurrencyGBP is the British Pound.
	CurrencyGBP = 826
	// CurrencyTRY is the Turkish Lira.
	CurrencyTRY = 949
)

// config holds the YekPay specific options.
type config struct {
	// fromCurrency is the currency the payer is charged in.
	fromCurrency int
	// toCurrency is the currency the merchant is settled in.
	toCurrency int
	// description is used when a purchase request carries none.
	description string
	// address is the payer address some YekPay contracts require.
	address string
}

// settings returns the YekPay option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithCurrencies pins the pair of ISO 4217 numeric codes: the currency the
// payer is charged in and the one the merchant is settled in. Without it both
// default to Rial.
func WithCurrencies(from, to int) core.Option {
	return func(o *core.Options) {
		s := settings(o)
		s.fromCurrency = from
		s.toCurrency = to
	}
}

// WithDefaultDescription sets the description used when a purchase request
// carries none.
func WithDefaultDescription(description string) core.Option {
	return func(o *core.Options) { settings(o).description = description }
}

// WithAddress sets the payer address sent with every request.
func WithAddress(address string) core.Option {
	return func(o *core.Options) { settings(o).address = address }
}
