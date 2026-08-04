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
	// reversible marks the payment as reversible, which PayPing requires at
	// creation time for [Gateway.Refund] to be accepted later.
	reversible bool
	// blocked creates the payment with its settlement blocked.
	blocked bool
	// multiplexing is the split settlement definition.
	multiplexing []Share
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

// WithReversible creates every payment as reversible, the flag PayPing requires
// at creation time before [Gateway.Refund] may return the money to the payer's
// card. Without it a reversal is rejected, and the window is 30 minutes after
// verification.
func WithReversible() core.Option {
	return func(o *core.Options) { settings(o).reversible = true }
}

// WithBlockedSettlement creates every payment with its settlement blocked, so
// the money stays held until it is released from the PayPing panel.
func WithBlockedSettlement() core.Option {
	return func(o *core.Options) { settings(o).blocked = true }
}

// WithMultiplexing enables split settlement between the given PayPing accounts.
// Every share amount is in Toman and the shares must add up to the amount of
// the purchase request, which Payvand checks before sending anything.
func WithMultiplexing(shares ...Share) core.Option {
	return func(o *core.Options) { settings(o).multiplexing = shares }
}
