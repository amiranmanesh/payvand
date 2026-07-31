package core

import (
	"time"
)

// PurchaseRequest is the provider independent input of [Gateway.Purchase].
type PurchaseRequest struct {
	// Amount is the amount to charge, in the unit the caller chose. It is
	// converted to whatever the provider expects.
	Amount Money
	// OrderID is the merchant side unique identifier of the order. Several
	// gateways (Sadad, Parsian, Mellat, TOP, Sepehr) require it and some of
	// them require it to be numeric; it is also the key used to reconcile a
	// lost callback through [Gateway.Inquiry].
	OrderID string
	// CallbackURL is the absolute URL the bank returns the payer to.
	CallbackURL string
	// Description is a free text shown on the bank page or the receipt.
	Description string
	// Mobile is the payer's mobile number in "09xxxxxxxxx" form. Gateways use
	// it to pre-fill the saved-card list.
	Mobile string
	// Email is the payer's email address, used by the few gateways that send
	// receipts.
	Email string
	// NationalID is the payer's national code, used by gateways that match it
	// against the card holder.
	NationalID string
	// PayerName is the payer's full name, when the provider accepts it.
	PayerName string
	// AllowedCards restricts the payment to these PANs, when supported.
	AllowedCards []string
	// Metadata carries additional provider specific key/value pairs.
	Metadata map[string]string
}

// PurchaseResponse is the provider independent output of [Gateway.Purchase].
type PurchaseResponse struct {
	// Token is the payment token/authority/reference issued by the provider.
	// It is the value to persist next to the order.
	Token string
	// OrderID echoes the order id the payment was created for.
	OrderID string
	// Redirect describes how to send the payer to the bank page.
	Redirect Redirect
	// Amount is the amount the payment was created for.
	Amount Money
	// Extra carries provider specific values that must be persisted next to
	// the order because verification needs them again, such as the Pasargad
	// invoice date.
	Extra map[string]string
	// Raw is the untouched provider response body, for logging and support.
	Raw string
}

// Get returns the Extra entry for key, or "" when it is absent.
func (r PurchaseResponse) Get(key string) string {
	if r.Extra == nil {
		return ""
	}
	return r.Extra[key]
}

// VerifyRequest is the provider independent input of [Gateway.Verify]. Fill it
// from the persisted payment plus the parsed [Callback].
type VerifyRequest struct {
	// Token is the payment token returned by [Gateway.Purchase].
	Token string
	// OrderID is the merchant side order identifier.
	OrderID string
	// Amount is the amount that was requested. Gateways that do not echo the
	// amount reuse this value, and those that do compare against it.
	Amount Money
	// ReferenceNumber is the bank reference (RRN) coming from the callback,
	// required by IranKish, Mellat and Saman.
	ReferenceNumber string
	// TraceNumber is the system trace audit number coming from the callback,
	// required by IranKish and Mellat.
	TraceNumber string
	// CardNumber is the masked PAN coming from the callback, echoed back by
	// gateways that do not return it on verification.
	CardNumber string
	// Extra carries provider specific callback fields (Sepehr digital
	// receipt, AsanPardakht payGateTranId, ...).
	Extra map[string]string
}

// Get returns the Extra entry for key, or "" when it is absent.
func (r VerifyRequest) Get(key string) string {
	if r.Extra == nil {
		return ""
	}
	return r.Extra[key]
}

// VerifyResponse is the provider independent output of [Gateway.Verify]. A nil
// error means the money is settled to the merchant.
type VerifyResponse struct {
	// ReferenceNumber is the final bank reference number (RRN) to show to the
	// payer and to keep for reconciliation.
	ReferenceNumber string
	// TransactionID is the provider side transaction identifier, when it
	// differs from the reference number (used by refunds).
	TransactionID string
	// OrderID echoes the merchant side order identifier.
	OrderID string
	// CardNumber is the masked PAN of the card that paid.
	CardNumber string
	// CardHash is the hashed PAN, when the provider returns one.
	CardHash string
	// Amount is the settled amount as reported by the provider.
	Amount Money
	// Fee is the provider fee deducted from the amount, in Rial, when known.
	Fee int64
	// PaidAt is the settlement time reported by the provider, when known.
	PaidAt time.Time
	// Raw is the untouched provider response body.
	Raw string
}

