package irankish

// tokenRequest is the body of POST /api/v3/tokenization/make.
type tokenRequest struct {
	Request                requestData            `json:"request"`
	AuthenticationEnvelope authenticationEnvelope `json:"authenticationEnvelope"`
	CmsPreservationID      string                 `json:"cmsPreservationId,omitempty"`
}

// requestData carries the transaction fields of a tokenization request.
type requestData struct {
	AcceptorID       string `json:"acceptorId"`
	Amount           int64  `json:"amount"`
	BillInfo         any    `json:"billInfo,omitempty"`
	PaymentID        string `json:"paymentId,omitempty"`
	RequestID        string `json:"requestId"`
	RequestTimestamp int64  `json:"requestTimestamp"`
	RevertURI        string `json:"revertUri"`
	TerminalID       string `json:"terminalId"`
	TransactionType  string `json:"transactionType"`
}

// authenticationEnvelope is the AES key and IV encrypted with the acquirer's
// RSA public key.
type authenticationEnvelope struct {
	Data string `json:"data"`
	IV   string `json:"iv"`
}

// tokenResponse is the answer of the tokenization endpoint.
type tokenResponse struct {
	ResponseCode string `json:"responseCode"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	Result       struct {
		Token string `json:"token"`
	} `json:"result"`
}

// confirmRequest is the body of POST /api/v3/confirmation/purchase.
type confirmRequest struct {
	TerminalID               string `json:"terminalId"`
	RetrievalReferenceNumber string `json:"retrievalReferenceNumber"`
	SystemTraceAuditNumber   string `json:"systemTraceAuditNumber"`
	TokenIdentity            string `json:"tokenIdentity"`
}

// confirmResponse is the answer of the confirmation endpoint.
type confirmResponse struct {
	ResponseCode string `json:"responseCode"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	Result       struct {
		RetrievalReferenceNumber string `json:"retrievalReferenceNumber"`
		SystemTraceAuditNumber   string `json:"systemTraceAuditNumber"`
		ResponseCode             string `json:"responseCode"`
		Amount                   int64  `json:"amount"`
		MaskedPan                string `json:"maskedPan"`
	} `json:"result"`
}
