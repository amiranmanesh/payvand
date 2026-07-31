package asanpardakht

// tokenRequest is the body of POST /v1/Token.
type tokenRequest struct {
	ServiceTypeID           int          `json:"serviceTypeId"`
	MerchantConfigurationID string       `json:"merchantConfigurationId"`
	LocalInvoiceID          string       `json:"localInvoiceId"`
	AmountInRials           int64        `json:"amountInRials"`
	AdditionalData          string       `json:"additionalData,omitempty"`
	CallbackURL             string       `json:"callbackURL"`
	PaymentID               string       `json:"paymentId,omitempty"`
	Settlements             []Settlement `json:"settlementPortions,omitempty"`
}

// Settlement is one destination of an AsanPardakht split settlement.
type Settlement struct {
	// IBAN is the destination account.
	IBAN string `json:"iban"`
	// AmountInRials is the share value.
	AmountInRials int64 `json:"amountInRials"`
	// PaymentID identifies the share on the settlement report.
	PaymentID string `json:"paymentId,omitempty"`
}

// tranResult is the answer of GET /v1/TranResult.
type tranResult struct {
	EventType         int    `json:"eventType"`
	Amount            int64  `json:"amount"`
	RefID             string `json:"refID"`
	RRN               string `json:"rrn"`
	PayGateTranID     int64  `json:"payGateTranID"`
	PayGateTranDate   string `json:"payGateTranDate"`
	CardNumber        string `json:"cardNumber"`
	SalesOrderID      string `json:"salesOrderId"`
	ServiceStatusCode string `json:"serviceStatusCode"`
	ServiceTypeID     int    `json:"serviceTypeId"`
	Message           string `json:"message"`
}

// tranRequest is the body of the verify, settle, cancel and reverse endpoints.
type tranRequest struct {
	MerchantConfigurationID string `json:"merchantConfigurationId"`
	PayGateTranID           int64  `json:"payGateTranId"`
}

// errorResponse is the shape of an AsanPardakht failure body.
type errorResponse struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}
