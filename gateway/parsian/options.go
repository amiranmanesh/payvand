package parsian

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// Share is one destination account of a Parsian split settlement.
type Share struct {
	// IBAN is the destination account, e.g. "IR000000000000000000000000".
	IBAN string
	// Amount is the share value in Rial.
	Amount int64
	// PayID is the settlement identifier of the share. Leave it zero to let
	// Payvand generate one.
	PayID int64
}

// config holds the Parsian specific options, all of them optional.
type config struct {
	// additionalData is the free text kept with the transaction.
	additionalData string
	// shares is a fixed split settlement definition.
	shares []Share
	// splitToConfigIBAN sends the whole amount to the terminal IBAN through
	// the multiplexed service.
	splitToConfigIBAN bool
	// sendMobileAsOriginator sends the payer mobile as the originator.
	sendMobileAsOriginator bool
}

// settings returns the Parsian option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithAdditionalData sets the free text kept with the transaction.
func WithAdditionalData(data string) core.Option {
	return func(o *core.Options) { settings(o).additionalData = data }
}

// WithMultiplexing switches the terminal to the multiplexed sale service and
// splits every payment between the given accounts. Without it the plain sale
// service is used.
func WithMultiplexing(shares ...Share) core.Option {
	return func(o *core.Options) { settings(o).shares = shares }
}

// WithSettlementToIBAN switches the terminal to the multiplexed sale service
// and settles the full amount to [core.Config.IBAN].
func WithSettlementToIBAN(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).splitToConfigIBAN = enabled }
}

// WithMobileAsOriginator sends the payer mobile number as Parsian's originator
// field, so the payment page can offer the cards saved for that number.
func WithMobileAsOriginator(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).sendMobileAsOriginator = enabled }
}
