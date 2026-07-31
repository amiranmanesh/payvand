package top_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/top"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

func TestPurchase(t *testing.T) {
	var sent struct {
		Amount   int64  `json:"Amount"`
		OrderID  int64  `json:"OrderId"`
		Pin      string `json:"Pin"`
		MobileNo int64  `json:"MobileNo"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/GetToken": testutil.Capture(t, &sent, `{"Status":0,"Message":"ok","Data":{"Token":"tok-1","OrderId":1001}}`),
	})

	gw, err := top.New(core.Config{MerchantKey: "pin-1"}, core.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount:  core.Rial(150_000),
		OrderID: "1001",
		Mobile:  "09120000000",
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	switch {
	case sent.Pin != "pin-1" || sent.OrderID != 1001 || sent.Amount != 150_000:
		t.Errorf("request = %+v", sent)
	case sent.MobileNo != 9120000000:
		t.Errorf("MobileNo = %d, want the numeric mobile", sent.MobileNo)
	case res.Token != "tok-1":
		t.Errorf("token = %q", res.Token)
	case res.Redirect.URL != "":
		t.Errorf("an in-app gateway must not return a redirect, got %q", res.Redirect.URL)
	}
}

func TestPurchaseRejectsNonNumericOrderID(t *testing.T) {
	gw, _ := top.New(core.Config{MerchantKey: "pin"})
	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{Amount: core.Rial(1000), OrderID: "abc"})
	if !errors.Is(err, core.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestVerifyRunsSetDataWhenConfigured(t *testing.T) {
	setDataCalled := false
	server := testutil.NewServer(t, testutil.Routes{
		"/Confirm": testutil.JSON(`{"Status":0,"Data":{"InvoiceNumber":"INV-9"}}`),
		"/SetData": func(w http.ResponseWriter, _ *http.Request) {
			setDataCalled = true
			_, _ = w.Write([]byte(`{"Status":0}`))
		},
	})
	gw, _ := top.New(core.Config{MerchantKey: "pin"}, core.WithBaseURL(server.URL),
		top.WithSetData(top.AdditionalData{Tag1: "order-1001"}))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "tok-1", Amount: core.Rial(150_000)})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if res.ReferenceNumber != "INV-9" || !setDataCalled {
		t.Fatalf("response = %+v, setData called = %v", res, setDataCalled)
	}
}

func TestVerifyFailure(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/Confirm": testutil.JSON(`{"Status":12,"Message":"token expired"}`),
	})
	gw, _ := top.New(core.Config{MerchantKey: "pin"}, core.WithBaseURL(server.URL))

	if _, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "tok-1"}); !errors.Is(err, core.ErrPaymentFailed) {
		t.Fatalf("error = %v, want ErrPaymentFailed", err)
	}
}

func TestInquiry(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/Inquiry": testutil.JSON(`{"Status":0,"Data":{"Amount":150000,"RRN":"RRN-1","Status":0}}`),
	})
	gw, _ := top.New(core.Config{MerchantKey: "pin"}, core.WithBaseURL(server.URL))

	res, err := gw.Inquiry(context.Background(), core.InquiryRequest{Token: "tok-1"})
	if err != nil {
		t.Fatalf("Inquiry() error = %v", err)
	}
	if res.Status != core.StatusVerified || res.ReferenceNumber != "RRN-1" {
		t.Fatalf("response = %+v", res)
	}
}

func TestCallbackIsNotSupported(t *testing.T) {
	gw, _ := top.New(core.Config{MerchantKey: "pin"})
	if _, err := gw.ParseCallback(nil); !errors.Is(err, core.ErrNotSupported) {
		t.Fatalf("error = %v, want ErrNotSupported for an in-app gateway", err)
	}
	if gw.Capabilities().Callback {
		t.Error("the capabilities must report that there is no callback")
	}
}
