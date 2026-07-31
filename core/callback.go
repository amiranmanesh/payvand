package core

import (
	"net/http"
	"net/url"
)

// Callback is the provider independent view of the request the bank sends the
// payer back with. Feed it into a [VerifyRequest] to settle the payment.
type Callback struct {
	// Gateway is the gateway that produced the callback.
	Gateway Name
	// Succeeded reports whether the bank says the payer completed the payment.
	// It is a hint only: the payment is not final until [Gateway.Verify]
	// returns without error.
	Succeeded bool
	// Token is the payment token, matching [PurchaseResponse.Token].
	Token string
	// OrderID is the merchant side order identifier, when the bank echoes it.
	OrderID string
	// ReferenceNumber is the bank reference number (RRN), when present.
	ReferenceNumber string
	// TraceNumber is the system trace audit number, when present.
	TraceNumber string
	// CardNumber is the masked PAN, when present.
	CardNumber string
	// Amount is the amount reported by the bank, when present.
	Amount Money
	// Code is the raw status/result code sent by the bank.
	Code string
	// Message is the raw message sent by the bank.
	Message string
	// Values holds every parameter of the callback (query and form merged),
	// so provider specific fields stay reachable.
	Values map[string]string
}

// Get returns the raw callback parameter for key, or "" when it is absent.
func (c Callback) Get(key string) string {
	if c.Values == nil {
		return ""
	}
	return c.Values[key]
}

// VerifyRequest builds the verification request matching this callback. The
// caller must still set the amount from its own records, because trusting the
// amount reported by the browser is never safe.
func (c Callback) VerifyRequest(amount Money) VerifyRequest {
	return VerifyRequest{
		Token:           c.Token,
		OrderID:         c.OrderID,
		Amount:          amount,
		ReferenceNumber: c.ReferenceNumber,
		TraceNumber:     c.TraceNumber,
		CardNumber:      c.CardNumber,
		Extra:           c.Values,
	}
}

// CallbackValues merges the query string and the form body of an incoming
// callback into a flat map. It is the helper every gateway uses in
// [Gateway.ParseCallback]; the body is only parsed for methods that carry one.
func CallbackValues(r *http.Request) (map[string]string, error) {
	values := map[string]string{}
	if r == nil {
		return values, nil
	}

	collect := func(src url.Values) {
		for key, list := range src {
			if len(list) > 0 {
				values[key] = list[0]
			}
		}
	}

	if r.URL != nil {
		collect(r.URL.Query())
	}
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
		if err := r.ParseForm(); err != nil {
			return values, err
		}
		collect(r.PostForm)
	}
	return values, nil
}

// FirstValue returns the first non-empty value among the given keys, which
// keeps gateways tolerant to the casing drift of the Iranian PSP callbacks.
func FirstValue(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if v, ok := values[key]; ok && v != "" {
			return v
		}
	}
	return ""
}
