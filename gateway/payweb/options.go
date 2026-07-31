package payweb

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the PayWeb specific options, all of them optional.
type config struct {
	// comment is used when a purchase request carries no description.
	comment string
	// restrictToCard sends the payer card number so only that card can pay.
	restrictToCard bool
}

// settings returns the PayWeb option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithDefaultComment sets the comment used when a purchase request carries no
// description.
func WithDefaultComment(comment string) core.Option {
	return func(o *core.Options) { settings(o).comment = comment }
}

// WithCardRestriction forwards the first entry of
// [core.PurchaseRequest.AllowedCards] to PayWeb, so only that card can pay.
func WithCardRestriction(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).restrictToCard = enabled }
}
