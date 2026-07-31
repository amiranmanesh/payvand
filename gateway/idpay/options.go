package idpay

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the IDPay specific options, all of them optional.
type config struct {
	// description is used when a purchase request carries none.
	description string
}

// settings returns the IDPay option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithDefaultDescription sets the description used when a purchase request
// carries none.
func WithDefaultDescription(description string) core.Option {
	return func(o *core.Options) { settings(o).description = description }
}
