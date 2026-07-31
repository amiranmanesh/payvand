package saman_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/saman"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

func TestPurchase(t *testing.T) {
	var sent struct {
		Action      string `json:"action"`
		TerminalID  string `json:"TerminalId"`
		Amount      int64  `json:"Amount"`
		ResNum      string `json:"ResNum"`
		CellNumber  string `json:"CellNumber"`
		RedirectURL string `json:"RedirectUrl"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/onlinepg/onlinepg": testutil.Capture(t, &sent, `{"status":1,"token":"tok-1"}`),
	})

	gw, err := saman.New(core.Config{TerminalID: "12345678"},
		core.WithBaseURL(server.URL), saman.WithMobile(true), saman.WithGetMethod(true))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount:      core.Rial(150_000),
		OrderID:     "1001",
		CallbackURL: "https://shop.example/callback",
		Mobile:      "09120000000",
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	switch {
	case sent.Action != "token" || sent.TerminalID != "12345678" || sent.ResNum != "1001":
		t.Errorf("request = %+v", sent)
	case sent.CellNumber != "09120000000":
		t.Errorf("CellNumber = %q", sent.CellNumber)
	case res.Redirect.Params["token"] != "tok-1" || res.Redirect.Params["GetMethod"] != "true":
		t.Errorf("redirect params = %v", res.Redirect.Params)
	}
}

func TestPurchaseFailure(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/onlinepg/onlinepg": testutil.JSON(`{"status":-1,"errorCode":3,"errorDesc":"terminal is invalid"}`),
	})
	gw, _ := saman.New(core.Config{TerminalID: "1"}, core.WithBaseURL(server.URL))

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(1000), OrderID: "1", CallbackURL: "https://shop.example/cb",
	})
	var typed *core.Error
	if !errors.As(err, &typed) || typed.Code != "3" {
		t.Fatalf("error = %v, want the SEP error code", err)
	}
}

func TestVerifyDetectsAmountMismatch(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/verifyTxnRandomSessionkey/ipg/VerifyTransaction": testutil.JSON(
			`{"Success":true,"ResultCode":0,"TransactionDetail":{"RRN":"RRN-1","RefNum":"ref-1",
			"AffectiveAmount":15000,"OriginalAmount":150000}}`),
	})
	gw, _ := saman.New(core.Config{TerminalID: "12345678"}, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{
		ReferenceNumber: "ref-1", OrderID: "1001", Amount: core.Rial(150_000),
	})
	if !errors.Is(err, core.ErrAmountMismatch) {
		t.Fatalf("error = %v, want ErrAmountMismatch", err)
	}
}

func TestVerify(t *testing.T) {
	var sent struct {
		RefNum         string `json:"RefNum"`
		TerminalNumber string `json:"TerminalNumber"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/verifyTxnRandomSessionkey/ipg/VerifyTransaction": testutil.Capture(t, &sent,
			`{"Success":true,"ResultCode":0,"TransactionDetail":{"RRN":"RRN-1","RefNum":"ref-1",
			"MaskedPan":"603799******1234","HashedPan":"HASH","AffectiveAmount":150000}}`),
	})
	gw, _ := saman.New(core.Config{TerminalID: "12345678"}, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{
		ReferenceNumber: "ref-1", OrderID: "1001", Amount: core.Rial(150_000),
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	switch {
	case sent.RefNum != "ref-1" || sent.TerminalNumber != "12345678":
		t.Errorf("request = %+v", sent)
	case res.ReferenceNumber != "RRN-1" || res.CardHash != "HASH":
		t.Errorf("response = %+v", res)
	}
}

func TestRefund(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/verifyTxnRandomSessionkey/ipg/ReverseTransaction": testutil.JSON(`{"Success":true,"ResultCode":0}`),
	})
	gw, _ := saman.New(core.Config{TerminalID: "1"}, core.WithBaseURL(server.URL))

	if _, err := gw.Refund(context.Background(), core.RefundRequest{ReferenceNumber: "ref-1"}); err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := saman.New(core.Config{TerminalID: "1"})
	body := strings.NewReader("State=OK&Status=2&RefNum=ref-1&ResNum=1001&TraceNo=99&Amount=150000&SecurePan=603799******1234")
	request := httptest.NewRequest(http.MethodPost, "/cb", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	switch {
	case !callback.Succeeded:
		t.Error("an OK state must be reported as a success")
	case callback.ReferenceNumber != "ref-1" || callback.OrderID != "1001":
		t.Errorf("callback = %+v", callback)
	case callback.Amount.Rial() != 150_000:
		t.Errorf("amount = %v", callback.Amount)
	}
}
