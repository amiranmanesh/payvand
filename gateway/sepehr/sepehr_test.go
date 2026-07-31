package sepehr_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/sepehr"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

func TestPurchase(t *testing.T) {
	var sent struct {
		Amount      int64  `json:"Amount"`
		InvoiceID   string `json:"invoiceID"`
		TerminalID  string `json:"terminalID"`
		CallbackURL string `json:"callbackURL"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/V1/PeymentApi/GetToken": testutil.Capture(t, &sent, `{"Status":0,"Accesstoken":"tok-1"}`),
	})

	gw, err := sepehr.New(core.Config{TerminalID: "9999"}, core.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount:      core.Rial(150_000),
		OrderID:     "1001",
		CallbackURL: "https://shop.example/callback",
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	switch {
	case sent.TerminalID != "9999" || sent.InvoiceID != "1001" || sent.Amount != 150_000:
		t.Errorf("request = %+v", sent)
	case res.Redirect.Method != http.MethodPost:
		t.Errorf("redirect method = %q, want POST", res.Redirect.Method)
	case res.Redirect.Params["token"] != "tok-1" || res.Redirect.Params["TerminalID"] != "9999":
		t.Errorf("redirect params = %v", res.Redirect.Params)
	}
}

func TestPurchaseFailure(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/V1/PeymentApi/GetToken": testutil.JSON(`{"Status":-1,"Message":"terminal is invalid"}`),
	})
	gw, _ := sepehr.New(core.Config{TerminalID: "1"}, core.WithBaseURL(server.URL))

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(1000), OrderID: "1", CallbackURL: "https://shop.example/cb",
	})
	if !errors.Is(err, core.ErrPaymentFailed) {
		t.Fatalf("error = %v, want ErrPaymentFailed", err)
	}
}

func TestVerifyUsesDigitalReceipt(t *testing.T) {
	var sent struct {
		DigitalReceipt string `json:"digitalreceipt"`
		Tid            string `json:"Tid"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/V1/PeymentApi/Advice": testutil.Capture(t, &sent, `{"Status":"Ok","ReturnId":12345}`),
	})
	gw, _ := sepehr.New(core.Config{TerminalID: "9999"}, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{
		OrderID: "1001",
		Amount:  core.Rial(150_000),
		Extra:   map[string]string{"digitalreceipt": "receipt-1"},
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if sent.DigitalReceipt != "receipt-1" || sent.Tid != "9999" {
		t.Errorf("request = %+v", sent)
	}
	if res.ReferenceNumber != "12345" {
		t.Errorf("reference = %q", res.ReferenceNumber)
	}
}

func TestVerifyReportsDuplicateAsAlreadyVerified(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/V1/PeymentApi/Advice": testutil.JSON(`{"Status":"Duplicate","ReturnId":"12345"}`),
	})
	gw, _ := sepehr.New(core.Config{TerminalID: "9999"}, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token: "receipt-1", Amount: core.Rial(1000),
	})
	if !errors.Is(err, core.ErrAlreadyVerified) {
		t.Fatalf("error = %v, want ErrAlreadyVerified", err)
	}
}

func TestVerifyRejectsAReceiptForAnotherInvoice(t *testing.T) {
	gw, _ := sepehr.New(core.Config{TerminalID: "9999"}, core.WithBaseURL("https://example.invalid"))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token:   "receipt-1",
		OrderID: "1001",
		Amount:  core.Rial(1000),
		Extra:   map[string]string{"digitalreceipt": "receipt-1", "invoiceid": "2002"},
	})
	if !errors.Is(err, core.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestVerifyRejectsAReceiptForAnotherAmount(t *testing.T) {
	gw, _ := sepehr.New(core.Config{TerminalID: "9999"}, core.WithBaseURL("https://example.invalid"))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token:   "receipt-1",
		OrderID: "1001",
		Amount:  core.Rial(1_000_000),
		Extra:   map[string]string{"digitalreceipt": "receipt-1", "invoiceid": "1001", "amount": "1000"},
	})
	if !errors.Is(err, core.ErrAmountMismatch) {
		t.Fatalf("error = %v, want ErrAmountMismatch", err)
	}
}

func TestRefund(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/V1/PeymentApi/Rollback": testutil.JSON(`{"Status":"Ok","ReturnId":1}`),
	})
	gw, _ := sepehr.New(core.Config{TerminalID: "9999"}, core.WithBaseURL(server.URL))

	if _, err := gw.Refund(context.Background(), core.RefundRequest{Token: "receipt-1"}); err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
}

func TestParseCallbackFeedsVerify(t *testing.T) {
	gw, _ := sepehr.New(core.Config{TerminalID: "9999"})
	body := strings.NewReader("respcode=0&digitalreceipt=receipt-1&invoiceid=1001&rrn=RRN-1&amount=150000&cardnumber=603799******1234")
	request := httptest.NewRequest(http.MethodPost, "/cb", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.Token != "receipt-1" || callback.OrderID != "1001" {
		t.Fatalf("callback = %+v", callback)
	}
	if callback.VerifyRequest(core.Rial(150_000)).Get("digitalreceipt") != "receipt-1" {
		t.Fatal("the digital receipt must survive into the verify request")
	}
}
