package parsian_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/parsian"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

// capturingSOAP records the request envelope and answers with payload.
func capturingSOAP(t *testing.T, envelope *string, payload string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading the soap request: %v", err)
		}
		*envelope = string(body)
		testutil.SOAPBody(payload)(w, r)
	}
}

func TestPurchaseUsesPlainSaleService(t *testing.T) {
	var envelope string
	server := testutil.NewServer(t, testutil.Routes{
		"/NewIPGServices/Sale/SaleService.asmx": capturingSOAP(t, &envelope,
			`<SalePaymentRequestResponse><SalePaymentRequestResult><Token>987654</Token><Status>0</Status>
			</SalePaymentRequestResult></SalePaymentRequestResponse>`),
	})

	gw, err := parsian.New(core.Config{MerchantKey: "login-account"}, core.WithBaseURL(server.URL))
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
	case !strings.Contains(envelope, "<ns:LoginAccount>login-account</ns:LoginAccount>"):
		t.Errorf("the login account is missing from the envelope:\n%s", envelope)
	case !strings.Contains(envelope, "<ns:Amount>150000</ns:Amount>"):
		t.Errorf("the amount is missing from the envelope:\n%s", envelope)
	case res.Token != "987654":
		t.Errorf("token = %q", res.Token)
	case res.Redirect.Params["Token"] != "987654":
		t.Errorf("redirect params = %v", res.Redirect.Params)
	}
}

func TestPurchaseUsesMultiplexedServiceWhenSharesAreSet(t *testing.T) {
	var envelope string
	server := testutil.NewServer(t, testutil.Routes{
		"/NewIPGServices/MultiplexedSale/OnlineMultiplexedSalePaymentService.asmx": capturingSOAP(t, &envelope,
			`<MultiplexedSaleWithIBANPaymentRequestResponse><MultiplexedSaleWithIBANPaymentRequestResult>
			<Token>555</Token><Status>0</Status></MultiplexedSaleWithIBANPaymentRequestResult>
			</MultiplexedSaleWithIBANPaymentRequestResponse>`),
	})

	gw, _ := parsian.New(core.Config{MerchantKey: "login", IBAN: "IR000000000000000000000001"},
		core.WithBaseURL(server.URL), parsian.WithSettlementToIBAN(true))

	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount:      core.Rial(150_000),
		OrderID:     "1001",
		CallbackURL: "https://shop.example/callback",
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	if !strings.Contains(envelope, "<ns:IBAN>IR000000000000000000000001</ns:IBAN>") {
		t.Errorf("the settlement IBAN is missing:\n%s", envelope)
	}
	if res.Token != "555" {
		t.Errorf("token = %q", res.Token)
	}
}

func TestPurchaseFailure(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/NewIPGServices/Sale/SaleService.asmx": testutil.SOAPBody(
			`<SalePaymentRequestResponse><SalePaymentRequestResult><Token>0</Token><Status>-138</Status>
			<Message>canceled</Message></SalePaymentRequestResult></SalePaymentRequestResponse>`),
	})
	gw, _ := parsian.New(core.Config{MerchantKey: "login"}, core.WithBaseURL(server.URL))

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(1000), OrderID: "1", CallbackURL: "https://shop.example/cb",
	})
	var typed *core.Error
	if !errors.As(err, &typed) || typed.Code != "-138" {
		t.Fatalf("error = %v, want the Parsian status", err)
	}
}

func TestVerify(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/NewIPGServices/Confirm/ConfirmService.asmx": testutil.SOAPBody(
			`<ConfirmPaymentResponse><ConfirmPaymentResult><Status>0</Status><RRN>123456789</RRN>
			<CardNumberMasked>603799******1234</CardNumberMasked></ConfirmPaymentResult></ConfirmPaymentResponse>`),
	})
	gw, _ := parsian.New(core.Config{MerchantKey: "login"}, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token: "987654", OrderID: "1001", Amount: core.Rial(150_000),
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if res.ReferenceNumber != "123456789" || res.CardNumber != "603799******1234" {
		t.Fatalf("response = %+v", res)
	}
}

func TestVerifyWithoutReferenceNumberFails(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/NewIPGServices/Confirm/ConfirmService.asmx": testutil.SOAPBody(
			`<ConfirmPaymentResponse><ConfirmPaymentResult><Status>0</Status><RRN>0</RRN>
			</ConfirmPaymentResult></ConfirmPaymentResponse>`),
	})
	gw, _ := parsian.New(core.Config{MerchantKey: "login"}, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "987654", OrderID: "1001"})
	if !errors.Is(err, core.ErrUnexpectedResponse) {
		t.Fatalf("error = %v, want ErrUnexpectedResponse", err)
	}
}

func TestRefund(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/NewIPGServices/Reverse/ReversalService.asmx": testutil.SOAPBody(
			`<ReversalRequestResponse><ReversalRequestResult><Status>0</Status><Token>987654</Token>
			</ReversalRequestResult></ReversalRequestResponse>`),
	})
	gw, _ := parsian.New(core.Config{MerchantKey: "login"}, core.WithBaseURL(server.URL))

	if _, err := gw.Refund(context.Background(), core.RefundRequest{Token: "987654"}); err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := parsian.New(core.Config{MerchantKey: "login"})
	body := strings.NewReader("Token=987654&status=0&OrderId=1001&RRN=123456789")
	request := httptest.NewRequest(http.MethodPost, "/cb", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.Token != "987654" || callback.ReferenceNumber != "123456789" {
		t.Fatalf("callback = %+v", callback)
	}
}
