package core

import (
	"context"
	"net/http"
)

// Unsupported implements the optional half of [Gateway] with "not supported"
// answers. A gateway embeds it and overrides only the operations its provider
// actually offers, which keeps every implementation free of boilerplate while
// still satisfying the single interface callers program against.
type Unsupported struct {
	// GatewayName is reported in the returned errors.
	GatewayName Name
}

// Refund reports that the provider has no refund API.
func (u Unsupported) Refund(context.Context, RefundRequest) (RefundResponse, error) {
	return RefundResponse{}, NewError(u.GatewayName, "refund", ErrNotSupported)
}

// Inquiry reports that the provider has no transaction status API.
func (u Unsupported) Inquiry(context.Context, InquiryRequest) (InquiryResponse, error) {
	return InquiryResponse{}, NewError(u.GatewayName, "inquiry", ErrNotSupported)
}

// ParseCallback reports that the provider does not call back into the merchant
// server, e.g. in-app gateways that report the result to the mobile client.
func (u Unsupported) ParseCallback(*http.Request) (Callback, error) {
	return Callback{}, NewError(u.GatewayName, "callback", ErrNotSupported)
}
