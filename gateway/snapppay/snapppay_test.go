package snapppay_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/snapppay"
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
		Mobile               string `json:"mobile"`
		PaymentMethodTypeDto string `json:"paymentMethodTypeDto"`
		TransactionID        string `json:"transactionId"`
		ReturnURL            string `json:"returnURL"`
		DiscountAmount       int64  `json:"discountAmount"`
		CartList             []struct {
			TotalAmount int64 `json:"totalAmount"`
			CartItems   []struct {
				Name     string `json:"name"`
				Amount   int64  `json:"amount"`
				Category string `json:"category"`
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
				"paymentPageUrl":"https://api.snapppay.ir/online/pay?token=pt-1"}}`)(w, r)
		},
	})

	gw, err := snapppay.New(merchant, core.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount:      core.Toman(150_000),
		OrderID:     "1001",
		CallbackURL: "https://shop.example/callback",
		Mobile:      "09121234567",
		Description: "Winter jacket",
		Metadata:    map[string]string{snapppay.DiscountKey: "10000"},
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}

	switch {
	case authorization != "Bearer access-1":
		t.Errorf("Authorization = %q", authorization)
	case sent.Amount != 1_500_000:
		t.Errorf("amount = %d, want Rial", sent.Amount)
	case sent.Mobile != "+989121234567":
		t.Errorf("mobile = %q", sent.Mobile)
	case sent.PaymentMethodTypeDto != "INSTALLMENT":
		t.Errorf("paymentMethodTypeDto = %q", sent.PaymentMethodTypeDto)
	case sent.TransactionID != "1001":
		t.Errorf("transactionId = %q", sent.TransactionID)
	case sent.DiscountAmount != 100_000:
		t.Errorf("discountAmount = %d, want the Toman value in Rial", sent.DiscountAmount)
	case len(sent.CartList) != 1 || len(sent.CartList[0].CartItems) != 1:
		t.Fatalf("cartList = %+v", sent.CartList)
	case sent.CartList[0].CartItems[0].Name != "Winter jacket":
		t.Errorf("cart item name = %q", sent.CartList[0].CartItems[0].Name)
	case res.Token != "pt-1":
		t.Errorf("token = %q", res.Token)
	case res.Redirect.Method != http.MethodGet:
		t.Errorf("redirect method = %q", res.Redirect.Method)
	}
}

func TestPurchaseUsesTheConfiguredCartBuilder(t *testing.T) {
	var sent struct {
		CartList []struct {
			CartID    int64 `json:"cartId"`
			CartItems []struct {
				ID    string `json:"id"`
				Count int    `json:"count"`
			} `json:"cartItems"`
		} `json:"cartList"`
	}
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath: oauthHandler,
		"/api/online/payment/v1/token": testutil.Capture(t, &sent,
			`{"successful":true,"response":{"paymentToken":"pt","paymentPageUrl":"https://pay"}}`),
	})

	gw, _ := snapppay.New(merchant, core.WithBaseURL(server.URL),
		snapppay.WithCartBuilder(func(req core.PurchaseRequest) []snapppay.Cart {
			return []snapppay.Cart{{
				CartID:      7,
				TotalAmount: req.Amount.Rial(),
				CartItems: []snapppay.CartItem{
					{ID: "sku-1", Name: "Shoes", Count: 2, Amount: 500_000, Category: "SHOES"},
				},
			}}
		}))

	if _, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(1_000_000), OrderID: "1", CallbackURL: "https://shop.example/cb",
	}); err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	if len(sent.CartList) != 1 || sent.CartList[0].CartID != 7 || sent.CartList[0].CartItems[0].Count != 2 {
		t.Fatalf("cartList = %+v", sent.CartList)
	}
}

func TestPurchaseFailureCarriesTheProviderCode(t *testing.T) {
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath: oauthHandler,
		"/api/online/payment/v1/token": testutil.JSON(
			`{"successful":false,"errorData":{"errorCode":1011,"message":"amount is not eligible"}}`),
	})
	gw, _ := snapppay.New(merchant, core.WithBaseURL(server.URL))

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(1_000_000), OrderID: "1", CallbackURL: "https://shop.example/cb",
	})
	if !errors.Is(err, core.ErrPaymentFailed) {
		t.Fatalf("error = %v, want ErrPaymentFailed", err)
	}

	var gwErr *core.Error
	if !errors.As(err, &gwErr) || gwErr.Code != "1011" || gwErr.Message != "amount is not eligible" {
		t.Fatalf("error = %+v, want the provider code and message", gwErr)
	}
}

func TestVerifyAlsoSettles(t *testing.T) {
	verified, settled := false, false
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath: oauthHandler,
		"/api/online/payment/v1/verify": func(w http.ResponseWriter, r *http.Request) {
			verified = true
			testutil.JSON(`{"successful":true,"response":{"transactionId":"tx-9","amount":1500000}}`)(w, r)
		},
		"/api/online/payment/v1/settle": func(w http.ResponseWriter, r *http.Request) {
			settled = true
			testutil.JSON(`{"successful":true,"response":{"transactionId":"tx-9","amount":1500000,
				"referenceNumber":"rrn-77"}}`)(w, r)
		},
	})
	gw, _ := snapppay.New(merchant, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "pt-1", OrderID: "1001"})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	switch {
	case !verified || !settled:
		t.Errorf("verified = %v, settled = %v, want both", verified, settled)
	case res.TransactionID != "tx-9":
		t.Errorf("transaction = %q", res.TransactionID)
	case res.ReferenceNumber != "rrn-77":
		t.Errorf("reference = %q", res.ReferenceNumber)
	case res.Amount.Rial() != 1_500_000:
		t.Errorf("amount = %v", res.Amount)
	}
}

func TestVerifyWithoutAutoSettleLeavesSettlementToTheCaller(t *testing.T) {
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath: oauthHandler,
		"/api/online/payment/v1/verify": testutil.JSON(
			`{"successful":true,"response":{"transactionId":"tx-9","amount":1500000}}`),
	})
	gw, _ := snapppay.New(merchant, core.WithBaseURL(server.URL), snapppay.WithAutoSettle(false))

	// The fake server fails the test on any call to the settle endpoint, so a
	// clean run proves settlement was not attempted.
	if _, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "pt-1"}); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestEligible(t *testing.T) {
	var query string
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath: oauthHandler,
		"/api/online/offer/v1/eligible": func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.RawQuery
			testutil.JSON(`{"successful":true,"response":{"eligible":true,"title":"Pay in 4",
				"minEligibleAmount":1000000,"maxEligibleAmount":200000000,
				"installmentList":[{"count":4,"amount":375000,"wage":0}]}}`)(w, r)
		},
	})
	gw, _ := snapppay.New(merchant, core.WithBaseURL(server.URL))

	eligibility, err := gw.Eligible(context.Background(), core.Toman(150_000))
	if err != nil {
		t.Fatalf("Eligible() error = %v", err)
	}
	switch {
	case query != "amount=1500000":
		t.Errorf("query = %q, want the amount in Rial", query)
	case !eligibility.Eligible:
		t.Error("payer should be eligible")
	case len(eligibility.Plans) != 1 || eligibility.Plans[0].Count != 4:
		t.Errorf("plans = %+v", eligibility.Plans)
	}
}

func TestRefund(t *testing.T) {
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath: oauthHandler,
		"/api/online/payment/v1/revert": testutil.JSON(
			`{"successful":true,"response":{"transactionId":"tx-9","amount":1500000}}`),
	})
	gw, _ := snapppay.New(merchant, core.WithBaseURL(server.URL))

	res, err := gw.Refund(context.Background(), core.RefundRequest{Token: "pt-1"})
	if err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
	if res.RefundID != "tx-9" || res.Amount.Rial() != 1_500_000 {
		t.Fatalf("refund = %+v", res)
	}
}

func TestInquiry(t *testing.T) {
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath: oauthHandler,
		"/api/online/payment/v1/status": testutil.JSON(
			`{"successful":true,"response":{"status":"SETTLE","transactionId":"tx-9","amount":1500000}}`),
	})
	gw, _ := snapppay.New(merchant, core.WithBaseURL(server.URL))

	res, err := gw.Inquiry(context.Background(), core.InquiryRequest{Token: "pt-1"})
	if err != nil {
		t.Fatalf("Inquiry() error = %v", err)
	}
	if res.Status != core.StatusVerified {
		t.Fatalf("status = %v", res.Status)
	}
}

func TestOAuthSendsTheClientCredentials(t *testing.T) {
	var authorization string
	var form map[string]string
	server := testutil.NewServer(t, testutil.Routes{
		"/api/online/v1/oauth/token": func(w http.ResponseWriter, r *http.Request) {
			authorization = r.Header.Get("Authorization")
			testutil.CaptureForm(t, &form, `{"access_token":"a","expires_in":3600}`)(w, r)
		},
		"/api/online/payment/v1/status": testutil.JSON(`{"successful":true,"response":{"status":"INIT"}}`),
	})
	gw, _ := snapppay.New(merchant, core.WithBaseURL(server.URL))

	if _, err := gw.Inquiry(context.Background(), core.InquiryRequest{Token: "pt-1"}); err != nil {
		t.Fatalf("Inquiry() error = %v", err)
	}

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("client-id:client-secret"))
	switch {
	case authorization != want:
		t.Errorf("Authorization = %q, want %q", authorization, want)
	case form["grant_type"] != "password":
		t.Errorf("grant_type = %q", form["grant_type"])
	case form["scope"] != "online-merchant":
		t.Errorf("scope = %q", form["scope"])
	case form["username"] != "merchant-user":
		t.Errorf("username = %q", form["username"])
	}
}

func TestBadCredentialsAreReportedAsConfigErrors(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/api/online/v1/oauth/token": testutil.Status(http.StatusUnauthorized,
			`{"error":"invalid_grant","error_description":"bad credentials"}`),
	})
	gw, _ := snapppay.New(merchant, core.WithBaseURL(server.URL))

	_, err := gw.Inquiry(context.Background(), core.InquiryRequest{Token: "pt-1"})
	if !errors.Is(err, core.ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := snapppay.New(merchant)
	request := httptest.NewRequest(http.MethodGet,
		"/cb?paymentToken=pt-1&transactionId=1001&status=OK", nil)

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.Token != "pt-1" || callback.OrderID != "1001" {
		t.Fatalf("callback = %+v", callback)
	}
}

func TestParseCallbackRejectsAFailedStatus(t *testing.T) {
	gw, _ := snapppay.New(merchant)
	request := httptest.NewRequest(http.MethodGet, "/cb?paymentToken=pt-1&status=FAILED", nil)

	callback, _ := gw.ParseCallback(request)
	if callback.Succeeded {
		t.Fatal("a FAILED callback must not be reported as succeeded")
	}
}

func TestNewValidatesTheCredentials(t *testing.T) {
	if _, err := snapppay.New(core.Config{Username: "u", Password: "p"}); !errors.Is(err, core.ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
	if _, err := snapppay.New(core.Config{MerchantID: "c", MerchantKey: "s"}); !errors.Is(err, core.ErrInvalidConfig) {
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
	gw, _ := snapppay.New(merchant, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "tok-1", Amount: core.Rial(150_000)})
	if !errors.Is(err, core.ErrAmountMismatch) {
		t.Fatalf("error = %v, want ErrAmountMismatch", err)
	}
}
