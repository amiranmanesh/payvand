package nextpay

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the NextPay specific options, all of them optional.
type config struct {
	// currency pins the unit sent to NextPay ("IRR" or "IRT").
	currency string
	// autoVerify lets NextPay verify the payment on its own side.
	autoVerify bool
	// description is used when a purchase request carries none.
	description string
}

// settings returns the NextPay option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithCurrency pins the unit sent to NextPay. Leave it out to send Rial.
func WithCurrency(c core.Currency) core.Option {
	return func(o *core.Options) { settings(o).currency = c.String() }
}

// WithAutoVerify asks NextPay to verify the transaction itself, which suits
// merchants that cannot run a verification call of their own.
func WithAutoVerify(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).autoVerify = enabled }
}

// WithDefaultDescription sets the payer description used when a purchase
// request carries none.
func WithDefaultDescription(description string) core.Option {
	return func(o *core.Options) { settings(o).description = description }
}
