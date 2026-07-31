package vandar

// sendRequest is the body of POST /api/v4/send.
type sendRequest struct {
	APIKey          string   `json:"api_key"`
	Amount          int64    `json:"amount"`
	CallbackURL     string   `json:"callback_url"`
	MobileNumber    string   `json:"mobile_number,omitempty"`
	FactorNumber    string   `json:"factorNumber,omitempty"`
	Description     string   `json:"description,omitempty"`
	NationalCode    string   `json:"national_code,omitempty"`
	ValidCardNumber []string `json:"valid_card_number,omitempty"`
	Port            string   `json:"port,omitempty"`
	Comment         string   `json:"comment,omitempty"`
}

// sendResponse is the answer of the send endpoint.
type sendResponse struct {
	Status int      `json:"status"`
	Token  string   `json:"token"`
	Errors []string `json:"errors"`
}

// tokenRequest is the body shared by the transaction and verify endpoints.
type tokenRequest struct {
	APIKey string `json:"api_key"`
	Token  string `json:"token"`
}

// transactionResponse is the answer of POST /api/v4/transaction.
type transactionResponse struct {
	Status       int      `json:"status"`
	Amount       string   `json:"amount"`
	TransID      int64    `json:"transId"`
	RefNumber    string   `json:"refnumber"`
	TrackingCode string   `json:"trackingCode"`
	FactorNumber *string  `json:"factorNumber"`
	Mobile       *string  `json:"mobile"`
	Description  *string  `json:"description"`
	CardNumber   *string  `json:"cardNumber"`
	CID          *string  `json:"CID"`
	PaymentDate  *string  `json:"paymentDate"`
	Message      string   `json:"message"`
	Errors       []string `json:"errors"`
}

// verifyResponse is the answer of POST /api/v4/verify.
type verifyResponse struct {
	Status       int      `json:"status"`
	Amount       string   `json:"amount"`
	RealAmount   *int64   `json:"realAmount"`
	Wage         *int64   `json:"wage"`
	TransID      int64    `json:"transId"`
	FactorNumber *string  `json:"factorNumber"`
	Mobile       *string  `json:"mobile"`
	Description  *string  `json:"description"`
	CardNumber   *string  `json:"cardNumber"`
	PaymentDate  *string  `json:"paymentDate"`
	CID          *string  `json:"cid"`
	Message      string   `json:"message"`
	Errors       []string `json:"errors"`
}

// refundRequest is the body of the business refund endpoint.
type refundRequest struct {
	APIKey        string `json:"api_key"`
	TransactionID string `json:"transaction_id"`
	Amount        int64  `json:"amount,omitempty"`
	Comment       string `json:"comment,omitempty"`
}

// refundResponse is the answer of the business refund endpoint.
type refundResponse struct {
	Status  int      `json:"status"`
	Message string   `json:"message"`
	Errors  []string `json:"errors"`
}

// firstError returns the first message of a Vandar error list.
func firstError(errs []string, fallback string) string {
	if len(errs) > 0 {
		return errs[0]
	}
	return fallback
}

// deref returns the value behind a pointer, or "" when it is nil.
func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
