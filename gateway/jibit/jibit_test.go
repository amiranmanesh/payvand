package jibit_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/jibit"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

// tokens is the route every authenticated test needs.
func tokens() (string, http.HandlerFunc) {
	return "/v3/tokens", testutil.JSON(`{"accessToken":"access-1","refreshToken":"refresh-1"}`)
}

func TestPurchase(t *testing.T) {
	var sent struct {
		Amount                int64  `json:"amount"`
		Wage                  int64  `json:"wage"`
		Currency              string `json:"currency"`
		CallbackURL           string `json:"callbackUrl"`
		ClientReferenceNumber string `json:"clientReferenceNumber"`
		PayerMobileNumber     string `json:"payerMobileNumber"`
		Description           string `json:"description"`
	}
	var authorization string

	tokenPath, tokenHandler := tokens()
	server := testutil.NewServer(t, testutil.Routes{
		tokenPath: tokenHandler,
		"/v3/purchases": func(w http.ResponseWriter, r *http.Request) {
			authorization = r.Header.Get("Authorization")
			testutil.Capture(t, &sent,
				`{"purchaseId":700123,"purchaseIdStr":"700123","pspSwitchingUrl":"https://napi.jibit.ir/ppg/v3/purchases/700123/payments",
				  "clientReferenceNumber":"1001"}`)(w, r)
		},
	})

	gw, err := jibit.New(core.Config{MerchantKey: "api-key", Password: "secret-key"},
		core.WithBaseURL(server.URL),
		jibit.WithWage(5_000),
		jibit.WithDefaultDescription("Wallet top-up"),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount:      core.Toman(15_000),
		OrderID:     "1001",
		CallbackURL: "https://shop.example/callback",
		Mobile:      "09120000000",
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}

	switch {
	case authorization != "Bearer access-1":
		t.Errorf("Authorization = %q", authorization)
	case sent.Amount != 150_000:
		t.Errorf("amount = %d, want Rial", sent.Amount)
	case sent.Wage != 5_000:
		t.Errorf("wage = %d", sent.Wage)
	case sent.Currency != "IRR":
		t.Errorf("currency = %q", sent.Currency)
	case sent.ClientReferenceNumber != "1001":
		t.Errorf("clientReferenceNumber = %q", sent.ClientReferenceNumber)
	case sent.Description != "Wallet top-up":
		t.Errorf("description = %q", sent.Description)
	case res.Token != "700123":
		t.Errorf("token = %q", res.Token)
	case !strings.HasSuffix(res.Redirect.URL, "/payments"):
		t.Errorf("redirect = %q", res.Redirect.URL)
	}
}

func TestPurchaseRejectsSmallAmounts(t *testing.T) {
	gw, _ := jibit.New(core.Config{MerchantKey: "k", Password: "s"})
	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(1_000), OrderID: "1", CallbackURL: "https://shop.example/cb",
	})
	if !errors.Is(err, core.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestPurchaseFailureCarriesTheJibitCode(t *testing.T) {
	tokenPath, tokenHandler := tokens()
	server := testutil.NewServer(t, testutil.Routes{
		tokenPath: tokenHandler,
		"/v3/purchases": testutil.Status(http.StatusBadRequest,
			`{"errors":[{"code":"clientReferenceNumber.duplicated","message":"already used"}]}`),
	})
	gw, _ := jibit.New(core.Config{MerchantKey: "k", Password: "s"}, core.WithBaseURL(server.URL))

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(150_000), OrderID: "1001", CallbackURL: "https://shop.example/cb",
	})
	if !errors.Is(err, core.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}

	var gwErr *core.Error
	if !errors.As(err, &gwErr) || gwErr.Code != "clientReferenceNumber.duplicated" {
		t.Fatalf("error = %v, want the provider code", err)
	}
}

func TestVerify(t *testing.T) {
	tokenPath, tokenHandler := tokens()
	server := testutil.NewServer(t, testutil.Routes{
		tokenPath:                     tokenHandler,
		"/v3/purchases/700123/verify": testutil.JSON(`{"status":"SUCCESSFUL"}`),
	})
	gw, _ := jibit.New(core.Config{MerchantKey: "k", Password: "s"}, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token:           "700123",
		OrderID:         "1001",
		Amount:          core.Rial(150_000),
		ReferenceNumber: "884422",
		CardNumber:      "603799******1234",
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	switch {
	case res.ReferenceNumber != "884422":
		t.Errorf("reference = %q", res.ReferenceNumber)
	case res.TransactionID != "700123":
		t.Errorf("transaction = %q", res.TransactionID)
	case res.Amount.Rial() != 150_000:
		t.Errorf("amount = %v", res.Amount)
	}
}

func TestVerifyReportsAlreadyVerified(t *testing.T) {
	tokenPath, tokenHandler := tokens()
	server := testutil.NewServer(t, testutil.Routes{
		tokenPath:                     tokenHandler,
		"/v3/purchases/700123/verify": testutil.JSON(`{"status":"ALREADY_VERIFIED"}`),
	})
	gw, _ := jibit.New(core.Config{MerchantKey: "k", Password: "s"}, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "700123", ReferenceNumber: "1"})
	if !errors.Is(err, core.ErrAlreadyVerified) {
		t.Fatalf("error = %v, want ErrAlreadyVerified", err)
	}
}

