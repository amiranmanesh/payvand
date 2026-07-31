package mellat

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the Mellat specific options, all of them optional.
type config struct {
	// additionalData is the free text kept with the transaction.
	additionalData string
	// payerID is Mellat's payer identifier, sent as "0" when unset.
	payerID string
	// skipSettle stops the settlement call that normally follows a
	// verification, for merchants that settle in a separate batch.
	skipSettle bool
}

// settings returns the Mellat option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithAdditionalData sets the free text kept with the transaction.
func WithAdditionalData(data string) core.Option {
	return func(o *core.Options) { settings(o).additionalData = data }
}

// WithPayerID sets Mellat's payer identifier. Without it "0" is sent.
func WithPayerID(payerID string) core.Option {
	return func(o *core.Options) { settings(o).payerID = payerID }
}

// WithoutSettle stops [Gateway.Verify] from calling bpSettleRequest, leaving
// the settlement to a separate process. Note that an unsettled transaction is
// reversed by the bank, so only use it when that process exists.
func WithoutSettle(skip bool) core.Option {
	return func(o *core.Options) { settings(o).skipSettle = skip }
}
