package asanpardakht_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/asanpardakht"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

func newGateway(t *testing.T, baseURL string, opts ...core.Option) core.Gateway {
	t.Helper()
	gw, err := asanpardakht.New(core.Config{
		MerchantID: "1234",
		Username:   "usr",
		Password:   "pwd",
	}, append([]core.Option{core.WithBaseURL(baseURL)}, opts...)...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return gw
}

func TestPurchase(t *testing.T) {
	var user, password string
	var sent struct {
		ServiceTypeID  int    `json:"serviceTypeId"`
		LocalInvoiceID string `json:"localInvoiceId"`
		AmountInRials  int64  `json:"amountInRials"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/v1/Token": func(w http.ResponseWriter, r *http.Request) {
			user, password = r.Header.Get("usr"), r.Header.Get("pwd")
			testutil.Capture(t, &sent, `"tok-1"`)(w, r)
		},
	})

	gw := newGateway(t, server.URL)
	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount:      core.Rial(150_000),
		OrderID:     "1001",
		CallbackURL: "https://shop.example/callback",
		Mobile:      "09120000000",
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	switch {
	case user != "usr" || password != "pwd":
		t.Errorf("credentials = %q/%q", user, password)
	case sent.ServiceTypeID != 1 || sent.LocalInvoiceID != "1001" || sent.AmountInRials != 150_000:
		t.Errorf("request = %+v", sent)
	case res.Token != "tok-1":
		t.Errorf("token = %q, the quoted JSON string must be unwrapped", res.Token)
	case res.Redirect.Method != http.MethodPost || res.Redirect.Params["RefId"] != "tok-1":
		t.Errorf("redirect = %+v", res.Redirect)
	case res.Redirect.Params["mobileap"] != "09120000000":
		t.Errorf("the mobile was not forwarded: %v", res.Redirect.Params)
	}
}

func TestVerifyRunsResultVerifyAndSettlement(t *testing.T) {
	var calls []string
	server := testutil.NewServer(t, testutil.Routes{
		"/v1/TranResult": func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, "result")
			if got := r.URL.Query().Get("localInvoiceId"); got != "1001" {
				t.Errorf("localInvoiceId = %q", got)
			}
			_, _ = w.Write([]byte(`{"payGateTranID":556677,"amount":150000,"rrn":"RRN-1",
				"cardNumber":"603799******1234","refID":"ref-1"}`))
		},
		"/v1/Verify": func(w http.ResponseWriter, _ *http.Request) {
			calls = append(calls, "verify")
			w.WriteHeader(http.StatusOK)
		},
		"/v1/Settlement": func(w http.ResponseWriter, _ *http.Request) {
			calls = append(calls, "settlement")
			w.WriteHeader(http.StatusOK)
		},
	})

	gw := newGateway(t, server.URL)
	res, err := gw.Verify(context.Background(), core.VerifyRequest{OrderID: "1001", Amount: core.Rial(150_000)})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if strings.Join(calls, ",") != "result,verify,settlement" {
		t.Fatalf("calls = %v", calls)
	}
	if res.ReferenceNumber != "RRN-1" || res.TransactionID != "556677" {
		t.Fatalf("response = %+v", res)
	}
}

func TestVerifyDetectsAmountMismatch(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v1/TranResult": testutil.JSON(`{"payGateTranID":1,"amount":10000}`),
	})
	gw := newGateway(t, server.URL)

	_, err := gw.Verify(context.Background(), core.VerifyRequest{OrderID: "1001", Amount: core.Rial(150_000)})
	if !errors.Is(err, core.ErrAmountMismatch) {
		t.Fatalf("error = %v, want ErrAmountMismatch", err)
	}
}

func TestVerifyRejectsCallbackSuppliedTranID(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		// The invoice was never paid, so the provider reports no transaction.
		"/v1/TranResult": testutil.JSON(`{"payGateTranID":0,"amount":0}`),
	})
	gw := newGateway(t, server.URL)

	// A payer returning with ?pay_gate_tran_id=999888 lands here through
	// Callback.VerifyRequest, which copies the callback map into Extra.
	_, err := gw.Verify(context.Background(), core.VerifyRequest{
		OrderID: "1001",
		Amount:  core.Rial(150_000),
		Extra:   map[string]string{asanpardakht.PayGateTranIDKey: "999888"},
	})
	if !errors.Is(err, core.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestVerifyRejectsMissingProviderAmount(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v1/TranResult": testutil.JSON(`{"payGateTranID":556677,"amount":0}`),
	})
	gw := newGateway(t, server.URL)

	_, err := gw.Verify(context.Background(), core.VerifyRequest{OrderID: "1001", Amount: core.Rial(150_000)})
	if !errors.Is(err, core.ErrAmountMismatch) {
		t.Fatalf("error = %v, want ErrAmountMismatch", err)
	}
}

func TestRefundRejectsTranIDThatIsNotTheInvoices(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v1/TranResult": testutil.JSON(`{"payGateTranID":556677,"amount":150000}`),
	})
	gw := newGateway(t, server.URL)

	_, err := gw.Refund(context.Background(), core.RefundRequest{
		OrderID: "1001",
		Extra:   map[string]string{asanpardakht.PayGateTranIDKey: "999888"},
	})
	if !errors.Is(err, core.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestRefundReverses(t *testing.T) {
	reversed := false
	server := testutil.NewServer(t, testutil.Routes{
		"/v1/TranResult": testutil.JSON(`{"payGateTranID":556677,"amount":150000}`),
		"/v1/Reverse": func(w http.ResponseWriter, _ *http.Request) {
			reversed = true
			w.WriteHeader(http.StatusOK)
		},
	})
	gw := newGateway(t, server.URL)

	if _, err := gw.Refund(context.Background(), core.RefundRequest{
		OrderID: "1001",
		Extra:   map[string]string{asanpardakht.PayGateTranIDKey: "556677"},
	}); err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
	if !reversed {
		t.Fatal("the reverse endpoint was not called")
	}
}

func TestRefundCancelsWhenConfigured(t *testing.T) {
	canceled := false
	server := testutil.NewServer(t, testutil.Routes{
		"/v1/TranResult": testutil.JSON(`{"payGateTranID":556677,"amount":150000}`),
		"/v1/Cancel": func(w http.ResponseWriter, _ *http.Request) {
			canceled = true
			w.WriteHeader(http.StatusOK)
		},
	})
	gw := newGateway(t, server.URL, asanpardakht.WithCancelInsteadOfReverse(true))

	if _, err := gw.Refund(context.Background(), core.RefundRequest{
		OrderID: "1001",
		Extra:   map[string]string{asanpardakht.PayGateTranIDKey: "556677"},
	}); err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
	if !canceled {
		t.Fatal("the cancel endpoint was not called")
	}
}

func TestParseCallback(t *testing.T) {
	gw := newGateway(t, "https://example.invalid")
	body := strings.NewReader("localInvoiceId=1001&RefId=tok-1")
	request := httptest.NewRequest(http.MethodPost, "/cb", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.OrderID != "1001" || callback.Token != "tok-1" {
		t.Fatalf("callback = %+v", callback)
	}
}
