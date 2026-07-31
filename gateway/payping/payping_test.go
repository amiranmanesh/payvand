package payping_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/payping"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

func TestPurchaseSendsToman(t *testing.T) {
	var authorization string
	var sent struct {
		Amount      int64  `json:"amount"`
		ClientRefID string `json:"clientRefId"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/v2/pay": func(w http.ResponseWriter, r *http.Request) {
			authorization = r.Header.Get("Authorization")
			testutil.Capture(t, &sent, `{"code":"pp-1"}`)(w, r)
		},
	})

	gw, err := payping.New(core.Config{MerchantKey: "token-1"}, core.WithBaseURL(server.URL))
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
	case authorization != "Bearer token-1":
		t.Errorf("Authorization = %q", authorization)
	case sent.Amount != 15_000:
		t.Errorf("amount = %d, want the Rial amount converted to Toman", sent.Amount)
	case sent.ClientRefID != "1001":
		t.Errorf("clientRefId = %q", sent.ClientRefID)
	case res.Redirect.URL != server.URL+"/v2/pay/gotoipg/pp-1":
		t.Errorf("redirect = %q", res.Redirect.URL)
	}
}

func TestVerify(t *testing.T) {
	var sent struct {
		RefID  string `json:"refId"`
		Amount int64  `json:"amount"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/v2/pay/verify": testutil.Capture(t, &sent,
			`{"amount":15000,"cardNumber":"603799******1234","cardHashPan":"HASH","clientRefId":"1001"}`),
	})
	gw, _ := payping.New(core.Config{MerchantKey: "t"}, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{
		ReferenceNumber: "ref-1", Amount: core.Rial(150_000),
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	switch {
	case sent.RefID != "ref-1" || sent.Amount != 15_000:
		t.Errorf("request = %+v", sent)
	case res.Amount.Toman() != 15_000 || res.CardHash != "HASH" || res.OrderID != "1001":
		t.Errorf("response = %+v", res)
	}
}

func TestVerifyFailure(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v2/pay/verify": testutil.Status(http.StatusBadRequest, `{"Error":"amount mismatch"}`),
	})
	gw, _ := payping.New(core.Config{MerchantKey: "t"}, core.WithBaseURL(server.URL))

	if _, err := gw.Verify(context.Background(), core.VerifyRequest{ReferenceNumber: "ref-1"}); !errors.Is(err, core.ErrPaymentFailed) {
		t.Fatalf("error = %v, want ErrPaymentFailed", err)
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := payping.New(core.Config{MerchantKey: "t"})
	request := httptest.NewRequest(http.MethodGet, "/cb?refid=ref-1&clientrefid=1001&cardnumber=603799******1234", nil)

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.ReferenceNumber != "ref-1" || callback.OrderID != "1001" {
		t.Fatalf("callback = %+v", callback)
	}
}
