package sadad

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the Sadad specific options, all of them optional.
type config struct {
	// applicationName is shown on the Sadad payment page.
	applicationName string
	// additionalData is the free field kept on the Sadad receipt.
	additionalData string
	// sendMobileAsUserID sends the payer mobile as Sadad's UserId, which
	// pre-fills the saved card list on the payment page.
	sendMobileAsUserID bool
}

// settings returns the Sadad option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithApplicationName sets the merchant application name shown to the payer.
func WithApplicationName(name string) core.Option {
	return func(o *core.Options) { settings(o).applicationName = name }
}

// WithAdditionalData sets the free text kept with the transaction.
func WithAdditionalData(data string) core.Option {
	return func(o *core.Options) { settings(o).additionalData = data }
}

// WithMobileAsUserID sends the payer mobile number as Sadad's UserId so the
// payment page can offer the cards saved for that number.
func WithMobileAsUserID(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).sendMobileAsUserID = enabled }
}
