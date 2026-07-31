package pasargad

// tokenRequest is the body of POST /Api/v1/Payment/GetToken.
type tokenRequest struct {
	InvoiceNumber   string `json:"InvoiceNumber"`
	InvoiceDate     string `json:"InvoiceDate"`
	Amount          int64  `json:"Amount"`
	TerminalCode    string `json:"TerminalCode"`
	MerchantCode    string `json:"MerchantCode"`
	RedirectAddress string `json:"RedirectAddress"`
	Timestamp       string `json:"Timestamp"`
	Action          string `json:"Action"`
	Mobile          string `json:"Mobile,omitempty"`
	Email           string `json:"Email,omitempty"`
}

// tokenResponse is the answer of the token endpoint.
type tokenResponse struct {
	IsSuccess bool   `json:"IsSuccess"`
	Message   string `json:"Message"`
	Token     string `json:"Token"`
}

// checkRequest is the body of POST /Api/v1/Payment/CheckTransactionResult.
type checkRequest struct {
	TransactionReferenceID string `json:"TransactionReferenceID"`
	InvoiceNumber          string `json:"InvoiceNumber"`
	InvoiceDate            string `json:"InvoiceDate"`
	MerchantCode           string `json:"MerchantCode"`
	TerminalCode           string `json:"TerminalCode"`
}

// checkResponse is the answer of the transaction check endpoint.
type checkResponse struct {
	IsSuccess        bool   `json:"IsSuccess"`
	Message          string `json:"Message"`
	Action           string `json:"Action"`
	TraceNumber      int64  `json:"TraceNumber"`
	ReferenceNumber  int64  `json:"ReferenceNumber"`
	TransactionDate  string `json:"TransactionDate"`
	MaskedCardNumber string `json:"MaskedCardNumber"`
	HashedCardNumber string `json:"HashedCardNumber"`
	Amount           int64  `json:"Amount"`
	InvoiceNumber    string `json:"InvoiceNumber"`
}

// settlementRequest is the body of the verify and refund endpoints.
type settlementRequest struct {
	InvoiceNumber string `json:"InvoiceNumber"`
	InvoiceDate   string `json:"InvoiceDate"`
	Amount        int64  `json:"Amount"`
	TerminalCode  string `json:"TerminalCode"`
	MerchantCode  string `json:"MerchantCode"`
	Timestamp     string `json:"Timestamp"`
}

// settlementResponse is the answer of the verify and refund endpoints.
type settlementResponse struct {
	IsSuccess         bool   `json:"IsSuccess"`
	Message           string `json:"Message"`
	MaskedCardNumber  string `json:"MaskedCardNumber"`
	HashedCardNumber  string `json:"HashedCardNumber"`
	ShaparakRefNumber string `json:"ShaparakRefNumber"`
}
