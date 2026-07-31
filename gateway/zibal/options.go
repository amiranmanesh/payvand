package zibal

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the Zibal specific options, all of them optional.
type config struct {
	// ledgerID routes the payment to a specific Zibal ledger.
	ledgerID string
	// feeMode selects who pays the Zibal fee (0 merchant, 1 payer, 2 ledger).
	feeMode int
	// checkMobileWithCard forces the paying card to belong to the mobile.
	checkMobileWithCard bool
	// multiplexing is the split settlement definition.
	multiplexing []Share
	// percentMode expresses the shares as percentages instead of amounts.
	percentMode bool
	// description is used when a purchase request carries none.
	description string
}

// settings returns the Zibal option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithLedger routes settlement to a specific Zibal ledger.
func WithLedger(ledgerID string) core.Option {
	return func(o *core.Options) { settings(o).ledgerID = ledgerID }
}

// WithFeeMode selects who pays the gateway fee: 0 the merchant, 1 the payer,
// 2 the ledger. Omit it to keep the terminal default.
func WithFeeMode(mode int) core.Option {
	return func(o *core.Options) { settings(o).feeMode = mode }
}

// WithMobileCardCheck makes Zibal reject a card that does not belong to the
// mobile number sent with the request.
func WithMobileCardCheck(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).checkMobileWithCard = enabled }
}

// WithMultiplexing enables split settlement between the given destinations.
func WithMultiplexing(shares ...Share) core.Option {
	return func(o *core.Options) { settings(o).multiplexing = shares }
}

// WithPercentMultiplexing enables split settlement where every share amount is
// a percentage of the payment.
func WithPercentMultiplexing(shares ...Share) core.Option {
	return func(o *core.Options) {
		s := settings(o)
		s.multiplexing = shares
		s.percentMode = true
	}
}

// WithDefaultDescription sets the description used when a purchase request
// carries none.
func WithDefaultDescription(description string) core.Option {
	return func(o *core.Options) { settings(o).description = description }
}
