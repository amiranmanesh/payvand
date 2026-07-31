package parsian

import "encoding/xml"

// saleRequest is the body element of the plain sale operation.
type saleRequest struct {
	XMLName     xml.Name        `xml:"ns:SalePaymentRequest"`
	RequestData saleRequestData `xml:"ns:requestData"`
}

// saleRequestData carries the fields of a plain sale.
type saleRequestData struct {
	LoginAccount   string `xml:"ns:LoginAccount"`
	Amount         int64  `xml:"ns:Amount"`
	OrderID        string `xml:"ns:OrderId"`
	CallBackURL    string `xml:"ns:CallBackUrl"`
	AdditionalData string `xml:"ns:AdditionalData"`
	Originator     string `xml:"ns:Originator"`
}

// multiplexedSaleRequest is the body element of the split settlement sale.
type multiplexedSaleRequest struct {
	XMLName     xml.Name               `xml:"ns:MultiplexedSaleWithIBANPaymentRequest"`
	RequestData multiplexedRequestData `xml:"ns:requestData"`
}

// multiplexedRequestData carries the fields of a split settlement sale.
type multiplexedRequestData struct {
	LoginAccount        string           `xml:"ns:LoginAccount"`
	Amount              int64            `xml:"ns:Amount"`
	OrderID             string           `xml:"ns:OrderId"`
	CallBackURL         string           `xml:"ns:CallBackUrl"`
	AdditionalData      string           `xml:"ns:AdditionalData"`
	Originator          string           `xml:"ns:Originator"`
	MultiplexedAccounts []multiplexedRow `xml:"ns:MultiplexedAccounts"`
}

// multiplexedRow is one destination account of a split settlement.
type multiplexedRow struct {
	AccountWithIBAN accountWithIBAN `xml:"ns:AccountWithIBAN"`
}

// accountWithIBAN is the account/amount pair of a split settlement row.
type accountWithIBAN struct {
	IBAN   string `xml:"ns:IBAN"`
	Amount int64  `xml:"ns:Amount"`
	PayID  int64  `xml:"ns:PayId"`
}

// saleResponse is the answer of both sale operations.
type saleResponse struct {
	Result struct {
		Token   int64  `xml:"Token"`
		Status  int    `xml:"Status"`
		Message string `xml:"Message"`
	} `xml:",any"`
}

// confirmRequest is the body element of the confirm operation.
type confirmRequest struct {
	XMLName     xml.Name   `xml:"ns:ConfirmPayment"`
	RequestData tokenNamed `xml:"ns:requestData"`
}

// reversalRequest is the body element of the reversal operation.
type reversalRequest struct {
	XMLName     xml.Name   `xml:"ns:ReversalRequest"`
	RequestData tokenNamed `xml:"ns:requestData"`
}

// tokenNamed is the login/token pair both confirm and reversal take.
type tokenNamed struct {
	LoginAccount string `xml:"ns:LoginAccount"`
	Token        string `xml:"ns:Token"`
}

// confirmResponse is the answer of the confirm operation.
type confirmResponse struct {
	Result struct {
		Status           int    `xml:"Status"`
		CardNumberMasked string `xml:"CardNumberMasked"`
		RRN              int64  `xml:"RRN"`
		Token            int64  `xml:"Token"`
	} `xml:",any"`
}

// reversalResponse is the answer of the reversal operation.
type reversalResponse struct {
	Result struct {
		Status  int    `xml:"Status"`
		Token   int64  `xml:"Token"`
		Message string `xml:"Message"`
	} `xml:",any"`
}
