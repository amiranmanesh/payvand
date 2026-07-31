package zibal_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/zibal"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

func TestPurchase(t *testing.T) {
	var sent struct {
		Merchant     string `json:"merchant"`
		Amount       int64  `json:"amount"`
		CallbackURL  string `json:"callbackUrl"`
		OrderID      string `json:"orderId"`
		LedgerID     string `json:"ledgerId"`
		Multiplexing []struct {
			BankAccount string `json:"bankAccount"`
			Amount      int64  `json:"amount"`
		} `json:"multiplexingInfos"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/v1/request": testutil.Capture(t, &sent, `{"result":100,"trackId":3355,"message":"success"}`),
	})

	gw, err := zibal.New(core.Config{MerchantKey: "zibal-merchant"},
		core.WithBaseURL(server.URL),
		zibal.WithLedger("ledger-1"),
		zibal.WithMultiplexing(zibal.Share{BankAccount: "IR000000000000000000000001", Amount: 50_000}),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount:      core.Toman(15_000),
		OrderID:     "1001",
		CallbackURL: "https://shop.example/callback",
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}

	switch {
	case sent.Merchant != "zibal-merchant":
		t.Errorf("merchant = %q", sent.Merchant)
	case sent.Amount != 150_000:
		t.Errorf("amount = %d, want Rial", sent.Amount)
	case sent.LedgerID != "ledger-1":
		t.Errorf("ledgerId = %q", sent.LedgerID)
	case len(sent.Multiplexing) != 1 || sent.Multiplexing[0].Amount != 50_000:
		t.Errorf("multiplexingInfos = %+v", sent.Multiplexing)
	case res.Token != "3355":
		t.Errorf("token = %q", res.Token)
	case res.Redirect.URL != server.URL+"/start/3355":
		t.Errorf("redirect = %q", res.Redirect.URL)
	}
}

func TestPurchaseFailure(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v1/request": testutil.JSON(`{"result":102,"message":"merchant not found"}`),
	})
	gw, _ := zibal.New(core.Config{MerchantKey: "m"}, core.WithBaseURL(server.URL))

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(10_000), OrderID: "1", CallbackURL: "https://shop.example/cb",
	})
	if !errors.Is(err, core.ErrPaymentFailed) {
		t.Fatalf("error = %v, want ErrPaymentFailed", err)
	}
}

func TestVerify(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v1/verify": testutil.JSON(`{"result":100,"amount":150000,"refNumber":"9988","cardNumber":"603799******1234",
			"orderId":"1001","paidAt":"2024-05-01T10:20:30.000000","status":1}`),
	})
	gw, _ := zibal.New(core.Config{MerchantKey: "m"}, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "3355", Amount: core.Rial(150_000)})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	switch {
	case res.ReferenceNumber != "9988":
		t.Errorf("reference = %q", res.ReferenceNumber)
	case res.Amount.Rial() != 150_000:
		t.Errorf("amount = %v", res.Amount)
	case res.PaidAt.IsZero():
		t.Error("paidAt was not parsed")
	}
}

func TestVerifyDetectsAmountMismatch(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v1/verify": testutil.JSON(`{"result":100,"amount":15000,"refNumber":"9988","status":1}`),
	})
	gw, _ := zibal.New(core.Config{MerchantKey: "m"}, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "3355", Amount: core.Rial(150_000)})
	if !errors.Is(err, core.ErrAmountMismatch) {
		t.Fatalf("error = %v, want ErrAmountMismatch", err)
	}
}

func TestVerifyRejectsNonNumericToken(t *testing.T) {
	gw, _ := zibal.New(core.Config{MerchantKey: "m"})
	if _, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "not-a-track-id"}); !errors.Is(err, core.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestInquiry(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v1/inquiry": testutil.JSON(`{"result":100,"status":1,"amount":150000,"refNumber":"9988"}`),
	})
	gw, _ := zibal.New(core.Config{MerchantKey: "m"}, core.WithBaseURL(server.URL))

	res, err := gw.Inquiry(context.Background(), core.InquiryRequest{Token: "3355"})
	if err != nil {
		t.Fatalf("Inquiry() error = %v", err)
	}
	if res.Status != core.StatusVerified {
		t.Fatalf("status = %v", res.Status)
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := zibal.New(core.Config{MerchantKey: "m"})
	request := httptest.NewRequest(http.MethodGet, "/cb?trackId=3355&success=1&orderId=1001", nil)

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.Token != "3355" || callback.OrderID != "1001" {
		t.Fatalf("callback = %+v", callback)
	}
}

func TestSandboxForcesTestMerchant(t *testing.T) {
	var sent struct {
		Merchant string `json:"merchant"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/v1/request": testutil.Capture(t, &sent, `{"result":100,"trackId":1}`),
	})
	gw, _ := zibal.New(core.Config{MerchantKey: "real-merchant"},
		core.WithBaseURL(server.URL), core.WithSandbox(true))

	if _, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(10_000), OrderID: "1", CallbackURL: "https://shop.example/cb",
	}); err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	if sent.Merchant != "zibal" {
		t.Fatalf("merchant = %q, want the sandbox merchant", sent.Merchant)
	}
}
