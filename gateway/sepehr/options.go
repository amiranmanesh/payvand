package sepehr

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the Sepehr specific options, all of them optional.
type config struct {
	// payload is the free field echoed back with the callback.
	payload string
	// sendPayerDetails sends the mobile number and national code with the
	// token request, for terminals that require card holder matching.
	sendPayerDetails bool
}

// settings returns the Sepehr option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithPayload sets the free field Sepehr echoes back with the callback.
func WithPayload(payload string) core.Option {
	return func(o *core.Options) { settings(o).payload = payload }
}

// WithPayerDetails sends the payer mobile number and national code with the
// token request.
func WithPayerDetails(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).sendPayerDetails = enabled }
}
