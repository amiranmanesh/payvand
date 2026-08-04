package payping

import "encoding/json"

// Every amount in this file is a Toman value: the PayPing API specification
// states that the unit of all of its services is Toman
// ("واحد پول در تمام سرویس‌ها تومان می‌باشد").

// payRequest is the body of POST /v3/pay and of POST /v3/pay/shared. A shared
// payment carries no total: the shares hold the amounts, and PayPing derives
// the total from them.
type payRequest struct {
	Amount        int64   `json:"amount,omitempty"`
	ReturnURL     string  `json:"returnUrl"`
	PayerName     string  `json:"payerName,omitempty"`
	PayerIdentity string  `json:"payerIdentity,omitempty"`
	Description   string  `json:"description,omitempty"`
	ClientRefID   string  `json:"clientRefId,omitempty"`
	IsReversible  bool    `json:"isReversible,omitempty"`
	IsBlocked     bool    `json:"isBlocked,omitempty"`
	Items         []Share `json:"items,omitempty"`
}

// Share is one beneficiary of a shared (split settlement) payment.
type Share struct {
	// UserIdentity is the mobile number or email of the PayPing account the
	// share is settled to.
	UserIdentity string `json:"userIdentity"`
	// Amount is the share value in Toman.
	Amount int64 `json:"amount"`
	// Description is an optional free text kept with the share.
	Description string `json:"description,omitempty"`
}

// payResponse is the answer of the pay endpoints. GatewayAmount is what the
// payer is charged, which is above Amount when the terminal makes the payer
// carry the fee.
type payResponse struct {
	PaymentCode   string `json:"paymentCode"`
	URL           string `json:"url"`
	Amount        int64  `json:"amount"`
	PayerWage     int64  `json:"payerWage"`
	BusinessWage  int64  `json:"businessWage"`
	GatewayAmount int64  `json:"gatewayAmount"`
	PaypingVat    int64  `json:"paypingVat"`
}

// verifyRequest is the body of POST /v3/pay/verify. Unlike v2, which keyed on
// the reference alone, v3 requires the payment code as well.
type verifyRequest struct {
	PaymentRefID int64  `json:"paymentRefId"`
	PaymentCode  string `json:"paymentCode"`
	Amount       int64  `json:"amount"`
}

// verifyResponse is the answer of the verification endpoint.
type verifyResponse struct {
	Amount        int64  `json:"amount"`
	CardNumber    string `json:"cardNumber"`
	CardHashPan   string `json:"cardHashPan"`
	ClientRefID   string `json:"clientRefId"`
	PaymentRefID  int64  `json:"paymentRefId"`
	Code          string `json:"code"`
	PayedDate     string `json:"payedDate"`
	PayerWage     int64  `json:"payerWage"`
	BusinessWage  int64  `json:"businessWage"`
	GatewayAmount int64  `json:"gatewayAmount"`
	PaypingVat    int64  `json:"paypingVat"`
}

// reverseRequest is the body of POST /v3/pay/reverse. PayPing returns the whole
// payment to the card it came from, so there is no amount to send.
type reverseRequest struct {
	PaymentRefID int64  `json:"paymentRefId"`
	PaymentCode  string `json:"paymentCode"`
}

// reverseResponse is the answer of the reversal endpoint.
type reverseResponse struct {
	Amount        int64  `json:"amount"`
	ClientRefID   string `json:"clientRefId"`
	PaymentRefID  int64  `json:"paymentRefId"`
	Code          string `json:"code"`
	PayerWage     int64  `json:"payerWage"`
	GatewayAmount int64  `json:"gatewayAmount"`
	ReversedDate  string `json:"reversedDate"`
}

// reportResponse is the answer of GET /v1/report/{code}, the endpoint PayPing
// documents for reconciling a payment whose callback never arrived.
type reportResponse struct {
	Amount        int64  `json:"amount"`
	PayDate       string `json:"payDate"`
	ReqDate       string `json:"reqDate"`
	IsPaid        bool   `json:"isPaid"`
	Description   string `json:"description"`
	ClientRefID   string `json:"clientRefId"`
	RefID         string `json:"refId"`
	Wage          int64  `json:"wage"`
	PaymentStatus int    `json:"paymentStatus"`
}

// problemDetails is the RFC 7807 body every v3 endpoint reports an error with.
// The business reason lives in metaData.code, and the human readable text in
// metaData.errors.
type problemDetails struct {
	Title          string `json:"title"`
	Status         int    `json:"status"`
	Detail         string `json:"detail"`
	PaypingTraceID string `json:"paypingTraceId"`
	MetaData       struct {
		Code   int `json:"code"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	} `json:"metaData"`
}

// message returns the most descriptive text the body carries.
func (p problemDetails) message() string {
	for _, e := range p.MetaData.Errors {
		if e.Message != "" {
			return e.Message
		}
	}
	return firstNonEmpty(p.Detail, p.Title)
}

// conflictDetails is the 409 body. Its metaData.message is the payment itself,
// not a list of errors, which is how PayPing reports a repeated verification.
type conflictDetails struct {
	Status   int `json:"status"`
	MetaData struct {
		Code    int `json:"code"`
		Message struct {
			Amount       int64  `json:"amount"`
			CardNumber   string `json:"cardNumber"`
			CardHashPan  string `json:"cardHashPan"`
			ClientRefID  string `json:"clientRefId"`
			PaymentRefID int64  `json:"paymentRefId"`
			Code         string `json:"code"`
		} `json:"message"`
	} `json:"metaData"`
}

// callbackData is the JSON document v3 posts in the "data" field of the
// callback form. PaymentRefID is a number in the document but is read as a
// [json.Number] so a provider quoting it does not break the parse.
type callbackData struct {
	ClientRefID   string      `json:"clientRefId"`
	PaymentCode   string      `json:"paymentCode"`
	PaymentRefID  json.Number `json:"paymentRefId"`
	Amount        int64       `json:"amount"`
	GatewayAmount int64       `json:"gatewayAmount"`
	CardNumber    string      `json:"cardNumber"`
	CardHashPan   string      `json:"cardHashPan"`
}
