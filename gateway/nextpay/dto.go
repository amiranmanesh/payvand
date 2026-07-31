package nextpay

// tokenRequest is the body of POST /nx/gateway/token.
type tokenRequest struct {
	APIKey        string `json:"api_key"`
	OrderID       string `json:"order_id"`
	Amount        int64  `json:"amount"`
	CallbackURI   string `json:"callback_uri"`
	Currency      string `json:"currency,omitempty"`
	CustomerPhone string `json:"customer_phone,omitempty"`
	PayerName     string `json:"payer_name,omitempty"`
	PayerDesc     string `json:"payer_desc,omitempty"`
	AutoVerify    string `json:"auto_verify,omitempty"`
	AllowedCard   string `json:"allowed_card,omitempty"`
}

// tokenResponse is the answer of the token endpoint. NextPay reports success
// with code -1 here, and with code 0 everywhere else.
type tokenResponse struct {
	Code    int    `json:"code"`
	TransID string `json:"trans_id"`
}

// verifyRequest is the body of POST /nx/gateway/verify. The same endpoint
// performs refunds when RefundRequest is set.
type verifyRequest struct {
	APIKey        string `json:"api_key"`
	TransID       string `json:"trans_id"`
	Amount        int64  `json:"amount"`
	RefundRequest string `json:"refund_request,omitempty"`
}

// verifyResponse is the answer of the verify endpoint.
type verifyResponse struct {
	Code          int    `json:"code"`
	Amount        int64  `json:"amount"`
	OrderID       string `json:"order_id"`
	CardHolder    string `json:"card_holder"`
	ShaparakRefID string `json:"Shaparak_Ref_Id"`
	CustomerPhone string `json:"customer_phone"`
	CustomJSON    any    `json:"custom_json_fields"`
	ProcessStatus string `json:"process_status"`
	CreatedAt     string `json:"created_at"`
	VerifiedAt    string `json:"verified_at"`
}
