package sadad_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/sadad"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

// terminalKey is a valid base64 encoded 24 byte 3DES key.
var terminalKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef01234567"))

func newGateway(t *testing.T, baseURL string, opts ...core.Option) core.Gateway {
	t.Helper()
	gw, err := sadad.New(core.Config{
		TerminalID:  "12345678",
		MerchantID:  "87654321",
		MerchantKey: terminalKey,
	}, append([]core.Option{core.WithBaseURL(baseURL)}, opts...)...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return gw
}

func TestPurchase(t *testing.T) {
	var sent struct {
		TerminalID string `json:"TerminalId"`
		MerchantID string `json:"MerchantId"`
		Amount     int64  `json:"Amount"`
		SignData   string `json:"SignData"`
		OrderID    string `json:"OrderId"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/vpg/api/v0/Request/PaymentRequest": testutil.Capture(t, &sent,
			`{"ResCode":0,"Token":"token-1","Description":"success"}`),
	})

	gw := newGateway(t, server.URL)
	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount:      core.Rial(150_000),
		OrderID:     "1001",
		CallbackURL: "https://shop.example/callback",
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	switch {
	case sent.TerminalID != "12345678" || sent.MerchantID != "87654321":
		t.Errorf("credentials = %+v", sent)
	case sent.SignData == "":
		t.Error("the 3DES signature is missing")
	case res.Token != "token-1":
		t.Errorf("token = %q", res.Token)
	case res.Redirect.Params["Token"] != "token-1":
		t.Errorf("redirect params = %v", res.Redirect.Params)
	case !strings.HasSuffix(res.Redirect.URL, "/VPG/Purchase"):
		t.Errorf("redirect = %q", res.Redirect.URL)
	}
}

func TestPurchaseFailure(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/vpg/api/v0/Request/PaymentRequest": testutil.JSON(`{"ResCode":"3","Description":"terminal is invalid"}`),
	})
	gw := newGateway(t, server.URL)

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(150_000), OrderID: "1001", CallbackURL: "https://shop.example/cb",
	})
	var typed *core.Error
	if !errors.As(err, &typed) || typed.Code != "3" {
		t.Fatalf("error = %v, want the Sadad result code", err)
	}
}

func TestVerify(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/vpg/api/v0/Advice/Verify": testutil.JSON(`{"ResCode":0,"Amount":150000,"OrderId":1001,
			"RetrivalRefNo":"RRN-3","SystemTraceNo":"TRACE-3","CardNumberMasked":"603799******1234"}`),
	})
	gw := newGateway(t, server.URL)

	res, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "token-1", Amount: core.Rial(150_000)})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	switch {
	case res.ReferenceNumber != "RRN-3":
		t.Errorf("reference = %q", res.ReferenceNumber)
	case res.OrderID != "1001":
		t.Errorf("order = %q", res.OrderID)
	case res.CardNumber != "603799******1234":
		t.Errorf("card = %q", res.CardNumber)
	}
}

func TestParseCallback(t *testing.T) {
	gw := newGateway(t, "https://example.invalid")
	body := strings.NewReader("token=token-1&OrderId=1001&ResCode=0")
	request := httptest.NewRequest(http.MethodPost, "/cb", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.Token != "token-1" || callback.OrderID != "1001" {
		t.Fatalf("callback = %+v", callback)
	}
}

func TestNewValidatesCredentials(t *testing.T) {
	if _, err := sadad.New(core.Config{MerchantID: "m", MerchantKey: terminalKey}); !errors.Is(err, core.ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
}
