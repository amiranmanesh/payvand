package payping

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the PayPing specific options, all of them optional.
type config struct {
	// description is used when a purchase request carries none.
	description string
	// payerIdentity overrides the payer identity (mobile or email) sent with
	// the request.
	payerIdentity string
}

// settings returns the PayPing option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithDefaultDescription sets the description used when a purchase request
// carries none.
func WithDefaultDescription(description string) core.Option {
	return func(o *core.Options) { settings(o).description = description }
}

// WithPayerIdentity pins the payer identity PayPing shows on the receipt.
// Without it the mobile number of the request is used.
func WithPayerIdentity(identity string) core.Option {
	return func(o *core.Options) { settings(o).payerIdentity = identity }
}
