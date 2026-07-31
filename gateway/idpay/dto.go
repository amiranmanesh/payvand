package idpay

// purchaseRequest is the body of POST /v1.1/payment.
type purchaseRequest struct {
	OrderID  string `json:"order_id"`
	Amount   int64  `json:"amount"`
	Name     string `json:"name,omitempty"`
	Phone    string `json:"phone,omitempty"`
	Mail     string `json:"mail,omitempty"`
	Desc     string `json:"desc,omitempty"`
	Callback string `json:"callback"`
}

// purchaseResponse is the answer of the payment endpoint.
type purchaseResponse struct {
	ID   string `json:"id"`
	Link string `json:"link"`
}

// lookupRequest is the body shared by the verify and inquiry endpoints.
type lookupRequest struct {
	ID      string `json:"id"`
	OrderID string `json:"order_id"`
}

// verifyResponse is the answer of the verify and inquiry endpoints.
type verifyResponse struct {
	Status  int    `json:"status"`
	TrackID int64  `json:"track_id"`
	ID      string `json:"id"`
	OrderID string `json:"order_id"`
	Amount  int64  `json:"amount"`
	Date    string `json:"date"`
	Payment struct {
		TrackID      int64  `json:"track_id"`
		Amount       int64  `json:"amount"`
		CardNo       string `json:"card_no"`
		HashedCardNo string `json:"hashed_card_no"`
		Date         string `json:"date"`
	} `json:"payment"`
	Verify struct {
		Date string `json:"date"`
	} `json:"verify"`
}

// errorResponse is the shape of an IDPay failure body.
type errorResponse struct {
	ErrorCode    int    `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}
