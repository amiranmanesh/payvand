package nextpay_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/nextpay"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

func TestPurchase(t *testing.T) {
	var sent struct {
		APIKey     string `json:"api_key"`
		OrderID    string `json:"order_id"`
		Amount     int64  `json:"amount"`
		AutoVerify string `json:"auto_verify"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/nx/gateway/token": testutil.Capture(t, &sent, `{"code":-1,"trans_id":"trans-1"}`),
	})

	gw, err := nextpay.New(core.Config{MerchantKey: "api-key"},
		core.WithBaseURL(server.URL), nextpay.WithAutoVerify(true))
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
	case sent.APIKey != "api-key" || sent.OrderID != "1001" || sent.Amount != 150_000:
		t.Errorf("request = %+v", sent)
	case sent.AutoVerify != "yes":
		t.Errorf("auto_verify = %q", sent.AutoVerify)
	case res.Redirect.URL != server.URL+"/nx/gateway/payment/trans-1":
		t.Errorf("redirect = %q", res.Redirect.URL)
	}
}

func TestPurchaseFailure(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/nx/gateway/token": testutil.JSON(`{"code":-20}`),
	})
	gw, _ := nextpay.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(10_000), OrderID: "1", CallbackURL: "https://shop.example/cb",
	})
	if !errors.Is(err, core.ErrPaymentFailed) || !strings.Contains(err.Error(), "api key") {
		t.Fatalf("error = %v, want the decoded NextPay message", err)
	}
}

func TestVerifyAndRefund(t *testing.T) {
	var refundFlag string
	server := testutil.NewServer(t, testutil.Routes{
		"/nx/gateway/verify": func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				RefundRequest string `json:"refund_request"`
			}
			testutil.Capture(t, &body, `{"code":0,"amount":150000,"order_id":"1001","Shaparak_Ref_Id":"RRN-7",
				"card_holder":"603799******1234"}`)(w, r)
			refundFlag = body.RefundRequest
		},
	})
	gw, _ := nextpay.New(core.Config{MerchantKey: "k"}, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "trans-1", Amount: core.Rial(150_000)})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if res.ReferenceNumber != "RRN-7" || refundFlag != "" {
		t.Fatalf("verify must not set the refund flag, got %+v / %q", res, refundFlag)
	}

	if _, err := gw.Refund(context.Background(), core.RefundRequest{
		Token: "trans-1", Amount: core.Rial(150_000),
	}); err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
	if refundFlag != "yes_money_back" {
		t.Fatalf("refund_request = %q", refundFlag)
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := nextpay.New(core.Config{MerchantKey: "k"})
	body := strings.NewReader("trans_id=trans-1&order_id=1001&amount=150000")
	request := httptest.NewRequest(http.MethodPost, "/cb", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.Token != "trans-1" || callback.OrderID != "1001" {
		t.Fatalf("callback = %+v", callback)
	}
}
