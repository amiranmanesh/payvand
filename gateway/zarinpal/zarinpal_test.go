package zarinpal_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/zarinpal"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

func TestPurchase(t *testing.T) {
	var sent struct {
		MerchantID  string `json:"merchant_id"`
		Amount      int64  `json:"amount"`
		Description string `json:"description"`
		Currency    string `json:"currency"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/pg/v4/payment/request.json": testutil.Capture(t, &sent,
			`{"data":{"code":100,"authority":"A00000000000000000000000000000012345"},"errors":[]}`),
	})

	gw, err := zarinpal.New(core.Config{MerchantKey: "merchant-id"}, core.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount:      core.Toman(15_000),
		OrderID:     "1001",
		CallbackURL: "https://shop.example/callback",
		Description: "Wallet top-up",
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}

	switch {
	case sent.MerchantID != "merchant-id":
		t.Errorf("merchant_id = %q", sent.MerchantID)
	case sent.Amount != 150_000:
		t.Errorf("amount = %d, want the Toman amount converted to Rial", sent.Amount)
	case sent.Description != "Wallet top-up":
		t.Errorf("description = %q", sent.Description)
	case res.Token != "A00000000000000000000000000000012345":
		t.Errorf("token = %q", res.Token)
	case res.Redirect.URL != server.URL+"/pg/StartPay/"+res.Token:
		t.Errorf("redirect = %q", res.Redirect.URL)
	}
}

func TestPurchaseWithTomanTerminal(t *testing.T) {
	var sent struct {
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/pg/v4/payment/request.json": testutil.Capture(t, &sent, `{"data":{"code":100,"authority":"A1"},"errors":[]}`),
	})

	gw, _ := zarinpal.New(core.Config{MerchantKey: "m"},
		core.WithBaseURL(server.URL), zarinpal.WithCurrency(core.IRT))

	if _, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount:      core.Rial(150_000),
		CallbackURL: "https://shop.example/callback",
		OrderID:     "1",
	}); err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	if sent.Amount != 15_000 || sent.Currency != "IRT" {
		t.Fatalf("amount = %d, currency = %q, want 15000 IRT", sent.Amount, sent.Currency)
	}
}

func TestPurchaseRejectedByGateway(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/pg/v4/payment/request.json": testutil.JSON(
			`{"data":[],"errors":{"code":-9,"message":"validation error"}}`),
	})
	gw, _ := zarinpal.New(core.Config{MerchantKey: "m"}, core.WithBaseURL(server.URL))

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(1000), CallbackURL: "https://shop.example/cb", OrderID: "1",
	})
	if !errors.Is(err, core.ErrPaymentFailed) {
		t.Fatalf("error = %v, want ErrPaymentFailed", err)
	}
	var typed *core.Error
	if !errors.As(err, &typed) || typed.Code != "-9" {
		t.Fatalf("the gateway code was not carried, got %#v", typed)
	}
}

func TestVerify(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/pg/v4/payment/verify.json": testutil.JSON(
			`{"data":{"code":100,"ref_id":98765,"card_pan":"601234******5678","fee":1000},"errors":[]}`),
	})
	gw, _ := zarinpal.New(core.Config{MerchantKey: "m"}, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token: "A1", OrderID: "1001", Amount: core.Rial(150_000),
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if res.ReferenceNumber != "98765" || res.CardNumber != "601234******5678" || res.Fee != 1000 {
		t.Fatalf("unexpected response %+v", res)
	}
}

func TestVerifyAcceptsAlreadyVerified(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/pg/v4/payment/verify.json": testutil.JSON(`{"data":{"code":101,"ref_id":55},"errors":[]}`),
	})
	gw, _ := zarinpal.New(core.Config{MerchantKey: "m"}, core.WithBaseURL(server.URL))

	if _, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "A1", Amount: core.Rial(1000)}); err != nil {
		t.Fatalf("Verify() error = %v, want a repeated verification to succeed", err)
	}
}

func TestInquiry(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/pg/v4/payment/inquiry.json": testutil.JSON(`{"data":{"code":100,"status":"VERIFIED","amount":150000},"errors":[]}`),
	})
	gw, _ := zarinpal.New(core.Config{MerchantKey: "m"}, core.WithBaseURL(server.URL))

	res, err := gw.Inquiry(context.Background(), core.InquiryRequest{Token: "A1"})
	if err != nil {
		t.Fatalf("Inquiry() error = %v", err)
	}
	if res.Status != core.StatusVerified {
		t.Fatalf("status = %v, want verified", res.Status)
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := zarinpal.New(core.Config{MerchantKey: "m"})

	request := httptest.NewRequest(http.MethodGet, "/callback?Authority=A1&Status=OK", nil)
	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.Token != "A1" {
		t.Fatalf("callback = %+v", callback)
	}

	request = httptest.NewRequest(http.MethodGet, "/callback?Authority=A1&Status=NOK", nil)
	callback, _ = gw.ParseCallback(request)
	if callback.Succeeded {
		t.Fatal("a NOK status must not be reported as a success")
	}
}

func TestRefundIsNotSupported(t *testing.T) {
	gw, _ := zarinpal.New(core.Config{MerchantKey: "m"})
	if _, err := gw.Refund(context.Background(), core.RefundRequest{}); !errors.Is(err, core.ErrNotSupported) {
		t.Fatalf("error = %v, want ErrNotSupported", err)
	}
}

func TestNewRejectsEmptyMerchantKey(t *testing.T) {
	_, err := zarinpal.New(core.Config{})
	if !errors.Is(err, core.ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
}

func TestSandboxUsesSandboxHost(t *testing.T) {
	gw, err := zarinpal.New(core.Config{MerchantKey: "m"}, core.WithSandbox(true))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if gw.Name() != zarinpal.Name {
		t.Fatalf("name = %q", gw.Name())
	}
	if !strings.Contains(gw.Capabilities().RedirectMethod, http.MethodGet) {
		t.Fatalf("redirect method = %q", gw.Capabilities().RedirectMethod)
	}
}
