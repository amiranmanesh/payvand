package digipay

import "encoding/json"

// result is the status block Digipay puts in every business response. A status
// of zero means the call succeeded.
type result struct {
	Status  json.Number `json:"status"`
	Level   string      `json:"level"`
	Message string      `json:"message"`
	Title   string      `json:"title"`
}

// ok reports whether the call succeeded.
func (r result) ok() bool { return r.Status.String() == "" || r.Status.String() == "0" }

// message returns the provider message, or fallback when there is none.
func (r result) message(fallback string) string {
	if r.Message != "" {
		return r.Message
	}
	if r.Title != "" {
		return r.Title
	}
	return fallback
}

// oauthResponse is the answer of the token endpoint.
type oauthResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

// ticketRequest is the body of POST /digipay/api/tickets/business.
type ticketRequest struct {
	Amount           int64  `json:"amount"`
	CellNumber       string `json:"cellNumber,omitempty"`
	ProviderID       string `json:"providerId"`
	CallbackURL      string `json:"callbackUrl"`
	BasketDetailsDto any    `json:"basketDetailsDto,omitempty"`
	SplitDetailsList any    `json:"splitDetailsList,omitempty"`
	PreferredGateway *int   `json:"preferredGateway,omitempty"`
}

// ticketResponse is the answer of the ticket endpoint.
type ticketResponse struct {
	Ticket      string `json:"ticket"`
	RedirectURL string `json:"redirectUrl"`
	Result      result `json:"result"`
}

// verifyResponse is the answer of POST /digipay/api/purchases/verify/{code}.
type verifyResponse struct {
	TrackingCode   string         `json:"trackingCode"`
	ProviderID     string         `json:"providerId"`
	Amount         json.Number    `json:"amount"`
	PaymentGateway json.Number    `json:"paymentGateway"`
	RRN            string         `json:"rrn"`
	CardNumber     string         `json:"maskedPan"`
	AdditionalInfo map[string]any `json:"additionalInfo"`
	Result         result         `json:"result"`
}

// reverseRequest is the body of POST /digipay/api/reverse.
type reverseRequest struct {
	TrackingCode string `json:"trackingCode"`
	ProviderID   string `json:"providerId"`
}

// reverseResponse is the answer of the reverse endpoint.
type reverseResponse struct {
	TrackingCode string      `json:"trackingCode"`
	ProviderID   string      `json:"providerId"`
	Amount       json.Number `json:"amount"`
	Result       result      `json:"result"`
}

// refundRequest is the body of POST /digipay/api/refunds.
type refundRequest struct {
	ProviderID       string `json:"providerId"`
	Amount           int64  `json:"amount"`
	SaleTrackingCode string `json:"saleTrackingCode"`
}

// refundResponse is the answer of the refund endpoint.
type refundResponse struct {
	TrackingCode string `json:"trackingCode"`
	Result       result `json:"result"`
}

// refundStatusResponse is the answer of POST /digipay/api/refunds/{id}.
type refundStatusResponse struct {
	ProviderID      string      `json:"providerId"`
	TrackingCode    string      `json:"trackingCode"`
	Status          json.Number `json:"status"`
	TransferDate    string      `json:"transferDate"`
	DestinationType json.Number `json:"destinationType"`
	Destination     string      `json:"destination"`
	Result          result      `json:"result"`
}

// deliverRequest is the body of POST /digipay/api/purchases/deliver.
type deliverRequest struct {
	DeliveryDate  string   `json:"deliveryDate"`
	InvoiceNumber string   `json:"invoiceNumber"`
	TrackingCode  string   `json:"trackingCode"`
	Products      []string `json:"products"`
}

// deliverResponse is the answer of the deliver endpoint.
type deliverResponse struct {
	Result result `json:"result"`
}
