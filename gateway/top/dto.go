package top

// getTokenRequest is the body of POST /GetToken.
type getTokenRequest struct {
	Amount         int64  `json:"Amount"`
	OrderID        int64  `json:"OrderId"`
	Pin            string `json:"Pin"`
	MobileNo       int64  `json:"MobileNo,omitempty"`
	UserID         string `json:"UserId,omitempty"`
	AdditionalInfo string `json:"AdditionalInfo,omitempty"`
	CallBackURL    string `json:"CallBackUrl,omitempty"`
}

// getTokenResponse is the answer of the token endpoint.
type getTokenResponse struct {
	Status  int    `json:"Status"`
	Message string `json:"Message"`
	Data    struct {
		Amount     int64  `json:"Amount"`
		OrderID    int64  `json:"OrderId"`
		Token      string `json:"Token"`
		ExpireLen  int    `json:"ExpireLen"`
		CreateDate string `json:"CreateDate"`
	} `json:"Data"`
}

// tokenBody is the single field body shared by Confirm and Inquiry.
type tokenBody struct {
	Token string `json:"Token"`
}

// confirmResponse is the answer of the confirm endpoint.
type confirmResponse struct {
	Status  int    `json:"Status"`
	Message string `json:"Message"`
	Data    struct {
		InvoiceNumber string `json:"InvoiceNumber"`
	} `json:"Data"`
}

// AdditionalData carries the supplementary tags pushed back to the TOP app
// after a confirmation. Their meaning is agreed per merchant contract.
type AdditionalData struct {
	// Tag1 is the first contract defined field.
	Tag1 string `json:"Tag1,omitempty"`
	// Tag2 is the second contract defined field.
	Tag2 string `json:"Tag2,omitempty"`
	// Tag3 is the third contract defined field.
	Tag3 string `json:"Tag3,omitempty"`
}

// setDataRequest is the body of POST /SetData.
type setDataRequest struct {
	Token          string         `json:"Token"`
	AdditionalData AdditionalData `json:"AdditionalData"`
}

// setDataResponse is the answer of the SetData endpoint.
type setDataResponse struct {
	Status  int    `json:"Status"`
	Message string `json:"Message"`
}

// inquiryResponseBody is the answer of the inquiry endpoint.
type inquiryResponseBody struct {
	Status  int    `json:"Status"`
	Message string `json:"Message"`
	Data    struct {
		Amount         int64  `json:"Amount"`
		TransDate      string `json:"TransDate"`
		RRN            string `json:"RRN"`
		TermNo         int    `json:"TermNo"`
		Status         int    `json:"Status"`
		AdditionalInfo string `json:"AdditionalInfo"`
		EShopToken     string `json:"EShopToken"`
		MobileNo       int64  `json:"MobileNo"`
	} `json:"Data"`
}
