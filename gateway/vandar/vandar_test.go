package vandar_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/vandar"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

func TestPurchase(t *testing.T) {
	var sent struct {
		APIKey       string `json:"api_key"`
		Amount       int64  `json:"amount"`
		FactorNumber string `json:"factorNumber"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/api/v4/send": testutil.Capture(t, &sent, `{"status":1,"token":"tok-1"}`),
	})

	gw, err := vandar.New(core.Config{MerchantKey: "api-key"},
		core.WithBaseURL(server.URL), vandar.WithOrderAsFactorNumber(true))
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
	case sent.APIKey != "api-key":
		t.Errorf("api_key = %q", sent.APIKey)
	case sent.FactorNumber != "1001":
		t.Errorf("factorNumber = %q", sent.FactorNumber)
	case res.Redirect.URL != server.URL+"/v4/tok-1":
		t.Errorf("redirect = %q", res.Redirect.URL)
	}
}

func TestPurchaseRejectsSmallAmount(t *testing.T) {
	gw, _ := vandar.New(core.Config{MerchantKey: "k"})
	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(500), CallbackURL: "https://shop.example/cb",
	})
	if !errors.Is(err, core.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestVerifyRunsBothSteps(t *testing.T) {
	transactionCalled, verifyCalled := false, false
	server := testutil.NewServer(t, testutil.Routes{
		"/api/v4/transaction": func(w http.ResponseWriter, _ *http.Request) {
			transactionCalled = true
			_, _ = w.Write([]byte(`{"status":1,"refnumber":"RRN-1","transId":42,"amount":"150000"}`))
		},
		"/api/v4/verify": func(w http.ResponseWriter, _ *http.Request) {
			verifyCalled = true
			_, _ = w.Write([]byte(`{"status":1,"amount":"150000","transId":42,"cardNumber":"603799******1234","wage":1000}`))
		},
	})
	gw, _ := vandar.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "tok-1", Amount: core.Rial(150_000)})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	switch {
	case !transactionCalled || !verifyCalled:
		t.Error("both the transaction and the verify endpoints must be called")
	case res.ReferenceNumber != "RRN-1":
		t.Errorf("reference = %q", res.ReferenceNumber)
	case res.CardNumber != "603799******1234":
		t.Errorf("card = %q", res.CardNumber)
	case res.Fee != 1000:
		t.Errorf("fee = %d", res.Fee)
	}
}

func TestRefund(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v3/business/my-business/transaction/42/refund": testutil.JSON(`{"status":1,"message":"ok"}`),
	})
	gw, _ := vandar.New(core.Config{MerchantKey: "k", MerchantID: "my-business"},
		core.WithBaseURL(server.URL), vandar.WithAccessToken("bearer-token"))

	if _, err := gw.Refund(context.Background(), core.RefundRequest{
		TransactionID: "42", Amount: core.Rial(150_000),
	}); err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
}

func TestRefundNeedsBusinessName(t *testing.T) {
	gw, _ := vandar.New(core.Config{MerchantKey: "k"})
	_, err := gw.Refund(context.Background(), core.RefundRequest{TransactionID: "42"})
	if !errors.Is(err, core.ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := vandar.New(core.Config{MerchantKey: "k"})
	request := httptest.NewRequest(http.MethodGet, "/cb?token=tok-1&payment_status=OK", nil)

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.Token != "tok-1" {
		t.Fatalf("callback = %+v", callback)
	}
}

func TestVerifyDetectsAmountMismatch(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/api/v4/transaction": testutil.JSON(`{"status":1,"refnumber":"RRN-1","transId":42,"amount":"15000.00"}`),
		"/api/v4/verify":      testutil.JSON(`{"status":1,"amount":"15000.00","transId":42}`),
	})
	gw, _ := vandar.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "tok-1", Amount: core.Rial(150_000)})
	if !errors.Is(err, core.ErrAmountMismatch) {
		t.Fatalf("error = %v, want ErrAmountMismatch", err)
	}
}

func TestVerifyReadsDecimalAmount(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/api/v4/transaction": testutil.JSON(`{"status":1,"refnumber":"RRN-1","transId":42,"amount":"150000.00"}`),
		"/api/v4/verify":      testutil.JSON(`{"status":1,"amount":"150000.00","transId":42}`),
	})
	gw, _ := vandar.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "tok-1", Amount: core.Rial(150_000)})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if res.Amount.Rial() != 150_000 {
		t.Fatalf("amount = %v, want 150000 IRR", res.Amount)
	}
}
