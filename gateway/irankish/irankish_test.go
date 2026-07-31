package irankish_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/irankish"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

// publicKeyPEM builds the acquirer RSA public key the envelope is sealed with.
func publicKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating the test key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("encoding the test key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func newGateway(t *testing.T, baseURL string, opts ...core.Option) core.Gateway {
	t.Helper()
	gw, err := irankish.New(core.Config{
		// The terminal id and password are hexadecimal per the protocol.
		TerminalID:  "12345678",
		Username:    "acceptor-1",
		Password:    "abcdef0123456789",
		MerchantKey: publicKeyPEM(t),
	}, append([]core.Option{core.WithBaseURL(baseURL)}, opts...)...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return gw
}

func TestPurchase(t *testing.T) {
	var sent struct {
		Request struct {
			AcceptorID      string `json:"acceptorId"`
			Amount          int64  `json:"amount"`
			TerminalID      string `json:"terminalId"`
			TransactionType string `json:"transactionType"`
			RevertURI       string `json:"revertUri"`
		} `json:"request"`
		AuthenticationEnvelope struct {
			Data string `json:"data"`
			IV   string `json:"iv"`
		} `json:"authenticationEnvelope"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/api/v3/tokenization/make": testutil.Capture(t, &sent,
			`{"responseCode":"00","description":"ok","result":{"token":"tok-1"}}`),
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
	case sent.Request.AcceptorID != "acceptor-1" || sent.Request.Amount != 150_000:
		t.Errorf("request = %+v", sent.Request)
	case sent.Request.TransactionType != "Purchase":
		t.Errorf("transactionType = %q", sent.Request.TransactionType)
	case sent.AuthenticationEnvelope.Data == "" || sent.AuthenticationEnvelope.IV == "":
		t.Error("the authentication envelope was not built")
	case res.Redirect.Method != http.MethodPost || res.Redirect.Params["tokenIdentity"] != "tok-1":
		t.Errorf("redirect = %+v", res.Redirect)
	}
}

func TestPurchaseFailure(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/api/v3/tokenization/make": testutil.JSON(`{"responseCode":"915","description":""}`),
	})
	gw := newGateway(t, server.URL)

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(1000), OrderID: "1", CallbackURL: "https://shop.example/cb",
	})
	if !errors.Is(err, core.ErrPaymentFailed) || !strings.Contains(err.Error(), "terminal id is invalid") {
		t.Fatalf("error = %v, want the decoded Iran Kish message", err)
	}
}

func TestVerify(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/api/v3/confirmation/purchase": testutil.JSON(`{"responseCode":"00","result":
			{"retrievalReferenceNumber":"RRN-1","systemTraceAuditNumber":"TRACE-1","amount":150000}}`),
	})
	gw := newGateway(t, server.URL)

	res, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token:           "tok-1",
		ReferenceNumber: "RRN-1",
		TraceNumber:     "TRACE-1",
		Amount:          core.Rial(150_000),
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if res.ReferenceNumber != "RRN-1" || res.TransactionID != "TRACE-1" {
		t.Fatalf("response = %+v", res)
	}
}

func TestVerifyDetectsAmountMismatch(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/api/v3/confirmation/purchase": testutil.JSON(`{"responseCode":"00","result":
			{"retrievalReferenceNumber":"RRN-1","systemTraceAuditNumber":"TRACE-1","amount":15000}}`),
	})
	gw := newGateway(t, server.URL)

	_, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token:           "tok-1",
		ReferenceNumber: "RRN-1",
		TraceNumber:     "TRACE-1",
		Amount:          core.Rial(150_000),
	})
	if !errors.Is(err, core.ErrAmountMismatch) {
		t.Fatalf("error = %v, want ErrAmountMismatch", err)
	}
}

func TestVerifyNeedsCallbackFields(t *testing.T) {
	gw := newGateway(t, "https://example.invalid")
	if _, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "tok-1"}); !errors.Is(err, core.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestParseCallback(t *testing.T) {
	gw := newGateway(t, "https://example.invalid")
	body := strings.NewReader("token=tok-1&responseCode=00&retrievalReferenceNumber=RRN-1&systemTraceAuditNumber=TRACE-1")
	request := httptest.NewRequest(http.MethodPost, "/cb", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.ReferenceNumber != "RRN-1" || callback.TraceNumber != "TRACE-1" {
		t.Fatalf("callback = %+v", callback)
	}
}

func TestNewValidatesCredentials(t *testing.T) {
	if _, err := irankish.New(core.Config{TerminalID: "1"}); !errors.Is(err, core.ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
}
