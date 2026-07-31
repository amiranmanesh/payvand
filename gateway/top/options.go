package top

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the TOP specific options, all of them optional.
type config struct {
	// additionalInfo is the free text kept with the transaction.
	additionalInfo string
	// userID identifies the payer inside the TOP app when no mobile number is
	// available. One of the two must be present per business agreement.
	userID string
	// setData is the supplementary payload pushed after a confirmation.
	setData *AdditionalData
}

// settings returns the TOP option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithAdditionalInfo sets the free text kept with the transaction.
func WithAdditionalInfo(info string) core.Option {
	return func(o *core.Options) { settings(o).additionalInfo = info }
}

// WithUserID identifies the payer inside the TOP app when the request carries
// no mobile number.
func WithUserID(userID string) core.Option {
	return func(o *core.Options) { settings(o).userID = userID }
}

// WithSetData enables the optional SetData step run after a successful
// confirmation, which pushes service information back to the TOP app. It may
// be called once per transaction and never fails an already settled payment.
func WithSetData(data AdditionalData) core.Option {
	return func(o *core.Options) { settings(o).setData = &data }
}
