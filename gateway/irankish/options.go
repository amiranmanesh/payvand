package irankish

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the IranKish specific options, all of them optional.
type config struct {
	// transactionType overrides the operation sent to IranKish.
	transactionType string
	// sendMobileAsCmsID sends the payer mobile as the CMS preservation id,
	// which lets the payer use the cards saved for that number.
	sendMobileAsCmsID bool
}

// settings returns the IranKish option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithTransactionType overrides the IranKish transaction type. The default is
// "Purchase"; bill payments and top-ups use their own values.
func WithTransactionType(transactionType string) core.Option {
	return func(o *core.Options) { settings(o).transactionType = transactionType }
}

// WithMobileAsCmsID sends the payer mobile as IranKish's CMS preservation id.
func WithMobileAsCmsID(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).sendMobileAsCmsID = enabled }
}
