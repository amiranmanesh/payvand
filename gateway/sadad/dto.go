package sadad

// paymentRequest is the body of POST /vpg/api/v0/Request/PaymentRequest.
type paymentRequest struct {
	TerminalID      string `json:"TerminalId"`
	MerchantID      string `json:"MerchantId"`
	Amount          int64  `json:"Amount"`
	SignData        string `json:"SignData"`
	ReturnURL       string `json:"ReturnUrl"`
	LocalDateTime   string `json:"LocalDateTime"`
	OrderID         string `json:"OrderId"`
	UserID          string `json:"UserId,omitempty"`
	ApplicationName string `json:"ApplicationName,omitempty"`
	AdditionalData  string `json:"AdditionalData,omitempty"`
}

// paymentResponse is the answer of the payment request endpoint.
type paymentResponse struct {
	ResCode     any    `json:"ResCode"`
	Token       string `json:"Token"`
	Description string `json:"Description"`
}

// verifyRequestBody is the body of POST /vpg/api/v0/Advice/Verify.
type verifyRequestBody struct {
	Token    string `json:"Token"`
	SignData string `json:"SignData"`
}

// verifyResponseBody is the answer of the verification endpoint.
type verifyResponseBody struct {
	ResCode          any    `json:"ResCode"`
	Description      string `json:"Description"`
	SystemTraceNo    string `json:"SystemTraceNo"`
	OrderID          int64  `json:"OrderId"`
	Amount           int64  `json:"Amount"`
	RetrivalRefNo    string `json:"RetrivalRefNo"`
	TransactionDate  string `json:"TransactionDate"`
	CardNumberMasked string `json:"CardNumberMasked"`
}
