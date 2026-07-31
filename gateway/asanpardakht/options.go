package asanpardakht

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the AsanPardakht specific options, all of them optional.
type config struct {
	// serviceTypeID overrides the service type of the token request.
	serviceTypeID int
	// paymentID is the bill payment identifier, when the terminal uses one.
	paymentID string
	// additionalData is the free text kept with the transaction.
	additionalData string
	// settlements is the split settlement definition.
	settlements []Settlement
	// skipSettlement stops the settlement call that normally follows a
	// verification.
	skipSettlement bool
	// cancelInsteadOfReverse makes Refund call Cancel, which only works
	// before the transaction is settled.
	cancelInsteadOfReverse bool
}

// settings returns the AsanPardakht option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithServiceType overrides the AsanPardakht service type id. The default is 1
// (purchase).
func WithServiceType(serviceTypeID int) core.Option {
	return func(o *core.Options) { settings(o).serviceTypeID = serviceTypeID }
}

// WithPaymentID sets the bill payment identifier.
func WithPaymentID(paymentID string) core.Option {
	return func(o *core.Options) { settings(o).paymentID = paymentID }
}

// WithAdditionalData sets the free text kept with the transaction.
func WithAdditionalData(data string) core.Option {
	return func(o *core.Options) { settings(o).additionalData = data }
}

// WithSettlements enables split settlement between the given accounts.
func WithSettlements(settlements ...Settlement) core.Option {
	return func(o *core.Options) { settings(o).settlements = settlements }
}

// WithoutSettlement stops [Gateway.Verify] from calling the settlement
// endpoint, leaving it to a separate process.
func WithoutSettlement(skip bool) core.Option {
	return func(o *core.Options) { settings(o).skipSettlement = skip }
}

// WithCancelInsteadOfReverse makes [Gateway.Refund] call Cancel rather than
// Reverse. Cancel only works while the transaction is not settled yet.
func WithCancelInsteadOfReverse(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).cancelInsteadOfReverse = enabled }
}