func TestExpiredTokenIsRenewedOnce(t *testing.T) {
	issued := 0
	verifyCalls := 0
	server := testutil.NewServer(t, testutil.Routes{
		"/v3/tokens": func(w http.ResponseWriter, r *http.Request) {
			issued++
			testutil.JSON(`{"accessToken":"access","refreshToken":"refresh"}`)(w, r)
		},
		"/v3/purchases/1/verify": func(w http.ResponseWriter, r *http.Request) {
			verifyCalls++
			if verifyCalls == 1 {
				testutil.Status(http.StatusUnauthorized,
					`{"errors":[{"code":"security.auth_required","message":"token expired"}]}`)(w, r)
				return
			}
			testutil.JSON(`{"status":"SUCCESSFUL"}`)(w, r)
		},
	})
	gw, _ := jibit.New(core.Config{MerchantKey: "k", Password: "s"}, core.WithBaseURL(server.URL))

	if _, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "1", ReferenceNumber: "2"}); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if issued != 2 || verifyCalls != 2 {
		t.Fatalf("issued = %d, verifyCalls = %d, want 2 and 2", issued, verifyCalls)
	}
}

func TestRefund(t *testing.T) {
	var sent struct {
		PurchaseID  int64 `json:"purchaseId"`
		Amount      int64 `json:"amount"`
		Cancellable bool  `json:"cancellable"`
	}
	tokenPath, tokenHandler := tokens()
	server := testutil.NewServer(t, testutil.Routes{
		tokenPath:              tokenHandler,
		"/v3/purchases/refund": testutil.Capture(t, &sent, `{"refundId":"rf-99","transferId":"tr-1"}`),
	})
	gw, _ := jibit.New(core.Config{MerchantKey: "k", Password: "s"},
		core.WithBaseURL(server.URL), jibit.WithCancellableRefunds(true))

	res, err := gw.Refund(context.Background(), core.RefundRequest{
		Token: "700123", Amount: core.Toman(5_000),
	})
	if err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
	switch {
	case sent.PurchaseID != 700123:
		t.Errorf("purchaseId = %d", sent.PurchaseID)
	case sent.Amount != 50_000:
		t.Errorf("amount = %d, want Rial", sent.Amount)
	case !sent.Cancellable:
		t.Error("cancellable was not sent")
	case res.RefundID != "rf-99":
		t.Errorf("refundId = %q", res.RefundID)
	}
}

func TestReverse(t *testing.T) {
	tokenPath, tokenHandler := tokens()
	server := testutil.NewServer(t, testutil.Routes{
		tokenPath:               tokenHandler,
		"/v3/purchases/reverse": testutil.JSON(`{"status":"SUCCESSFUL"}`),
	})
	gw, _ := jibit.New(core.Config{MerchantKey: "k", Password: "s"}, core.WithBaseURL(server.URL))

	if _, err := gw.Reverse(context.Background(), core.RefundRequest{Token: "700123"}); err != nil {
		t.Fatalf("Reverse() error = %v", err)
	}
}

func TestInquiry(t *testing.T) {
	tokenPath, tokenHandler := tokens()
	server := testutil.NewServer(t, testutil.Routes{
		tokenPath: tokenHandler,
		"/v3/purchases": testutil.JSON(`{"elements":[{"purchaseId":700123,"state":"VERIFIED","amount":150000,
			"pspRrn":"884422","payerMaskedCardNumber":"603799******1234","updatedAt":"2026-07-31T10:20:30"}]}`),
	})
	gw, _ := jibit.New(core.Config{MerchantKey: "k", Password: "s"}, core.WithBaseURL(server.URL))

	res, err := gw.Inquiry(context.Background(), core.InquiryRequest{Token: "700123"})
	if err != nil {
		t.Fatalf("Inquiry() error = %v", err)
	}
	switch {
	case res.Status != core.StatusVerified:
		t.Errorf("status = %v", res.Status)
	case res.ReferenceNumber != "884422":
		t.Errorf("reference = %q", res.ReferenceNumber)
	case res.PaidAt.IsZero():
		t.Error("updatedAt was not parsed")
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := jibit.New(core.Config{MerchantKey: "k", Password: "s"})
	form := strings.NewReader("status=SUCCESSFUL&purchaseId=700123&clientReferenceNumber=1001&pspRRN=884422" +
		"&payerMaskedCardNumber=603799%2A%2A%2A%2A%2A%2A1234&amount=150000")
	request := httptest.NewRequest(http.MethodPost, "/cb", form)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	switch {
	case !callback.Succeeded:
		t.Error("callback should have succeeded")
	case callback.Token != "700123":
		t.Errorf("token = %q", callback.Token)
	case callback.OrderID != "1001":
		t.Errorf("order = %q", callback.OrderID)
	case callback.ReferenceNumber != "884422":
		t.Errorf("reference = %q", callback.ReferenceNumber)
	case callback.Amount.Rial() != 150_000:
		t.Errorf("amount = %v", callback.Amount)
	}
}

func TestParseCallbackReportsTheFailureReason(t *testing.T) {
	gw, _ := jibit.New(core.Config{MerchantKey: "k", Password: "s"})
	request := httptest.NewRequest(http.MethodGet,
		"/cb?status=FAILED&purchaseId=700123&failReason=CANCELLED_BY_USER", nil)

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if callback.Succeeded || callback.Message != "CANCELLED_BY_USER" {
		t.Fatalf("callback = %+v", callback)
	}
}

func TestNewRequiresBothKeys(t *testing.T) {
	if _, err := jibit.New(core.Config{MerchantKey: "k"}); !errors.Is(err, core.ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
	if _, err := jibit.New(core.Config{Password: "s"}); !errors.Is(err, core.ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
}
