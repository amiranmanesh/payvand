package snapppay

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// Cart is one basket sent with a payment. SnappPay lends against the goods, so
// it wants to know what is being bought; a purchase without a cart is refused.
type Cart struct {
	// CartID identifies the basket inside the order.
	CartID int64 `json:"cartId"`
	// TotalAmount is the basket total in Rial.
	TotalAmount int64 `json:"totalAmount"`
	// ShippingAmount is the delivery cost in Rial.
	ShippingAmount int64 `json:"shippingAmount"`
	// TaxAmount is the tax in Rial.
	TaxAmount int64 `json:"taxAmount"`
	// IsTaxIncluded reports whether TaxAmount is already part of TotalAmount.
	IsTaxIncluded bool `json:"isTaxIncluded"`
	// IsShipmentIncluded reports whether ShippingAmount is already part of
	// TotalAmount.
	IsShipmentIncluded bool `json:"isShipmentIncluded"`
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
	// CommissionType is the commission group agreed with SnappPay, when the
	// contract uses per-category commissions.
	CommissionType int `json:"commissionType,omitempty"`
}

// CartBuilder turns a purchase request into the baskets sent to SnappPay. Set
// one with [WithCartBuilder] when the real basket lines matter; the default
// builder sends a single line covering the whole order.
type CartBuilder func(req core.PurchaseRequest) []Cart

// config holds the SnappPay specific options.
type config struct {
	// paymentMethod is the SnappPay payment method type. It defaults to
	// INSTALLMENT, the online instalment product.
	paymentMethod string
	// cartBuilder produces the baskets of a purchase.
	cartBuilder CartBuilder
	// defaultCategory is used by the default cart builder.
	defaultCategory string
	// manualSettle keeps the settle call out of Verify. It is stored inverted
	// so that the zero value settles automatically, which is what a normal
	// shop wants: an unsettled payment is reverted by SnappPay.
	manualSettle bool
	// scope is the OAuth scope requested with the password grant.
	scope string
}

// settings returns the SnappPay option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithPaymentMethod overrides the SnappPay payment method type, for contracts
// that sell a product other than the default INSTALLMENT one.
func WithPaymentMethod(method string) core.Option {
	return func(o *core.Options) { settings(o).paymentMethod = method }
}

// WithCartBuilder installs the function that turns a purchase request into the
// baskets SnappPay is shown. Use it to send the real basket lines instead of
// the single line the default builder produces.
func WithCartBuilder(builder CartBuilder) core.Option {
	return func(o *core.Options) { settings(o).cartBuilder = builder }
}

// WithCart sends a fixed basket with every purchase. It is the shortcut for
// shops that sell one thing, such as a wallet top-up.
func WithCart(carts ...Cart) core.Option {
	return func(o *core.Options) {
		settings(o).cartBuilder = func(core.PurchaseRequest) []Cart { return carts }
	}
}

// WithDefaultCategory sets the product category the default cart builder uses.
func WithDefaultCategory(category string) core.Option {
	return func(o *core.Options) { settings(o).defaultCategory = category }
}

// WithAutoSettle controls whether Verify also settles the payment. It is on by
// default; turn it off only when your own code calls [Gateway.Settle] later,
// because an unsettled payment is reverted by SnappPay.
func WithAutoSettle(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).manualSettle = !enabled }
}

// WithScope overrides the OAuth scope of the password grant.
func WithScope(scope string) core.Option {
	return func(o *core.Options) { settings(o).scope = scope }
}
