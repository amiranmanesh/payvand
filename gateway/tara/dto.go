package tara

import "encoding/json"

// authenticateRequest is the body of POST /api/v2/authenticate.
type authenticateRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// authenticateResponse is the answer of the authentication endpoint.
type authenticateResponse struct {
	AccessToken string      `json:"accessToken"`
	ExpiresIn   json.Number `json:"expiresIn"`
	Result      json.Number `json:"result"`
	Description string      `json:"description"`
	Message     string      `json:"message"`
}

// tokenRequest is the body of POST /api/getToken.
type tokenRequest struct {
	AdditionalData      string          `json:"additionalData,omitempty"`
	Mobile              string          `json:"mobile"`
	CallBackURL         string          `json:"callBackUrl"`
	Amount              int64           `json:"amount"`
	VAT                 int64           `json:"vat"`
	ServiceAmountList   []serviceAmount `json:"serviceAmountList,omitempty"`
	TaraInvoiceItemList []InvoiceItem   `json:"taraInvoiceItemList"`
	IP                  string          `json:"ip,omitempty"`
}

// serviceAmount routes part of the amount to a service of the merchant.
type serviceAmount struct {
	ServiceID string `json:"serviceId"`
	Amount    int64  `json:"amount"`
}

// tokenResponse is the answer of the token endpoint.
type tokenResponse struct {
	Token       string      `json:"token"`
	Result      json.Number `json:"result"`
	Description string      `json:"description"`
	Message     string      `json:"message"`
}

// verifyRequest is the body of POST /api/purchaseVerify.
type verifyRequest struct {
	Token string `json:"token"`
	IP    string `json:"ip,omitempty"`
}

// verifyResponse is the answer of the verification endpoint. A result of zero
// means the payment is settled.
type verifyResponse struct {
	Result         json.Number `json:"result"`
	RRN            string      `json:"rrn"`
	Amount         json.Number `json:"amount"`
	CardNumber     string      `json:"cardNumber"`
	TraceNumber    string      `json:"traceNumber"`
	InvoiceDate    string      `json:"invoiceDate"`
	Description    string      `json:"description"`
	Message        string      `json:"message"`
	PaymentDetails string      `json:"paymentDetails"`
}

// succeeded reports whether a Tara result code means success.
func succeeded(result json.Number) bool {
	value := result.String()
	return value == "" || value == "0"
}

// describe returns the provider description, its message, or fallback.
func describe(description, message, fallback string) string {
	switch {
	case description != "":
		return description
	case message != "":
		return message
	default:
		return fallback
	}
}
