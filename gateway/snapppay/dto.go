package snapppay

import "encoding/json"

// envelope is the wrapper SnappPay puts around every answer: a success flag
// plus, on failure, the provider's own error code and message.
type envelope struct {
	Successful bool       `json:"successful"`
	ErrorData  *errorData `json:"errorData"`
}

// errorData is the failure half of an envelope.
type errorData struct {
	ErrorCode json.Number `json:"errorCode"`
	Message   string      `json:"message"`
}

// code returns the provider error code as a string, or "".
func (e *errorData) code() string {
	if e == nil {
		return ""
	}
	return e.ErrorCode.String()
}

// message returns the provider error message, or fallback when there is none.
func (e *errorData) message(fallback string) string {
	if e == nil || e.Message == "" {
		return fallback
	}
	return e.Message
}

// oauthResponse is the answer of the token endpoint. It is a plain OAuth 2
// password grant response, without the envelope.
type oauthResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

// eligibleResponse is the answer of the eligibility endpoint.
type eligibleResponse struct {
	envelope
	Response eligibleData `json:"response"`
}

// eligibleData reports whether the payer may pay in instalments and what the
// instalment plan would look like.
type eligibleData struct {
	Eligible          bool         `json:"eligible"`
	Title             string       `json:"title"`
	Description       string       `json:"description"`
	TitleWithAmount   string       `json:"titleWithAmount"`
	EligibleAmount    int64        `json:"eligibleAmount"`
	MinEligibleAmount int64        `json:"minEligibleAmount"`
	MaxEligibleAmount int64        `json:"maxEligibleAmount"`
	Plans             []instalment `json:"installmentList"`
}

// instalment is one repayment plan offered to the payer.
type instalment struct {
	Count  int   `json:"count"`
	Amount int64 `json:"amount"`
	Wage   int64 `json:"wage"`
}

// tokenRequest is the body of the payment token endpoint.
type tokenRequest struct {
	Amount               int64  `json:"amount"`
	Mobile               string `json:"mobile,omitempty"`
	PaymentMethodTypeDto string `json:"paymentMethodTypeDto"`
	TransactionID        string `json:"transactionId"`
	ReturnURL            string `json:"returnURL"`
	DiscountAmount       int64  `json:"discountAmount,omitempty"`
	ExternalSourceAmount int64  `json:"externalSourceAmount"`
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

// verifyResponse is the answer of the verify, settle and revert endpoints.
type verifyResponse struct {
	envelope
	Response verifyData `json:"response"`
}

// verifyData is what SnappPay reports about a completed payment.
type verifyData struct {
	TransactionID   string `json:"transactionId"`
	Amount          int64  `json:"amount"`
	OriginalAmount  int64  `json:"originalAmount"`
	Status          string `json:"status"`
	ReferenceNumber string `json:"referenceNumber"`
	ElectronicCode  string `json:"electronicCode"`
	InstallmentType string `json:"installmentType"`
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

// updateRequest is the body of the order update endpoint.
type updateRequest struct {
	Amount               int64  `json:"amount"`
	PaymentMethodTypeDto string `json:"paymentMethodTypeDto"`
	PaymentToken         string `json:"paymentToken"`
	DiscountAmount       int64  `json:"discountAmount,omitempty"`
	ExternalSourceAmount int64  `json:"externalSourceAmount"`
	CartList             []Cart `json:"cartList"`
}