// RefundRequest is the provider independent input of [Gateway.Refund].
type RefundRequest struct {
	// Token is the payment token of the transaction to refund.
	Token string
	// OrderID is the merchant side order identifier.
	OrderID string
	// TransactionID is the provider transaction identifier returned by
	// [Gateway.Verify].
	TransactionID string
	// ReferenceNumber is the bank reference number of the transaction.
	ReferenceNumber string
	// TraceNumber is the system trace audit number of the transaction.
	TraceNumber string
	// Amount is the amount to refund. Leave it zero for a full refund on
	// gateways that support partial refunds.
	Amount Money
	// Reason is an optional free text kept by some providers.
	Reason string
	// Extra carries provider specific fields.
	Extra map[string]string
}

// Get returns the Extra entry for key, or "" when it is absent.
func (r RefundRequest) Get(key string) string {
	if r.Extra == nil {
		return ""
	}
	return r.Extra[key]
}

// RefundResponse is the provider independent output of [Gateway.Refund].
type RefundResponse struct {
	// RefundID is the provider identifier of the refund, when one is issued.
	RefundID string
	// Amount is the refunded amount as reported by the provider.
	Amount Money
	// Raw is the untouched provider response body.
	Raw string
}

// InquiryRequest is the provider independent input of [Gateway.Inquiry].
type InquiryRequest struct {
	// Token is the payment token of the transaction to inspect.
	Token string
	// OrderID is the merchant side order identifier.
	OrderID string
	// ReferenceNumber is the bank reference number, when the provider keys on
	// it instead of the token.
	ReferenceNumber string
	// TraceNumber is the system trace audit number, when required.
	TraceNumber string
	// Amount is the expected amount, required by the few providers that use
	// it as part of the lookup key.
	Amount Money
	// Extra carries provider specific fields.
	Extra map[string]string
}

// Get returns the Extra entry for key, or "" when it is absent.
func (r InquiryRequest) Get(key string) string {
	if r.Extra == nil {
		return ""
	}
	return r.Extra[key]
}

// InquiryResponse is the provider independent output of [Gateway.Inquiry].
type InquiryResponse struct {
	// Status is the normalised transaction status.
	Status Status
	// ReferenceNumber is the bank reference number, when the transaction got
	// that far.
	ReferenceNumber string
	// CardNumber is the masked PAN, when known.
	CardNumber string
	// Amount is the transaction amount as reported by the provider.
	Amount Money
	// PaidAt is the payment time reported by the provider, when known.
	PaidAt time.Time
	// Message is the provider's own description of the status.
	Message string
	// Raw is the untouched provider response body.
	Raw string
}

// Status is the normalised life cycle state of a transaction.
type Status uint8

const (
	// StatusUnknown means the provider did not report a state Payvand can map.
	StatusUnknown Status = iota
	// StatusPending means the payer has not finished the payment yet.
	StatusPending
	// StatusPaid means the money was taken from the payer but the transaction
	// is not verified/settled yet.
	StatusPaid
	// StatusVerified means the transaction is verified and settled.
	StatusVerified
	// StatusFailed means the transaction failed or was rejected.
	StatusFailed
	// StatusCanceled means the payer aborted the payment.
	StatusCanceled
	// StatusRefunded means the transaction was refunded or reversed.
	StatusRefunded
)

// String returns a lower case label for the status.
func (s Status) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusPaid:
		return "paid"
	case StatusVerified:
		return "verified"
	case StatusFailed:
		return "failed"
	case StatusCanceled:
		return "canceled"
	case StatusRefunded:
		return "refunded"
	default:
		return "unknown"
	}
}
