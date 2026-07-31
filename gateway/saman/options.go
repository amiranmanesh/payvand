package saman

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the Saman specific options, all of them optional.
type config struct {
	// getMethod asks Saman to redirect the payer back with a GET request.
	getMethod bool
	// sendMobile sends the payer mobile so the saved cards can be offered.
	sendMobile bool
}

// settings returns the Saman option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithGetMethod asks Saman to send the payment result back as a GET request
// instead of the default POST form.
func WithGetMethod(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).getMethod = enabled }
}

// WithMobile sends the payer mobile number with the token request, which lets
// the payment page offer the cards saved for that number.
func WithMobile(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).sendMobile = enabled }
}
