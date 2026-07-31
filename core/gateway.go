package core

import (
	"context"
	"net/http"
)

// Name is the canonical identifier of a payment gateway, e.g. "zarinpal".
// It is what [Register] and [New] key on and what appears in [Error].
type Name string

// String returns the gateway name as a plain string.
func (n Name) String() string { return string(n) }

// Gateway is the single abstraction every payment provider implements: the
// strategy interface of the package. Switching provider is switching the value
// behind this interface, never the call sites.
//
// Operations a provider does not offer return an error wrapping
// [ErrNotSupported]; ask [Gateway.Capabilities] beforehand to avoid the round
// trip.
type Gateway interface {
	// Name returns the canonical gateway name.
	Name() Name

	// Capabilities reports which optional operations this gateway supports.
	Capabilities() Capabilities

	// Purchase creates a new payment on the provider side and returns the
	// token plus everything needed to send the payer to the bank page.
	Purchase(ctx context.Context, req PurchaseRequest) (PurchaseResponse, error)

	// Verify settles a payment the payer has just completed. It must be
	// called after the callback: most Iranian gateways roll a transaction
	// back automatically when it is never verified.
	Verify(ctx context.Context, req VerifyRequest) (VerifyResponse, error)

	// Refund reverses an already verified payment, fully or partially
	// depending on the provider.
	Refund(ctx context.Context, req RefundRequest) (RefundResponse, error)

	// Inquiry asks the provider for the current state of a transaction. It is
	// the recovery path when a callback is lost.
	Inquiry(ctx context.Context, req InquiryRequest) (InquiryResponse, error)

	// ParseCallback extracts the provider's callback parameters out of the
	// incoming HTTP request into a provider independent [Callback].
	ParseCallback(r *http.Request) (Callback, error)
}

// Capabilities describes what a gateway can do, so callers can build a generic
// UI or admin panel without special-casing providers.
type Capabilities struct {
	// Verify reports whether the gateway supports payment verification.
	// Every real gateway does; the field exists for completeness.
	Verify bool
	// Refund reports whether the gateway can reverse a payment.
	Refund bool
	// Inquiry reports whether the gateway can be asked for a transaction
	// status out of band.
	Inquiry bool
	// Callback reports whether the gateway calls back into the merchant
	// server (false for in-app flows such as TOP).
	Callback bool
	// RedirectMethod is the HTTP method the payer's browser must use to reach
	// the bank page: http.MethodGet or http.MethodPost.
	RedirectMethod string
	// Multiplexing reports whether the gateway can split a single payment
	// between several IBANs (settlement sharing).
	Multiplexing bool
	// Currencies lists the units the provider's own API accepts. Payvand
	// converts to the first entry automatically.
	Currencies []Currency
}

// Config carries the merchant credentials of a single terminal. Not every
// gateway uses every field; the README documents the mapping per gateway and
// each implementation validates what it needs at construction time.
type Config struct {
	// MerchantID is the merchant/business identifier (Sadad MerchantId,
	// Vandar business id, Pasargad MerchantCode, ...).
	MerchantID string
	// TerminalID is the terminal/acceptor identifier issued by the PSP.
	TerminalID string
	// Username is the terminal user name (Mellat, IranKish acceptor id,
	// AsanPardakht usr).
	Username string
	// Password is the terminal password (Mellat, IranKish, AsanPardakht pwd).
	Password string
	// MerchantKey is the API key, token or private key of the terminal
	// (Zarinpal merchant_id, Zibal merchant, Pay.ir api, PayPing token,
	// Pasargad RSA private key in PEM, ...).
	MerchantKey string
	// IBAN is the settlement account used by multiplexing capable gateways.
	IBAN string
	// Extra carries provider specific credentials that do not fit above.
	Extra map[string]string
}

// Get returns the Extra entry for key, or "" when it is absent.
func (c Config) Get(key string) string {
	if c.Extra == nil {
		return ""
	}
	return c.Extra[key]
}
