package yekpay_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/yekpay"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

func TestPurchase(t *testing.T) {
	var sent struct {
		MerchantID       string `json:"merchantId"`
		Amount           int64  `json:"amount"`
		FromCurrencyCode int    `json:"fromCurrencyCode"`
		ToCurrencyCode   int    `json:"toCurrencyCode"`
		FirstName        string `json:"firstName"`
		LastName         string `json:"lastName"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/api/payment/request": testutil.Capture(t, &sent, `{"Code":100,"Authority":"auth-1"}`),
	})

	gw, err := yekpay.New(core.Config{MerchantKey: "merchant-1"}, core.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount:      core.Rial(150_000),
		OrderID:     "1001",
		CallbackURL: "https://shop.example/callback",
		PayerName:   "Ali Rezaei",
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	switch {
	case sent.MerchantID != "merchant-1" || sent.Amount != 150_000:
		t.Errorf("request = %+v", sent)
	case sent.FromCurrencyCode != yekpay.CurrencyIRR || sent.ToCurrencyCode != yekpay.CurrencyIRR:
		t.Errorf("currencies = %d/%d, want Rial by default", sent.FromCurrencyCode, sent.ToCurrencyCode)
	case sent.FirstName != "Ali" || sent.LastName != "Rezaei":
		t.Errorf("payer name was not split: %q %q", sent.FirstName, sent.LastName)
	case res.Redirect.URL != server.URL+"/api/payment/start/auth-1":
		t.Errorf("redirect = %q", res.Redirect.URL)
	}
}

func TestSandboxUsesItsOwnPaths(t *testing.T) {
	// The sandbox is not a mirror of the production paths, so a gateway built
	// with WithSandbox must call /api/sandbox/* and redirect there too. The
	// fake server fails any other path, which is the assertion.
	server := testutil.NewServer(t, testutil.Routes{
		"/api/sandbox/request": testutil.JSON(`{"Code":100,"Authority":"auth-1"}`),
		"/api/sandbox/verify":  testutil.JSON(`{"Code":100,"Reference":"REF-9","OrderNo":"1001"}`),
	})
	gw, err := yekpay.New(core.Config{MerchantKey: "merchant-1"},
		core.WithSandbox(true), core.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(150_000), OrderID: "1001", CallbackURL: "https://shop.example/callback",
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	if res.Redirect.URL != server.URL+"/api/sandbox/payment/auth-1" {
		t.Fatalf("redirect = %q", res.Redirect.URL)
	}
	if _, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "auth-1"}); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestPurchaseWithForeignCurrency(t *testing.T) {
	var sent struct {
		Amount           int64 `json:"amount"`
		FromCurrencyCode int   `json:"fromCurrencyCode"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/api/payment/request": testutil.Capture(t, &sent, `{"Code":100,"Authority":"a"}`),
	})
	gw, _ := yekpay.New(core.Config{MerchantKey: "m"},
		core.WithBaseURL(server.URL), yekpay.WithCurrencies(yekpay.CurrencyEUR, yekpay.CurrencyIRR))

	if _, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Money{Amount: 25}, OrderID: "1", CallbackURL: "https://shop.example/cb",
	}); err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	if sent.Amount != 25 || sent.FromCurrencyCode != yekpay.CurrencyEUR {
		t.Fatalf("a foreign amount must be sent as is, got %+v", sent)
	}
}

func TestVerify(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/api/payment/verify": testutil.JSON(`{"Code":100,"Reference":"REF-9","OrderNo":"1001"}`),
	})
	gw, _ := yekpay.New(core.Config{MerchantKey: "m"}, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "auth-1", Amount: core.Rial(150_000)})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if res.ReferenceNumber != "REF-9" || res.OrderID != "1001" {
		t.Fatalf("response = %+v", res)
	}
}

func TestVerifyFailure(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/api/payment/verify": testutil.JSON(`{"Code":-3,"Description":"authority not found"}`),
	})
	gw, _ := yekpay.New(core.Config{MerchantKey: "m"}, core.WithBaseURL(server.URL))

	if _, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "auth-1"}); !errors.Is(err, core.ErrPaymentFailed) {
		t.Fatalf("error = %v, want ErrPaymentFailed", err)
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := yekpay.New(core.Config{MerchantKey: "m"})
	request := httptest.NewRequest(http.MethodGet, "/cb?authority=auth-1&status=OK", nil)

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.Token != "auth-1" {
		t.Fatalf("callback = %+v", callback)
	}
}

func TestVerifyDetectsAmountMismatch(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/api/payment/verify": testutil.JSON(`{"Code":100,"Reference":"YK-9","Amount":15000,"OrderNo":"1001"}`),
	})
	gw, _ := yekpay.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "auth-1", Amount: core.Rial(150_000)})
	if !errors.Is(err, core.ErrAmountMismatch) {
		t.Fatalf("error = %v, want ErrAmountMismatch", err)
	}
}
