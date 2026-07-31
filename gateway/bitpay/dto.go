package bitpay

// sendResponse is the answer of /payment/gateway-send. BitPay answers with a
// bare number, positive when it is the payment id and negative when it is an
// error code, so the body is parsed by hand and this shape is only the
// fallback for the JSON flavour of the endpoint.
type sendResponse struct {
	IDGet int64 `json:"id_get"`
}

// verifyResponse is the answer of /payment/gateway-result-second.
type verifyResponse struct {
	Status      int    `json:"status"`
	Amount      string `json:"amount"`
	CardNum     string `json:"cardNum"`
	FactorID    string `json:"factorId"`
	TransID     string `json:"trans_id"`
	IDGet       string `json:"id_get"`
	Message     string `json:"message"`
	Description string `json:"description"`
}
