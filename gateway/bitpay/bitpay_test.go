package bitpay_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/bitpay"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

func TestPurchaseParsesBareNumber(t *testing.T) {
	var form map[string]string
	server := testutil.NewServer(t, testutil.Routes{
		"/payment/gateway-send": testutil.CaptureForm(t, &form, "123456"),
	})

	gw, err := bitpay.New(core.Config{MerchantKey: "api-key"}, core.WithBaseURL(server.URL))
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
	case form["api"] != "api-key" || form["factorId"] != "1001":
		t.Errorf("form = %v", form)
	case res.Token != "123456":
		t.Errorf("token = %q", res.Token)
	case res.Redirect.URL != server.URL+"/payment/gateway-123456-get":
		t.Errorf("redirect = %q", res.Redirect.URL)
	}
}

func TestPurchaseNegativeCodeIsAnError(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/payment/gateway-send": testutil.JSON("-4"),
	})
	gw, _ := bitpay.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(10_000), CallbackURL: "https://shop.example/cb",
	})
	var typed *core.Error
	if !errors.As(err, &typed) || typed.Code != "-4" {
		t.Fatalf("error = %v, want the BitPay error code", err)
	}
}

func TestVerify(t *testing.T) {
	var form map[string]string
	server := testutil.NewServer(t, testutil.Routes{
		"/payment/gateway-result-second": testutil.CaptureForm(t, &form,
			`{"status":1,"amount":"150000","cardNum":"603799******1234","factorId":"1001"}`),
	})
	gw, _ := bitpay.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token: "123456", ReferenceNumber: "trans-9", Amount: core.Rial(150_000),
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	switch {
	case form["id_get"] != "123456" || form["trans_id"] != "trans-9":
		t.Errorf("form = %v", form)
	case res.OrderID != "1001" || res.CardNumber != "603799******1234":
		t.Errorf("response = %+v", res)
	}
}

func TestVerifyNeedsBothIdentifiers(t *testing.T) {
	gw, _ := bitpay.New(core.Config{MerchantKey: "k"})
	if _, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "123456"}); !errors.Is(err, core.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := bitpay.New(core.Config{MerchantKey: "k"})
	request := httptest.NewRequest(http.MethodGet, "/cb?id_get=123456&trans_id=trans-9&factorId=1001", nil)

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.Token != "123456" || callback.ReferenceNumber != "trans-9" {
		t.Fatalf("callback = %+v", callback)
	}
}

func TestVerifyDetectsAmountMismatch(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/payment/gateway-result-second": testutil.JSON(`{"status":1,"amount":"15000","factorId":"1001"}`),
	})
	gw, _ := bitpay.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token: "123456", ReferenceNumber: "trans-9", Amount: core.Rial(150_000),
	})
	if !errors.Is(err, core.ErrAmountMismatch) {
		t.Fatalf("error = %v, want ErrAmountMismatch", err)
	}
}

func TestVerifyReportsAlreadyVerified(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/payment/gateway-result-second": testutil.JSON(`{"status":11,"amount":"150000","factorId":"1001"}`),
	})
	gw, _ := bitpay.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token: "123456", ReferenceNumber: "trans-9", Amount: core.Rial(150_000),
	})
	if !errors.Is(err, core.ErrAlreadyVerified) {
		t.Fatalf("error = %v, want ErrAlreadyVerified", err)
	}
}
