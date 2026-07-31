package mellat

import "encoding/xml"

// payRequest is the body element of bpPayRequest. The Mellat webservice is
// rpc/encoded, so the parameters are unqualified and their order matters.
type payRequest struct {
	XMLName        xml.Name `xml:"ns:bpPayRequest"`
	TerminalID     string   `xml:"terminalId"`
	UserName       string   `xml:"userName"`
	UserPassword   string   `xml:"userPassword"`
	OrderID        string   `xml:"orderId"`
	Amount         int64    `xml:"amount"`
	LocalDate      string   `xml:"localDate"`
	LocalTime      string   `xml:"localTime"`
	AdditionalData string   `xml:"additionalData"`
	CallBackURL    string   `xml:"callBackUrl"`
	PayerID        string   `xml:"payerId"`
}

// verifyRequest is the body element of bpVerifyRequest.
type verifyRequest struct {
	XMLName         xml.Name `xml:"ns:bpVerifyRequest"`
	TerminalID      string   `xml:"terminalId"`
	UserName        string   `xml:"userName"`
	UserPassword    string   `xml:"userPassword"`
	OrderID         string   `xml:"orderId"`
	SaleOrderID     string   `xml:"saleOrderId"`
	SaleReferenceID string   `xml:"saleReferenceId"`
}

// settleRequest is the body element of bpSettleRequest.
type settleRequest struct {
	XMLName         xml.Name `xml:"ns:bpSettleRequest"`
	TerminalID      string   `xml:"terminalId"`
	UserName        string   `xml:"userName"`
	UserPassword    string   `xml:"userPassword"`
	OrderID         string   `xml:"orderId"`
	SaleOrderID     string   `xml:"saleOrderId"`
	SaleReferenceID string   `xml:"saleReferenceId"`
}

// inquiryRequest is the body element of bpInquiryRequest.
type inquiryRequest struct {
	XMLName         xml.Name `xml:"ns:bpInquiryRequest"`
	TerminalID      string   `xml:"terminalId"`
	UserName        string   `xml:"userName"`
	UserPassword    string   `xml:"userPassword"`
	OrderID         string   `xml:"orderId"`
	SaleOrderID     string   `xml:"saleOrderId"`
	SaleReferenceID string   `xml:"saleReferenceId"`
}

// reversalRequest is the body element of bpReversalRequest.
type reversalRequest struct {
	XMLName         xml.Name `xml:"ns:bpReversalRequest"`
	TerminalID      string   `xml:"terminalId"`
	UserName        string   `xml:"userName"`
	UserPassword    string   `xml:"userPassword"`
	OrderID         string   `xml:"orderId"`
	SaleOrderID     string   `xml:"saleOrderId"`
	SaleReferenceID string   `xml:"saleReferenceId"`
}
