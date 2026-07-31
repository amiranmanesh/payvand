package payping

// payRequest is the body of POST /v2/pay. Amounts are in Toman.
type payRequest struct {
	Amount        int64  `json:"amount"`
	ReturnURL     string `json:"returnUrl"`
	PayerName     string `json:"payerName,omitempty"`
	PayerIdentity string `json:"payerIdentity,omitempty"`
	Description   string `json:"description,omitempty"`
	ClientRefID   string `json:"clientRefId,omitempty"`
}

// payResponse is the answer of the pay endpoint: "code" is the payment token.
type payResponse struct {
	Code string `json:"code"`
}

// verifyRequest is the body of POST /v2/pay/verify.
type verifyRequest struct {
	RefID  string `json:"refId"`
	Amount int64  `json:"amount"`
}

// verifyResponse is the answer of the verify endpoint.
type verifyResponse struct {
	Amount      int64  `json:"amount"`
	CardNumber  string `json:"cardNumber"`
	CardHashPan string `json:"cardHashPan"`
	PayerName   string `json:"payerName"`
	ClientRefID string `json:"clientRefId"`
}
