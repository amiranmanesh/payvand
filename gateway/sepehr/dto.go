package sepehr

// tokenRequest is the body of POST /V1/PeymentApi/GetToken.
type tokenRequest struct {
	Amount       int64  `json:"Amount"`
	CallbackURL  string `json:"callbackURL"`
	InvoiceID    string `json:"invoiceID"`
	TerminalID   string `json:"terminalID"`
	Payload      string `json:"payload,omitempty"`
	MobileNo     string `json:"mobileNo,omitempty"`
	NationalCode string `json:"nationalCode,omitempty"`
}

// tokenResponse is the answer of the token endpoint.
type tokenResponse struct {
	Status      int    `json:"Status"`
	AccessToken string `json:"Accesstoken"`
	Message     string `json:"Message"`
}

// adviceRequest is the body shared by the advice and rollback endpoints.
type adviceRequest struct {
	DigitalReceipt string `json:"digitalreceipt"`
	Tid            string `json:"Tid"`
}

// adviceResponse is the answer of the advice and rollback endpoints. Sepehr
// answers with "Ok" on success and "Duplicate" when the receipt was already
// settled.
type adviceResponse struct {
	Status      string `json:"Status"`
	ReturnID    any    `json:"ReturnId"`
	Message     string `json:"Message"`
	Description string `json:"Description"`
}
