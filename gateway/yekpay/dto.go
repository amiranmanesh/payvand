package yekpay

// requestBody is the body of POST /api/payment/request.
type requestBody struct {
	MerchantID       string `json:"merchantId"`
	Amount           int64  `json:"amount"`
	FromCurrencyCode int    `json:"fromCurrencyCode"`
	ToCurrencyCode   int    `json:"toCurrencyCode"`
	OrderNumber      string `json:"orderNumber"`
	Callback         string `json:"callback"`
	FirstName        string `json:"firstName,omitempty"`
	LastName         string `json:"lastName,omitempty"`
	Email            string `json:"email,omitempty"`
	Mobile           string `json:"mobile,omitempty"`
	Address          string `json:"address,omitempty"`
	Description      string `json:"description,omitempty"`
}

// requestResponse is the answer of the payment request endpoint.
type requestResponse struct {
	Code        int    `json:"Code"`
	Description string `json:"Description"`
	Authority   string `json:"Authority"`
}

// verifyBody is the body of POST /api/payment/verify.
type verifyBody struct {
	MerchantID string `json:"merchantId"`
	Authority  string `json:"authority"`
}

// verifyResponseBody is the answer of the verification endpoint.
type verifyResponseBody struct {
	Code        int    `json:"Code"`
	Description string `json:"Description"`
	Reference   string `json:"Reference"`
	Amount      int64  `json:"Amount"`
	OrderNumber string `json:"OrderNo"`
}
