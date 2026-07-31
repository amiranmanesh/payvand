package jibit

// tokenRequest is the body of POST /v3/tokens.
type tokenRequest struct {
	APIKey    string `json:"apiKey"`
	SecretKey string `json:"secretKey"`
}

// tokenResponse is the answer of the token endpoints.
type tokenResponse struct {
	AccessToken  string         `json:"accessToken"`
	RefreshToken string         `json:"refreshToken"`
	Errors       []serviceError `json:"errors"`
}

// purchaseRequest is the body of POST /v3/purchases.
type purchaseRequest struct {
	Amount                int64             `json:"amount"`
	Wage                  int64             `json:"wage,omitempty"`
	Currency              string            `json:"currency,omitempty"`
	CallbackURL           string            `json:"callbackUrl"`
	ClientReferenceNumber string            `json:"clientReferenceNumber"`
	PayerCardNumber       string            `json:"payerCardNumber,omitempty"`
	PayerMobileNumber     string            `json:"payerMobileNumber,omitempty"`
	PayerNationalCode     string            `json:"payerNationalCode,omitempty"`
	UserIdentifier        string            `json:"userIdentifier,omitempty"`
	Description           string            `json:"description,omitempty"`
	AdditionalData        map[string]string `json:"additionalData,omitempty"`
}

// purchaseResponse is the answer of the purchase endpoint.
type purchaseResponse struct {
	PurchaseID            int64          `json:"purchaseId"`
	PurchaseIDStr         string         `json:"purchaseIdStr"`
	PSPSwitchingURL       string         `json:"pspSwitchingUrl"`
	ClientReferenceNumber string         `json:"clientReferenceNumber"`
	Errors                []serviceError `json:"errors"`
}

// verifyResponse is the answer of POST /v3/purchases/{id}/verify. Jibit puts
// the outcome in a status string rather than in the HTTP status code.
type verifyResponse struct {
	Status string         `json:"status"`
	Errors []serviceError `json:"errors"`
}

// refundRequest is the body of POST /v3/purchases/refund. Exactly one of the
// two identifiers is sent.
type refundRequest struct {
	PurchaseID            int64  `json:"purchaseId,omitempty"`
	ClientReferenceNumber string `json:"clientReferenceNumber,omitempty"`
	Amount                int64  `json:"amount,omitempty"`
	Cancellable           bool   `json:"cancellable,omitempty"`
}

// refundResponse is the answer of the refund endpoint.
type refundResponse struct {
	RefundID           string         `json:"refundId"`
	BatchID            string         `json:"batchId"`
	TransferID         string         `json:"transferId"`
	PartialRefundIndex int            `json:"partialRefundIndex"`
	Errors             []serviceError `json:"errors"`
}

// reverseRequest is the body of POST /v3/purchases/reverse.
type reverseRequest struct {
	PurchaseID            int64  `json:"purchaseId,omitempty"`
	ClientReferenceNumber string `json:"clientReferenceNumber,omitempty"`
}

// reverseResponse is the answer of the reverse endpoint.
type reverseResponse struct {
	Status string         `json:"status"`
	Errors []serviceError `json:"errors"`
}

// listResponse is the answer of GET /v3/purchases, used for inquiries.
type listResponse struct {
	Elements []purchaseElement `json:"elements"`
	Errors   []serviceError    `json:"errors"`
}

// purchaseElement is one row of the purchase list.
type purchaseElement struct {
	PurchaseID            int64  `json:"purchaseId"`
	State                 string `json:"state"`
	Amount                int64  `json:"amount"`
	Wage                  int64  `json:"wage"`
	Currency              string `json:"currency"`
	ClientReferenceNumber string `json:"clientReferenceNumber"`
	PSPName               string `json:"pspName"`
	PSPReferenceNumber    string `json:"pspReferenceNumber"`
	PSPRRN                string `json:"pspRrn"`
	PSPTraceNumber        string `json:"pspTraceNumber"`
	PayerCardNumber       string `json:"payerCardNumber"`
	PayerMaskedCardNumber string `json:"payerMaskedCardNumber"`
	CreatedAt             string `json:"createdAt"`
	UpdatedAt             string `json:"updatedAt"`
}

// serviceError is one entry of the Jibit error list. Jibit reports business
// failures with HTTP 4xx plus a machine readable code such as
// "purchase.not_found" or "amount.not_enough".
type serviceError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field"`
}

// first returns the first error of a Jibit error list, or the zero value.
func first(errs []serviceError) serviceError {
	if len(errs) == 0 {
		return serviceError{}
	}
	return errs[0]
}
