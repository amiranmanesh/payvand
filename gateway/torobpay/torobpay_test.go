package torobpay_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/torobpay"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

// merchant is the credential set every test builds a gateway from.
var merchant = core.Config{
	Username:    "merchant-user",
	Password:    "merchant-pass",
	MerchantID:  "client-id",
	MerchantKey: "client-secret",
}

// oauth is the token route shared by the authenticated tests.
func oauth() (string, http.HandlerFunc) {
	return "/api/online/v1/oauth/token",
		testutil.JSON(`{"access_token":"access-1","token_type":"bearer","expires_in":3600}`)
}

func TestPurchase(t *testing.T) {
	var sent struct {
		Amount               int64  `json:"amount"`
		PaymentMethodTypeDto string `json:"paymentMethodTypeDto"`
		TransactionID        string `json:"transactionId"`
		ReturnURL            string `json:"returnURL"`
		CartList             []struct {
			CartID      string `json:"cartId"`
			TotalAmount int64  `json:"totalAmount"`
			TaxAmount   int64  `json:"tax_amount"`
			CartItems   []struct {
				Name   string `json:"name"`
				Amount int64  `json:"amount"`
			} `json:"cartItems"`
		} `json:"cartList"`
	}
	var authorization string

	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath: oauthHandler,
		"/api/online/payment/v1/token": func(w http.ResponseWriter, r *http.Request) {
			authorization = r.Header.Get("Authorization")
			testutil.Capture(t, &sent, `{"successful":true,"response":{"paymentToken":"pt-1",
				"paymentPageUrl":"https://api.torobpay.com/pay?token=pt-1"}}`)(w, r)
		},
	})

	gw, err := torobpay.New(merchant, core.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount:      core.Toman(200_000),
		OrderID:     "2002",
		CallbackURL: "https://shop.example/callback",
		Mobile:      "09121234567",
		Description: "Coffee machine",
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}

	switch {
	case authorization != "Bearer access-1":
		t.Errorf("Authorization = %q", authorization)
	case sent.Amount != 2_000_000:
		t.Errorf("amount = %d, want Rial", sent.Amount)
	case sent.PaymentMethodTypeDto != "ONLINE_CREDIT":
		t.Errorf("paymentMethodTypeDto = %q", sent.PaymentMethodTypeDto)
	case sent.TransactionID != "2002":
		t.Errorf("transactionId = %q", sent.TransactionID)
	case len(sent.CartList) != 1 || sent.CartList[0].CartID != "2002":
		t.Fatalf("cartList = %+v", sent.CartList)
	case sent.CartList[0].CartItems[0].Name != "Coffee machine":
		t.Errorf("cart item name = %q", sent.CartList[0].CartItems[0].Name)
	case res.Token != "pt-1":
		t.Errorf("token = %q", res.Token)
	}
}

func TestPurchaseFailureCarriesTheProviderMessage(t *testing.T) {
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath: oauthHandler,
		"/api/online/payment/v1/token": testutil.Status(http.StatusBadRequest,
			`{"successful":false,"result":{"code":"1010","message":"merchant is not active"}}`),
	})
	gw, _ := torobpay.New(merchant, core.WithBaseURL(server.URL))

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(2_000_000), OrderID: "1", CallbackURL: "https://shop.example/cb",
	})
	if !errors.Is(err, core.ErrPaymentFailed) {
		t.Fatalf("error = %v, want ErrPaymentFailed", err)
	}

	var gwErr *core.Error
	if !errors.As(err, &gwErr) || gwErr.Message != "merchant is not active" {
		t.Fatalf("error = %+v, want the provider message", gwErr)
	}
}

func TestVerify(t *testing.T) {
	var sent struct {
		PaymentToken string `json:"paymentToken"`
	}
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath: oauthHandler,
		"/api/online/payment/v1/verify": testutil.Capture(t, &sent,
			`{"successful":true,"response":{"transactionId":"tx-4","amount":2000000,"referenceNumber":"rrn-3"}}`),
	})
	gw, _ := torobpay.New(merchant, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "pt-1", OrderID: "2002"})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	switch {
	case sent.PaymentToken != "pt-1":
		t.Errorf("paymentToken = %q", sent.PaymentToken)
	case res.TransactionID != "tx-4":
		t.Errorf("transaction = %q", res.TransactionID)
	case res.ReferenceNumber != "rrn-3":
		t.Errorf("reference = %q", res.ReferenceNumber)
	case res.Amount.Rial() != 2_000_000:
		t.Errorf("amount = %v", res.Amount)
	}
}

func TestVerifyRejectsAnUnsuccessfulAnswer(t *testing.T) {
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath: oauthHandler,
		"/api/online/payment/v1/verify": testutil.JSON(
			`{"successful":false,"error":{"code":"2001","message":"transaction not paid"}}`),
	})
	gw, _ := torobpay.New(merchant, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "pt-1"})
	if !errors.Is(err, core.ErrPaymentFailed) {
		t.Fatalf("error = %v, want ErrPaymentFailed", err)
	}
}

func TestRefund(t *testing.T) {
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath: oauthHandler,
		"/api/online/payment/v1/revert": testutil.JSON(
			`{"successful":true,"response":{"transactionId":"tx-4","amount":2000000}}`),
	})
	gw, _ := torobpay.New(merchant, core.WithBaseURL(server.URL))

	res, err := gw.Refund(context.Background(), core.RefundRequest{Token: "pt-1"})
	if err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
	if res.RefundID != "tx-4" || res.Amount.Rial() != 2_000_000 {
		t.Fatalf("refund = %+v", res)
	}
}

func TestCancel(t *testing.T) {
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath:                       oauthHandler,
		"/api/online/payment/v1/cancel": testutil.JSON(`{"successful":true,"response":{}}`),
	})
	gw, _ := torobpay.New(merchant, core.WithBaseURL(server.URL))

	if err := gw.Cancel(context.Background(), "pt-1"); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
}

func TestInquiry(t *testing.T) {
	var query string
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath: oauthHandler,
		"/api/online/payment/v1/status": func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.RawQuery
			testutil.JSON(`{"successful":true,"response":{"status":"VERIFY","amount":2000000}}`)(w, r)
		},
	})
	gw, _ := torobpay.New(merchant, core.WithBaseURL(server.URL))

	res, err := gw.Inquiry(context.Background(), core.InquiryRequest{Token: "pt-1"})
	if err != nil {
		t.Fatalf("Inquiry() error = %v", err)
	}
	if query != "paymentToken=pt-1" {
		t.Errorf("query = %q", query)
	}
	if res.Status != core.StatusVerified {
		t.Errorf("status = %v", res.Status)
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := torobpay.New(merchant)
	request := httptest.NewRequest(http.MethodGet,
		"/cb?paymentToken=pt-1&transactionId=2002&status=OK", nil)

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.Token != "pt-1" || callback.OrderID != "2002" {
		t.Fatalf("callback = %+v", callback)
	}
}

func TestNewValidatesTheCredentials(t *testing.T) {
	if _, err := torobpay.New(core.Config{Username: "u", Password: "p"}); !errors.Is(err, core.ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
}

func TestVerifyDetectsAmountMismatch(t *testing.T) {
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath: oauthHandler,
		"/api/online/payment/v1/verify": testutil.JSON(
			`{"successful":true,"response":{"amount":15000,"transactionId":"tr-1"}}`),
	})
	gw, _ := torobpay.New(merchant, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "tok-1", Amount: core.Rial(150_000)})
	if !errors.Is(err, core.ErrAmountMismatch) {
		t.Fatalf("error = %v, want ErrAmountMismatch", err)
	}
}
