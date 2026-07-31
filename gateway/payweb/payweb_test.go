package payweb_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/payweb"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

func TestPurchase(t *testing.T) {
	var authorization string
	var sent struct {
		Amount      int64  `json:"amount"`
		ClientRefID string `json:"clientRefId"`
		CardNumber  string `json:"cardNumber"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/Payment/create": func(w http.ResponseWriter, r *http.Request) {
			authorization = r.Header.Get("Authorization")
			testutil.Capture(t, &sent, `{"code":"PW-1"}`)(w, r)
		},
	})

	gw, err := payweb.New(core.Config{MerchantKey: "token-1"},
		core.WithBaseURL(server.URL), payweb.WithCardRestriction(true))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount:       core.Rial(150_000),
		OrderID:      "1001",
		CallbackURL:  "https://shop.example/callback",
		AllowedCards: []string{"6037991234567890"},
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	switch {
	case authorization != "Bearer token-1":
		t.Errorf("Authorization = %q", authorization)
	case sent.ClientRefID != "1001":
		t.Errorf("clientRefId = %q", sent.ClientRefID)
	case sent.CardNumber != "6037991234567890":
		t.Errorf("cardNumber = %q", sent.CardNumber)
	case res.Redirect.Params["id"] != "PW-1":
		t.Errorf("redirect params = %v", res.Redirect.Params)
	}
}

func TestVerify(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/Payment/verify": testutil.JSON(`{"amount":150000,"cardNumber":"603799******1234","trackingCode":"TR-9"}`),
	})
	gw, _ := payweb.New(core.Config{MerchantKey: "t"}, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "PW-1", Amount: core.Rial(150_000)})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if res.ReferenceNumber != "TR-9" || res.Amount.Rial() != 150_000 {
		t.Fatalf("unexpected response %+v", res)
	}
}

func TestVerifyFailure(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/Payment/verify": testutil.Status(http.StatusBadRequest, `{"message":"invalid ref"}`),
	})
	gw, _ := payweb.New(core.Config{MerchantKey: "t"}, core.WithBaseURL(server.URL))

	if _, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "PW-1"}); !errors.Is(err, core.ErrPaymentFailed) {
		t.Fatalf("error = %v, want ErrPaymentFailed", err)
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := payweb.New(core.Config{MerchantKey: "t"})
	request := httptest.NewRequest(http.MethodGet, "/cb?refId=PW-1&status=OK", nil)

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.Token != "PW-1" {
		t.Fatalf("callback = %+v", callback)
	}
}
