package idpay_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/idpay"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

func TestPurchase(t *testing.T) {
	var apiKey, sandbox string
	var sent struct {
		OrderID string `json:"order_id"`
		Amount  int64  `json:"amount"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/v1.1/payment": func(w http.ResponseWriter, r *http.Request) {
			apiKey = r.Header.Get("X-API-KEY")
			sandbox = r.Header.Get("X-SANDBOX")
			testutil.Capture(t, &sent, `{"id":"id-1","link":"https://idpay.ir/p/ws/id-1"}`)(w, r)
		},
	})

	gw, err := idpay.New(core.Config{MerchantKey: "api-key"},
		core.WithBaseURL(server.URL), core.WithSandbox(true))
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
	case apiKey != "api-key":
		t.Errorf("X-API-KEY = %q", apiKey)
	case sandbox != "1":
		t.Errorf("X-SANDBOX = %q, want 1", sandbox)
	case sent.Amount != 150_000:
		t.Errorf("amount = %d, want Rial", sent.Amount)
	case res.Token != "id-1" || !strings.HasPrefix(res.Redirect.URL, "https://idpay.ir/"):
		t.Errorf("unexpected response %+v", res)
	}
}

func TestPurchaseError(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v1.1/payment": testutil.Status(http.StatusBadRequest, `{"error_code":34,"error_message":"amount is too low"}`),
	})
	gw, _ := idpay.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(100), OrderID: "1", CallbackURL: "https://shop.example/cb",
	})
	var typed *core.Error
	if !errors.As(err, &typed) || typed.Code != "34" {
		t.Fatalf("error = %v, want the IDPay error code", err)
	}
}

func TestVerify(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v1.1/payment/verify": testutil.JSON(`{"status":100,"track_id":9911,"id":"id-1","order_id":"1001",
			"amount":150000,"date":"1714550400","payment":{"track_id":9911,"amount":150000,"card_no":"603799******1234",
			"hashed_card_no":"HASH"}}`),
	})
	gw, _ := idpay.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token: "id-1", OrderID: "1001", Amount: core.Rial(150_000),
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	switch {
	case res.ReferenceNumber != "9911":
		t.Errorf("reference = %q", res.ReferenceNumber)
	case res.CardHash != "HASH":
		t.Errorf("card hash = %q", res.CardHash)
	case res.PaidAt.IsZero():
		t.Error("the payment date was not parsed")
	}
}

func TestInquiry(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v1.1/payment/inquiry": testutil.JSON(`{"status":10,"track_id":9911,"id":"id-1","order_id":"1001","amount":150000}`),
	})
	gw, _ := idpay.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	res, err := gw.Inquiry(context.Background(), core.InquiryRequest{Token: "id-1", OrderID: "1001"})
	if err != nil {
		t.Fatalf("Inquiry() error = %v", err)
	}
	if res.Status != core.StatusPaid {
		t.Fatalf("status = %v, want paid", res.Status)
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := idpay.New(core.Config{MerchantKey: "k"})
	body := strings.NewReader("status=10&track_id=9911&id=id-1&order_id=1001&card_no=603799******1234")
	request := httptest.NewRequest(http.MethodPost, "/cb", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.Token != "id-1" || callback.ReferenceNumber != "9911" {
		t.Fatalf("callback = %+v", callback)
	}
}

func TestVerifyDetectsAmountMismatch(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v1.1/payment/verify": testutil.JSON(`{"status":100,"track_id":9911,"id":"id-1","order_id":"1001",
			"amount":15000,"payment":{"track_id":9911,"amount":15000}}`),
	})
	gw, _ := idpay.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token: "id-1", OrderID: "1001", Amount: core.Rial(150_000),
	})
	if !errors.Is(err, core.ErrAmountMismatch) {
		t.Fatalf("error = %v, want ErrAmountMismatch", err)
	}
}

func TestVerifyReportsAlreadyVerified(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v1.1/payment/verify": testutil.JSON(`{"status":101,"id":"id-1","order_id":"1001","amount":150000}`),
	})
	gw, _ := idpay.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token: "id-1", OrderID: "1001", Amount: core.Rial(150_000),
	})
	if !errors.Is(err, core.ErrAlreadyVerified) {
		t.Fatalf("error = %v, want ErrAlreadyVerified", err)
	}
}
