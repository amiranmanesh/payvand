package payir_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/payir"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

func TestPurchase(t *testing.T) {
	var form map[string]string
	server := testutil.NewServer(t, testutil.Routes{
		"/pg/send": testutil.CaptureForm(t, &form, `{"status":1,"token":"tok-1"}`),
	})

	gw, err := payir.New(core.Config{MerchantKey: "api-key"},
		core.WithBaseURL(server.URL), payir.WithOrderAsFactorNumber(true))
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
	case form["api"] != "api-key":
		t.Errorf("api = %q", form["api"])
	case form["amount"] != "150000":
		t.Errorf("amount = %q", form["amount"])
	case form["factorNumber"] != "1001":
		t.Errorf("factorNumber = %q", form["factorNumber"])
	case res.Redirect.URL != server.URL+"/pg/tok-1":
		t.Errorf("redirect = %q", res.Redirect.URL)
	}
}

func TestPurchaseFailure(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/pg/send": testutil.JSON(`{"status":0,"errorCode":3,"errorMessage":"api key is invalid"}`),
	})
	gw, _ := payir.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(10_000), CallbackURL: "https://shop.example/cb",
	})
	var typed *core.Error
	if !errors.As(err, &typed) || typed.Code != "3" {
		t.Fatalf("error = %v, want the Pay.ir error code", err)
	}
}

func TestVerify(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/pg/verify": testutil.JSON(`{"status":1,"amount":150000,"transId":8899,"cardNumber":"603799******1234",
			"factorNumber":"1001"}`),
	})
	gw, _ := payir.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "tok-1", Amount: core.Rial(150_000)})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if res.ReferenceNumber != "8899" || res.OrderID != "1001" {
		t.Fatalf("unexpected response %+v", res)
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := payir.New(core.Config{MerchantKey: "k"})
	request := httptest.NewRequest(http.MethodGet, "/cb?token=tok-1&status=1", nil)

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.Token != "tok-1" {
		t.Fatalf("callback = %+v", callback)
	}
}

func TestSandboxUsesTestKey(t *testing.T) {
	var form map[string]string
	server := testutil.NewServer(t, testutil.Routes{
		"/pg/send": testutil.CaptureForm(t, &form, `{"status":1,"token":"tok"}`),
	})
	gw, _ := payir.New(core.Config{MerchantKey: "real"}, core.WithBaseURL(server.URL), core.WithSandbox(true))

	if _, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(10_000), CallbackURL: "https://shop.example/cb",
	}); err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	if form["api"] != "test" {
		t.Fatalf("api = %q, want the sandbox key", form["api"])
	}
}

func TestVerifyDetectsAmountMismatch(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/pg/verify": testutil.JSON(`{"status":1,"amount":15000,"transId":8899,"factorNumber":"1001"}`),
	})
	gw, _ := payir.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "tok-1", Amount: core.Rial(150_000)})
	if !errors.Is(err, core.ErrAmountMismatch) {
		t.Fatalf("error = %v, want ErrAmountMismatch", err)
	}
}
