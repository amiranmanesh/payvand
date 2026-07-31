package zibal

// purchaseRequest is the body of POST /v1/request.
type purchaseRequest struct {
	Merchant            string   `json:"merchant"`
	Amount              int64    `json:"amount"`
	CallbackURL         string   `json:"callbackUrl"`
	Description         string   `json:"description,omitempty"`
	OrderID             string   `json:"orderId,omitempty"`
	Mobile              string   `json:"mobile,omitempty"`
	NationalCode        string   `json:"nationalCode,omitempty"`
	AllowedCards        []string `json:"allowedCards,omitempty"`
	LedgerID            string   `json:"ledgerId,omitempty"`
	FeeMode             int      `json:"feeMode,omitempty"`
	CheckMobileWithCard bool     `json:"checkMobileWithCard,omitempty"`
	PercentMode         int      `json:"percentMode,omitempty"`
	Multiplexing        []Share  `json:"multiplexingInfos,omitempty"`
}

// Share is one destination of a split settlement.
type Share struct {
	// BankAccount is the destination IBAN, when settling to a bank account.
	BankAccount string `json:"bankAccount,omitempty"`
	// SubMerchantID is the destination sub merchant, when settling inside
	// Zibal.
	SubMerchantID string `json:"subMerchantId,omitempty"`
	// WalletID is the destination Zibal wallet.
	WalletID string `json:"walletId,omitempty"`
	// Amount is the share value in Rial, or the percentage when the gateway
	// was built with percent mode.
	Amount int64 `json:"amount"`
}

// purchaseResponse is the answer of the payment request endpoint.
type purchaseResponse struct {
	Result  int    `json:"result"`
	TrackID int64  `json:"trackId"`
	Message string `json:"message"`
}

// verifyRequest is the body of POST /v1/verify.
type verifyRequest struct {
	Merchant string `json:"merchant"`
	TrackID  int64  `json:"trackId"`
}

// verifyResponse is the answer of the verification endpoint.
type verifyResponse struct {
	Result      int    `json:"result"`
	Message     string `json:"message"`
	PaidAt      string `json:"paidAt"`
	Amount      int64  `json:"amount"`
	Status      int    `json:"status"`
	RefNumber   string `json:"refNumber"`
	Description string `json:"description"`
	CardNumber  string `json:"cardNumber"`
	OrderID     string `json:"orderId"`
}

// inquiryResponse is the answer of POST /v1/inquiry.
type inquiryResponse struct {
	Result      int    `json:"result"`
	Message     string `json:"message"`
	RefNumber   string `json:"refNumber"`
	PaidAt      string `json:"paidAt"`
	Amount      int64  `json:"amount"`
	Status      int    `json:"status"`
	CardNumber  string `json:"cardNumber"`
	OrderID     string `json:"orderId"`
	Description string `json:"description"`
}
