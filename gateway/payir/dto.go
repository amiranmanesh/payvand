package payir

// sendResponse is the answer of POST /pg/send.
type sendResponse struct {
	Status       int    `json:"status"`
	Token        string `json:"token"`
	ErrorCode    int    `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
}

// verifyResponse is the answer of POST /pg/verify.
type verifyResponse struct {
	Status       int    `json:"status"`
	Amount       int64  `json:"amount"`
	TransID      int64  `json:"transId"`
	FactorNumber string `json:"factorNumber"`
	Mobile       string `json:"mobile"`
	Description  string `json:"description"`
	CardNumber   string `json:"cardNumber"`
	Message      string `json:"message"`
	ErrorCode    int    `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
}
