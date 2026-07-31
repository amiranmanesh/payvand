package pasargad

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the Pasargad specific options, all of them optional.
type config struct {
	// action overrides the operation code sent with the token request.
	action string
	// sendPayerDetails sends the payer mobile and email with the request.
	sendPayerDetails bool
	// skipTransactionCheck skips the CheckTransactionResult call that
	// normally precedes a verification.
	skipTransactionCheck bool
}

// settings returns the Pasargad option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithAction overrides the Pasargad action code. The default is "1003"
// (purchase); other codes cover instalments and bill payments.
func WithAction(action string) core.Option {
	return func(o *core.Options) { settings(o).action = action }
}

// WithPayerDetails sends the payer mobile number and email with the token
// request.
func WithPayerDetails(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).sendPayerDetails = enabled }
}

// WithoutTransactionCheck skips the CheckTransactionResult call that
// [Gateway.Verify] runs before settling, for merchants that already validated
// the transaction themselves.
func WithoutTransactionCheck(skip bool) core.Option {
	return func(o *core.Options) { settings(o).skipTransactionCheck = skip }
}
