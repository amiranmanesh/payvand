package mellat_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/mellat"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

// soapService answers every operation with the queued result, in call order.
func soapService(t *testing.T, actions *[]string, results ...string) http.HandlerFunc {
	t.Helper()
	call := 0
	return func(w http.ResponseWriter, r *http.Request) {
		*actions = append(*actions, r.Header.Get("SOAPAction"))
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "terminalId") {
			t.Errorf("the envelope carries no terminalId:\n%s", body)
		}
		result := results[len(results)-1]
		if call < len(results) {
			result = results[call]
		}
		call++
		testutil.SOAPBody(`<ns1:response><return>`+result+`</return></ns1:response>`)(w, r)
	}
}

func newGateway(t *testing.T, baseURL string, opts ...core.Option) core.Gateway {
	t.Helper()
	gw, err := mellat.New(core.Config{
		TerminalID: "1234567",
		Username:   "user",
		Password:   "pass",
	}, append([]core.Option{core.WithBaseURL(baseURL)}, opts...)...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return gw
}

func TestPurchase(t *testing.T) {
	var actions []string
	server := testutil.NewServer(t, testutil.Routes{
		"/pgwchannel/services/pgw": soapService(t, &actions, "0,3F2504E0"),
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
	case res.Token != "3F2504E0":
		t.Errorf("token = %q", res.Token)
	case res.Redirect.Method != http.MethodPost:
		t.Errorf("redirect method = %q, want POST", res.Redirect.Method)
	case res.Redirect.Params["RefId"] != "3F2504E0":
		t.Errorf("redirect params = %v", res.Redirect.Params)
	case actions[0] != "bpPayRequest":
		t.Errorf("SOAPAction = %q", actions[0])
	}
}

func TestPurchaseFailure(t *testing.T) {
	var actions []string
	server := testutil.NewServer(t, testutil.Routes{
		"/pgwchannel/services/pgw": soapService(t, &actions, "421"),
	})
	gw := newGateway(t, server.URL)

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(1000), OrderID: "1", CallbackURL: "https://shop.example/cb",
	})
	var typed *core.Error
	if !errors.As(err, &typed) || typed.Code != "421" {
		t.Fatalf("error = %v, want the Mellat result code", err)
	}
	if !strings.Contains(err.Error(), "ip address") {
		t.Fatalf("the decoded message is missing: %v", err)
	}
}

func TestVerifyCallsVerifyThenSettle(t *testing.T) {
	var actions []string
	server := testutil.NewServer(t, testutil.Routes{
		"/pgwchannel/services/pgw": soapService(t, &actions, "0", "0"),
	})
	gw := newGateway(t, server.URL)

	res, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token:           "3F2504E0",
		OrderID:         "1001",
		ReferenceNumber: "9911",
		Amount:          core.Rial(150_000),
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if len(actions) != 2 || actions[0] != "bpVerifyRequest" || actions[1] != "bpSettleRequest" {
		t.Fatalf("actions = %v, want verify then settle", actions)
	}
	if res.ReferenceNumber != "9911" {
		t.Errorf("reference = %q", res.ReferenceNumber)
	}
}

func TestVerifyWithoutSettle(t *testing.T) {
	var actions []string
	server := testutil.NewServer(t, testutil.Routes{
		"/pgwchannel/services/pgw": soapService(t, &actions, "0"),
	})
	gw := newGateway(t, server.URL, mellat.WithoutSettle(true))

	if _, err := gw.Verify(context.Background(), core.VerifyRequest{
		OrderID: "1001", ReferenceNumber: "9911", Amount: core.Rial(1000),
	}); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("actions = %v, want only the verify call", actions)
	}
}

func TestRefund(t *testing.T) {
	var actions []string
	server := testutil.NewServer(t, testutil.Routes{
		"/pgwchannel/services/pgw": soapService(t, &actions, "0"),
	})
	gw := newGateway(t, server.URL)

	if _, err := gw.Refund(context.Background(), core.RefundRequest{
		OrderID: "1001", ReferenceNumber: "9911",
	}); err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
	if actions[0] != "bpReversalRequest" {
		t.Fatalf("SOAPAction = %q", actions[0])
	}
}

func TestParseCallback(t *testing.T) {
	gw := newGateway(t, "https://example.invalid")
	body := strings.NewReader("RefId=3F2504E0&ResCode=0&saleOrderId=1001&SaleReferenceId=9911&CardHolderPan=603799******1234")
	request := httptest.NewRequest(http.MethodPost, "/cb", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	switch {
	case !callback.Succeeded:
		t.Error("a ResCode of 0 must be reported as a success")
	case callback.ReferenceNumber != "9911":
		t.Errorf("reference = %q", callback.ReferenceNumber)
	case callback.CardNumber != "603799******1234":
		t.Errorf("card = %q", callback.CardNumber)
	}
}

func TestVerifySettlesAfterAnAlreadyVerifiedTransaction(t *testing.T) {
	var actions []string
	server := testutil.NewServer(t, testutil.Routes{
		// 43: an earlier verify went through and its answer was lost. Mellat
		// still reverses the transaction unless it is settled.
		"/pgwchannel/services/pgw": soapService(t, &actions, "43", "0"),
	})
	gw := newGateway(t, server.URL)

	res, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token:           "3F2504E0",
		OrderID:         "1001",
		ReferenceNumber: "9911",
		Amount:          core.Rial(150_000),
	})
	if err != nil {
		t.Fatalf("Verify() error = %v, want the retry to settle", err)
	}
	if len(actions) != 2 || actions[1] != "bpSettleRequest" {
		t.Fatalf("actions = %v, want the settle call to follow", actions)
	}
	if res.ReferenceNumber != "9911" {
		t.Errorf("reference = %q", res.ReferenceNumber)
	}
}

func TestVerifyReportsAFullySettledTransaction(t *testing.T) {
	var actions []string
	server := testutil.NewServer(t, testutil.Routes{
		"/pgwchannel/services/pgw": soapService(t, &actions, "43", "45"),
	})
	gw := newGateway(t, server.URL)

	_, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token:           "3F2504E0",
		OrderID:         "1001",
		ReferenceNumber: "9911",
		Amount:          core.Rial(150_000),
	})
	if !errors.Is(err, core.ErrAlreadyVerified) {
		t.Fatalf("error = %v, want ErrAlreadyVerified", err)
	}
}

func TestInquiryReportsAVerifiedTransaction(t *testing.T) {
	var actions []string
	server := testutil.NewServer(t, testutil.Routes{
		"/pgwchannel/services/pgw": soapService(t, &actions, "43"),
	})
	gw := newGateway(t, server.URL)

	res, err := gw.Inquiry(context.Background(), core.InquiryRequest{
		OrderID: "1001", ReferenceNumber: "9911",
	})
	if err != nil {
		t.Fatalf("Inquiry() error = %v", err)
	}
	if res.Status != core.StatusPaid {
		t.Fatalf("status = %v, want paid", res.Status)
	}
}
