package core

import (
	"errors"
	"fmt"
)

// Sentinel errors returned by every gateway implementation. Compare them with
// [errors.Is]; gateway specific detail is carried by [Error], which wraps one
// of these.
var (
	// ErrNotSupported is returned by an operation the gateway does not
	// implement, e.g. Refund on Zarinpal.
	ErrNotSupported = errors.New("payvand: operation not supported by this gateway")
	// ErrGatewayNotRegistered is returned by [New] for an unknown gateway name.
	ErrGatewayNotRegistered = errors.New("payvand: gateway is not registered")
	// ErrInvalidConfig is returned when mandatory credentials are missing.
	ErrInvalidConfig = errors.New("payvand: invalid gateway configuration")
	// ErrInvalidRequest is returned when the caller supplied an unusable
	// request (zero amount, missing callback URL, malformed order id, ...).
	ErrInvalidRequest = errors.New("payvand: invalid request")
	// ErrPaymentFailed is returned when the gateway rejected the payment.
	ErrPaymentFailed = errors.New("payvand: payment failed")
	// ErrPaymentCanceled is returned when the payer aborted the payment.
	ErrPaymentCanceled = errors.New("payvand: payment canceled by payer")
	// ErrAlreadyVerified is returned when the transaction was verified before.
	ErrAlreadyVerified = errors.New("payvand: payment already verified")
	// ErrAmountMismatch is returned when the verified amount differs from the
	// amount the caller asked to verify.
	ErrAmountMismatch = errors.New("payvand: verified amount does not match requested amount")
	// ErrUnexpectedResponse is returned when the gateway answered with a body
	// that could not be understood.
	ErrUnexpectedResponse = errors.New("payvand: unexpected gateway response")
)

// Error is the rich error every gateway returns. It keeps the raw gateway code
// and message so the caller can log or map them, while still unwrapping to one
// of the sentinel errors above.
type Error struct {
	// Gateway is the gateway name, e.g. "zarinpal".
	Gateway Name
	// Op is the operation that failed: "purchase", "verify", "refund",
	// "inquiry" or "callback".
	Op string
	// Code is the gateway specific status/result code, if any.
	Code string
	// Message is the human readable message returned by the gateway.
	Message string
	// Err is the wrapped sentinel error.
	Err error
}

// Error implements the error interface.
func (e *Error) Error() string {
	msg := fmt.Sprintf("payvand: %s: %s failed", e.Gateway, e.Op)
	if e.Code != "" {
		msg += " (code " + e.Code + ")"
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	if e.Err != nil && e.Message == "" {
		msg += ": " + e.Err.Error()
	}
	return msg
}

// Unwrap exposes the wrapped sentinel error to [errors.Is] and [errors.As].
func (e *Error) Unwrap() error { return e.Err }

// NewError builds an [Error] for the given gateway and operation.
func NewError(gateway Name, op string, err error) *Error {
	return &Error{Gateway: gateway, Op: op, Err: err}
}

// WithCode returns a copy of the error carrying the gateway status code.
func (e *Error) WithCode(code string) *Error {
	clone := *e
	clone.Code = code
	return &clone
}

// WithMessage returns a copy of the error carrying the gateway message.
func (e *Error) WithMessage(message string) *Error {
	clone := *e
	clone.Message = message
	return &clone
}
