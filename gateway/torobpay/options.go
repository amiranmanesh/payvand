package torobpay

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// Cart is one basket sent with a payment. TorobPay lends against the goods, so
// it wants the basket contents; a purchase without one is refused.
//
// The field names follow TorobPay's own mixed spelling, which differs from the
// otherwise similar SnappPay contract.
type Cart struct {
	// CartID identifies the basket inside the order.
	CartID string `json:"cartId"`
	// TotalAmount is the basket total in Rial.
	TotalAmount int64 `json:"totalAmount"`
	// TaxAmount is the tax in Rial.
	TaxAmount int64 `json:"tax_amount"`
	// ShippingAmount is the delivery cost in Rial.
	ShippingAmount int64 `json:"shipping_amount"`
	// IsTaxIncluded reports whether TaxAmount is already part of TotalAmount.
	IsTaxIncluded bool `json:"is_tax_included"`
	// IsShipmentIncluded reports whether ShippingAmount is already part of
	// TotalAmount.
	IsShipmentIncluded bool `json:"is_shipment_included"`
	// CartItems are the lines of the basket.
	CartItems []CartItem `json:"cartItems"`
}

// CartItem is one line of a [Cart].
type CartItem struct {
	// ID is the merchant side product identifier.
	ID string `json:"id"`
	// Name is the product name shown to the payer.
	Name string `json:"name"`
	// Count is the quantity.
	Count int `json:"count"`
	// Amount is the unit price in Rial.
	Amount int64 `json:"amount"`
	// Category is the merchant's product category.
	Category string `json:"category"`
	// CommissionType is the commission group agreed with TorobPay. The field
	// keeps the provider's own spelling.
	CommissionType int `json:"comission_type,omitempty"`
}

// CartBuilder turns a purchase request into the baskets sent to TorobPay. Set
// one with [WithCartBuilder] when the real basket lines matter; the default
// builder sends a single line covering the whole order.
type CartBuilder func(req core.PurchaseRequest) []Cart

// config holds the TorobPay specific options.
type config struct {
	// paymentMethod is the TorobPay payment method type. It defaults to
	// ONLINE_CREDIT, the four-instalment product.
	paymentMethod string
	// cartBuilder produces the baskets of a purchase.
	cartBuilder CartBuilder
	// defaultCategory is used by the default cart builder.
	defaultCategory string
	// settle asks for the separate settlement call after a verification.
	settle bool
}

// settings returns the TorobPay option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithPaymentMethod overrides the TorobPay payment method type, for contracts
// that sell a product other than the default ONLINE_CREDIT one.
func WithPaymentMethod(method string) core.Option {
	return func(o *core.Options) { settings(o).paymentMethod = method }
}

// WithCartBuilder installs the function that turns a purchase request into the
// baskets TorobPay is shown.
func WithCartBuilder(builder CartBuilder) core.Option {
	return func(o *core.Options) { settings(o).cartBuilder = builder }
}

// WithCart sends a fixed basket with every purchase.
func WithCart(carts ...Cart) core.Option {
	return func(o *core.Options) {
		settings(o).cartBuilder = func(core.PurchaseRequest) []Cart { return carts }
	}
}

// WithDefaultCategory sets the product category the default cart builder uses.
func WithDefaultCategory(category string) core.Option {
	return func(o *core.Options) { settings(o).defaultCategory = category }
}

// WithSettle makes [core.Gateway.Verify] follow the verification with the
// settlement call.
//
// It is off by default because this package has always treated verification as
// the end of the TorobPay flow. Turn it on if your contract says otherwise —
// SnappPay, which serves the same endpoint paths, reverts any payment that is
// verified and never settled, and a payment lost that way is not recoverable
// from the merchant's side. Ask TorobPay which of the two your terminal
// follows rather than guessing from the behaviour of a test payment, since the
// reversal only shows up once the settlement window closes.
func WithSettle(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).settle = enabled }
}
