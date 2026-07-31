package payir

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the Pay.ir specific options, all of them optional.
type config struct {
	// description is used when a purchase request carries none.
	description string
	// sendOrderAsFactor sends the order id as Pay.ir's factor number.
	sendOrderAsFactor bool
}

// settings returns the Pay.ir option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithDefaultDescription sets the description used when a purchase request
// carries none.
func WithDefaultDescription(description string) core.Option {
	return func(o *core.Options) { settings(o).description = description }
}

// WithOrderAsFactorNumber sends the order id as the Pay.ir factor number, so
// the payment is searchable by it in the panel.
func WithOrderAsFactorNumber(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).sendOrderAsFactor = enabled }
}
