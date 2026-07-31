package zarinpal

import "encoding/json"

// purchaseRequest is the body of POST /pg/v4/payment/request.json.
type purchaseRequest struct {
	MerchantID  string            `json:"merchant_id"`
	Amount      int64             `json:"amount"`
	CallbackURL string            `json:"callback_url"`
	Description string            `json:"description"`
	Currency    string            `json:"currency,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Wages       []Wage            `json:"wages,omitempty"`
}

// Wage is one share of a split settlement (Zarinpal "wages").
type Wage struct {
	// IBAN is the destination account of the share.
	IBAN string `json:"iban"`
	// Amount is the share value, in the same unit as the payment.
	Amount int64 `json:"amount"`
	// Description explains the share on the settlement report.
	Description string `json:"description"`
}

// purchaseData is the payload of a successful payment request.
type purchaseData struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Authority string `json:"authority"`
	Fee       int64  `json:"fee"`
	FeeType   string `json:"fee_type"`
}

// UnmarshalJSON tolerates the empty array Zarinpal sends instead of an object
// when the request failed.
func (d *purchaseData) UnmarshalJSON(raw []byte) error {
	if !isObject(raw) {
		return nil
	}
	type alias purchaseData
	var decoded alias
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	*d = purchaseData(decoded)
	return nil
}

// purchaseResponse is the answer of the payment request endpoint.
type purchaseResponse struct {
	Data   purchaseData    `json:"data"`
	Errors json.RawMessage `json:"errors"`
}

// verifyRequest is the body of POST /pg/v4/payment/verify.json.
type verifyRequest struct {
	MerchantID string `json:"merchant_id"`
	Amount     int64  `json:"amount"`
	Authority  string `json:"authority"`
}

// verifyData is the payload of a successful verification.
type verifyData struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	RefID    int64  `json:"ref_id"`
	CardPan  string `json:"card_pan"`
	CardHash string `json:"card_hash"`
	Fee      int64  `json:"fee"`
	FeeType  string `json:"fee_type"`
}

// UnmarshalJSON tolerates the empty array sent on failure.
func (d *verifyData) UnmarshalJSON(raw []byte) error {
	if !isObject(raw) {
		return nil
	}
	type alias verifyData
	var decoded alias
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	*d = verifyData(decoded)
	return nil
}

// verifyResponse is the answer of the verification endpoint.
type verifyResponse struct {
	Data   verifyData      `json:"data"`
	Errors json.RawMessage `json:"errors"`
}

// inquiryRequest is the body of POST /pg/v4/payment/inquiry.json.
type inquiryRequest struct {
	MerchantID string `json:"merchant_id"`
	Authority  string `json:"authority"`
}

// inquiryData is the payload of a successful inquiry.
type inquiryData struct {
	Code   int    `json:"code"`
	Status string `json:"status"`
	Amount int64  `json:"amount"`
}

// UnmarshalJSON tolerates the empty array sent on failure.
func (d *inquiryData) UnmarshalJSON(raw []byte) error {
	if !isObject(raw) {
		return nil
	}
	type alias inquiryData
	var decoded alias
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	*d = inquiryData(decoded)
	return nil
}

// inquiryResponse is the answer of the inquiry endpoint.
type inquiryResponse struct {
	Data   inquiryData     `json:"data"`
	Errors json.RawMessage `json:"errors"`
}

// isObject reports whether a raw JSON value is an object, which is how the
// success and failure shapes of a Zarinpal field are told apart.
func isObject(raw []byte) bool {
	for _, b := range raw {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{':
			return true
		default:
			return false
		}
	}
	return false
}

// errorMessage extracts a readable message out of the polymorphic "errors"
// field, which Zarinpal sends as an empty array on success and as an object on
// failure.
func errorMessage(raw json.RawMessage) (code, message string) {
	if !isObject(raw) {
		return "", ""
	}
	var detail struct {
		Code    json.Number `json:"code"`
		Message string      `json:"message"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		return "", ""
	}
	return detail.Code.String(), detail.Message
}
