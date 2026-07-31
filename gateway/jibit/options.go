package jibit

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the Jibit specific options, all of them optional.
type config struct {
	// wage is the fee, in Rial, Jibit adds on top of every purchase and pays
	// out to the merchant's wage account.
	wage int64
	// description is used when a purchase request carries none.
	description string
	// userIdentifier is the merchant side payer identifier sent with every
	// purchase, used by Jibit to match the payer's cards.
	userIdentifier string
	// matchPayerCard sends the payer's card number so Jibit rejects a payment
	// made with any other card.
	matchPayerCard bool
	// refundCancellable asks Jibit to keep a refund cancellable instead of
	// transferring it immediately.
	refundCancellable bool
	// additionalData is attached to every purchase and echoed back in the
	// panel and the reports.
	additionalData map[string]string
}

// settings returns the Jibit option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithWage sets the fee, in Rial, added on top of every purchase amount.
func WithWage(rial int64) core.Option {
	return func(o *core.Options) { settings(o).wage = rial }
}

// WithDefaultDescription sets the description used when a purchase request
// carries none.
func WithDefaultDescription(description string) core.Option {
	return func(o *core.Options) { settings(o).description = description }
}

// WithUserIdentifier sets the merchant side payer identifier sent with every
// purchase. Jibit uses it to remember the payer's cards between orders.
func WithUserIdentifier(identifier string) core.Option {
	return func(o *core.Options) { settings(o).userIdentifier = identifier }
}

// WithPayerCardMatching sends [core.PurchaseRequest.AllowedCards] as Jibit's
// payerCardNumber, so a payment made with a different card is rejected.
func WithPayerCardMatching(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).matchPayerCard = enabled }
}

// WithCancellableRefunds marks refunds as cancellable, which holds the
// transfer until it is verified instead of sending it straight away.
func WithCancellableRefunds(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).refundCancellable = enabled }
}

// WithAdditionalData attaches a key/value pair to every purchase.
func WithAdditionalData(key, value string) core.Option {
	return func(o *core.Options) {
		c := settings(o)
		if c.additionalData == nil {
			c.additionalData = map[string]string{}
		}
		c.additionalData[key] = value
	}
}
