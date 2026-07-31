package torobpay

import "encoding/json"

// envelope is the wrapper TorobPay puts around every answer. Failures are
// reported in one of two shapes depending on the endpoint, so both are read.
type envelope struct {
	Successful bool       `json:"successful"`
	Error      *errorData `json:"error"`
	Result     *errorData `json:"result"`
	ErrorData  *errorData `json:"errorData"`
}

// errorData is the failure half of an envelope.
type errorData struct {
	Code    json.Number `json:"code"`
	Message string      `json:"message"`
}

// failure returns the first populated error of an envelope.
func (e envelope) failure() *errorData {
	for _, candidate := range []*errorData{e.Error, e.ErrorData, e.Result} {
		if candidate != nil && (candidate.Message != "" || candidate.Code.String() != "") {
			return candidate
		}
	}
	return nil
}

// code returns the provider error code as a string, or "".
func (e *errorData) code() string {
	if e == nil {
		return ""
	}
	return e.Code.String()
}

// message returns the provider error message, or fallback when there is none.
func (e *errorData) message(fallback string) string {
	if e == nil || e.Message == "" {
		return fallback
	}
	return e.Message
}

// oauthResponse is the answer of the token endpoint.
type oauthResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

// tokenRequest is the body of the payment token endpoint.
type tokenRequest struct {
	Amount               int64  `json:"amount"`
	Mobile               string `json:"mobile,omitempty"`
	PaymentMethodTypeDto string `json:"paymentMethodTypeDto"`
	TransactionID        string `json:"transactionId"`
	ReturnURL            string `json:"returnURL"`
	CartList             []Cart `json:"cartList"`
}

// tokenResponse is the answer of the payment token endpoint.
type tokenResponse struct {
	envelope
	Response tokenData `json:"response"`
}

// tokenData carries the payment token and the page the payer is sent to.
type tokenData struct {
	PaymentToken   string `json:"paymentToken"`
	PaymentPageURL string `json:"paymentPageUrl"`
}

// paymentTokenRequest is the body every follow-up call shares.
type paymentTokenRequest struct {
	PaymentToken string `json:"paymentToken"`
}

// verifyResponse is the answer of the verify, revert and cancel endpoints.
type verifyResponse struct {
	envelope
	Response verifyData `json:"response"`
}

// verifyData is what TorobPay reports about a completed payment.
type verifyData struct {
	TransactionID   string `json:"transactionId"`
	Amount          int64  `json:"amount"`
	Status          string `json:"status"`
	ReferenceNumber string `json:"referenceNumber"`
}

// statusResponse is the answer of the status endpoint.
type statusResponse struct {
	envelope
	Response statusData `json:"response"`
}

// statusData is the life cycle state of a payment.
type statusData struct {
	Status        string `json:"status"`
	TransactionID string `json:"transactionId"`
	Amount        int64  `json:"amount"`
}
