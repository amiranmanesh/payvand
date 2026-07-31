package saman

// tokenRequest is the body of the token action of the online payment endpoint.
type tokenRequest struct {
	Action      string `json:"action"`
	TerminalID  string `json:"TerminalId"`
	Amount      int64  `json:"Amount"`
	ResNum      string `json:"ResNum"`
	RedirectURL string `json:"RedirectUrl"`
	CellNumber  string `json:"CellNumber,omitempty"`
	// SIGN is only used by terminals configured for signed requests.
	Sign string `json:"Sign,omitempty"`
}

// tokenResponse is the answer of the token action.
type tokenResponse struct {
	Status    int    `json:"status"`
	Token     string `json:"token"`
	ErrorCode int    `json:"errorCode"`
	ErrorDesc string `json:"errorDesc"`
}

// transactionRequest is the body shared by verification and reversal.
type transactionRequest struct {
	RefNum         string `json:"RefNum"`
	TerminalNumber string `json:"TerminalNumber"`
}

// transactionResponse is the answer of verification and reversal.
type transactionResponse struct {
	Success           bool   `json:"Success"`
	ResultCode        int    `json:"ResultCode"`
	ResultDescription string `json:"ResultDescription"`
	TransactionDetail struct {
		RRN             string `json:"RRN"`
		RefNum          string `json:"RefNum"`
		MaskedPan       string `json:"MaskedPan"`
		HashedPan       string `json:"HashedPan"`
		TerminalNumber  int64  `json:"TerminalNumber"`
		OriginalAmount  int64  `json:"OriginalAmount"`
		AffectiveAmount int64  `json:"AffectiveAmount"`
		StraceNo        string `json:"StraceNo"`
		StraceDate      string `json:"StraceDate"`
	} `json:"TransactionDetail"`
}
