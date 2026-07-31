package vandar

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the Vandar specific options, all of them optional.
type config struct {
	// port selects a specific IPG port of the business.
	port string
	// description is used when a purchase request carries none.
	description string
	// comment is attached to the payment in the Vandar panel.
	comment string
	// accessToken authenticates the business API used by refunds. Without it
	// the api key is sent in the body, which older businesses still accept.
	accessToken string
	// sendOrderAsFactor sends the order id as the Vandar factor number.
	sendOrderAsFactor bool
}

// settings returns the Vandar option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithPort selects a specific IPG port configured on the business.
func WithPort(port string) core.Option {
	return func(o *core.Options) { settings(o).port = port }
}

// WithDefaultDescription sets the description used when a purchase request
// carries none.
func WithDefaultDescription(description string) core.Option {
	return func(o *core.Options) { settings(o).description = description }
}

// WithComment attaches a comment shown in the Vandar panel.
func WithComment(comment string) core.Option {
	return func(o *core.Options) { settings(o).comment = comment }
}

// WithAccessToken sets the bearer token of the Vandar business API, which the
// refund endpoint requires.
func WithAccessToken(token string) core.Option {
	return func(o *core.Options) { settings(o).accessToken = token }
}

// WithOrderAsFactorNumber sends the order id as Vandar's factor number, so the
// payment is searchable by it in the panel.
func WithOrderAsFactorNumber(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).sendOrderAsFactor = enabled }
}
