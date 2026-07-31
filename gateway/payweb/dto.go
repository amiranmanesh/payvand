package payweb

// createRequest is the body of POST /Payment/create.
type createRequest struct {
	Amount       int64  `json:"amount"`
	CallbackURL  string `json:"callbackUrl"`
	MobileNumber string `json:"mobileNumber,omitempty"`
	NationalCode string `json:"nationalCode,omitempty"`
	Comment      string `json:"comment,omitempty"`
	ClientRefID  string `json:"clientRefId,omitempty"`
	CardNumber   string `json:"cardNumber,omitempty"`
}

// createResponse is the answer of the create endpoint: "code" is the payment
// identifier used both as token and as gateway query parameter.
type createResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// verifyRequest is the body of POST /Payment/verify.
type verifyRequest struct {
	RefID string `json:"refId"`
}

// verifyResponse is the answer of the verify endpoint.
type verifyResponse struct {
	Amount       float64 `json:"amount"`
	CardNumber   string  `json:"cardNumber"`
	ClientRefID  string  `json:"clientRefId"`
	TrackingCode string  `json:"trackingCode"`
	Message      string  `json:"message"`
}
