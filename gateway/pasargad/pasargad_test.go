package pasargad_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/pasargad"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

// base64Bytes encodes a key component the way .NET serialises it.
func base64Bytes(value []byte) string { return base64.StdEncoding.EncodeToString(value) }

// privateKeyPEM builds the RSA private key the request bodies are signed with.
func privateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating the test key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
}

func newGateway(t *testing.T, baseURL string, opts ...core.Option) core.Gateway {
	t.Helper()
	gw, err := pasargad.New(core.Config{
		MerchantID:  "merchant-1",
		TerminalID:  "terminal-1",
		MerchantKey: privateKeyPEM(t),
	}, append([]core.Option{core.WithBaseURL(baseURL)}, opts...)...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return gw
}

func TestPurchaseSignsTheBody(t *testing.T) {
	var signature string
	var sent struct {
		InvoiceNumber string `json:"InvoiceNumber"`
		InvoiceDate   string `json:"InvoiceDate"`
		Amount        int64  `json:"Amount"`
		Action        string `json:"Action"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/Api/v1/Payment/GetToken": func(w http.ResponseWriter, r *http.Request) {
			signature = r.Header.Get("Sign")
			testutil.Capture(t, &sent, `{"IsSuccess":true,"Token":"tok-1"}`)(w, r)
		},
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
	case signature == "":
		t.Error("the Sign header is missing")
	case sent.InvoiceNumber != "1001" || sent.Amount != 150_000 || sent.Action != "1003":
		t.Errorf("request = %+v", sent)
	case res.Get(pasargad.InvoiceDateKey) != sent.InvoiceDate:
		t.Errorf("the invoice date was not returned for the caller to persist: %v", res.Extra)
	case res.Redirect.Params["n"] != "tok-1":
		t.Errorf("redirect params = %v", res.Redirect.Params)
	}
}

func TestVerifyChecksThenSettles(t *testing.T) {
	var calls []string
	server := testutil.NewServer(t, testutil.Routes{
		"/Api/v1/Payment/CheckTransactionResult": func(w http.ResponseWriter, _ *http.Request) {
			calls = append(calls, "check")
			_, _ = w.Write([]byte(`{"IsSuccess":true,"Amount":150000,"ReferenceNumber":778899,"TraceNumber":4455,
				"MaskedCardNumber":"603799******1234"}`))
		},
		"/Api/v1/Payment/VerifyPayment": func(w http.ResponseWriter, _ *http.Request) {
			calls = append(calls, "verify")
			_, _ = w.Write([]byte(`{"IsSuccess":true,"ShaparakRefNumber":"RRN-1","MaskedCardNumber":"603799******1234"}`))
		},
	})

	gw := newGateway(t, server.URL)
	res, err := gw.Verify(context.Background(), core.VerifyRequest{
		OrderID:         "1001",
		ReferenceNumber: "tref-1",
		Amount:          core.Rial(150_000),
		Extra:           map[string]string{pasargad.InvoiceDateKey: "2024/05/01 10:20:30"},
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if len(calls) != 2 || calls[0] != "check" || calls[1] != "verify" {
		t.Fatalf("calls = %v, want the check before the settlement", calls)
	}
	if res.ReferenceNumber != "RRN-1" || res.TransactionID != "4455" {
		t.Fatalf("response = %+v", res)
	}
}

func TestVerifyDetectsAmountMismatch(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/Api/v1/Payment/CheckTransactionResult": testutil.JSON(`{"IsSuccess":true,"Amount":10000}`),
	})
	gw := newGateway(t, server.URL)

	_, err := gw.Verify(context.Background(), core.VerifyRequest{
		OrderID:         "1001",
		ReferenceNumber: "tref-1",
		Amount:          core.Rial(150_000),
		Extra:           map[string]string{pasargad.InvoiceDateKey: "2024/05/01 10:20:30"},
	})
	if !errors.Is(err, core.ErrAmountMismatch) {
		t.Fatalf("error = %v, want ErrAmountMismatch", err)
	}
}

func TestVerifyNeedsInvoiceDate(t *testing.T) {
	gw := newGateway(t, "https://example.invalid")
	_, err := gw.Verify(context.Background(), core.VerifyRequest{OrderID: "1001"})
	if !errors.Is(err, core.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestRefund(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/Api/v1/Payment/RefundPayment": testutil.JSON(`{"IsSuccess":true}`),
	})
	gw := newGateway(t, server.URL)

	if _, err := gw.Refund(context.Background(), core.RefundRequest{
		OrderID: "1001",
		Amount:  core.Rial(150_000),
		Extra:   map[string]string{pasargad.InvoiceDateKey: "2024/05/01 10:20:30"},
	}); err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
}

func TestParseCallback(t *testing.T) {
	gw := newGateway(t, "https://example.invalid")
	request := httptest.NewRequest(http.MethodGet, "/cb?iN=1001&iD=2024%2F05%2F01+10%3A20%3A30&tref=tref-1", nil)

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	switch {
	case !callback.Succeeded || callback.OrderID != "1001":
		t.Errorf("callback = %+v", callback)
	case callback.Get(pasargad.InvoiceDateKey) != "2024/05/01 10:20:30":
		t.Errorf("the invoice date was not extracted: %v", callback.Values)
	}
}

func TestNewRejectsBrokenKey(t *testing.T) {
	_, err := pasargad.New(core.Config{MerchantID: "m", TerminalID: "t", MerchantKey: "not-a-key"})
	if !errors.Is(err, core.ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
}

func TestXMLPrivateKeyIsAccepted(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating the test key: %v", err)
	}
	xmlKey := "<RSAKeyValue>" +
		"<Modulus>" + base64Bytes(key.N.Bytes()) + "</Modulus>" +
		"<Exponent>" + base64Bytes([]byte{1, 0, 1}) + "</Exponent>" +
		"<P>" + base64Bytes(key.Primes[0].Bytes()) + "</P>" +
		"<Q>" + base64Bytes(key.Primes[1].Bytes()) + "</Q>" +
		"<D>" + base64Bytes(key.D.Bytes()) + "</D>" +
		"</RSAKeyValue>"

	if _, err := pasargad.New(core.Config{MerchantID: "m", TerminalID: "t", MerchantKey: xmlKey}); err != nil {
		t.Fatalf("New() error = %v, want the .NET XML key to be accepted", err)
	}
}
