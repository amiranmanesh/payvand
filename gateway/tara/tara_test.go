package tara_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/tara"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

// merchant is the credential set every test builds a gateway from.
var merchant = core.Config{Username: "shop", Password: "secret"}

// authenticate is the token route shared by the authenticated tests.
func authenticate() (string, http.HandlerFunc) {
	return "/api/v2/authenticate", testutil.JSON(`{"accessToken":"access-1","result":0}`)
}

func TestPurchase(t *testing.T) {
	var sent struct {
		Mobile      string `json:"mobile"`
		CallBackURL string `json:"callBackUrl"`
		Amount      int64  `json:"amount"`
		IP          string `json:"ip"`
		Services    []struct {
			ServiceID string `json:"serviceId"`
			Amount    int64  `json:"amount"`
		} `json:"serviceAmountList"`
		Items []struct {
			Name       string `json:"name"`
			Count      int    `json:"count"`
			Unit       int    `json:"unit"`
			Fee        int64  `json:"fee"`
			Group      string `json:"group"`
			GroupTitle string `json:"groupTitle"`
		} `json:"taraInvoiceItemList"`
	}
	var authorization string

	authPath, authHandler := authenticate()
	server := testutil.NewServer(t, testutil.Routes{
		authPath: authHandler,
		"/api/getToken": func(w http.ResponseWriter, r *http.Request) {
			authorization = r.Header.Get("Authorization")
			testutil.Capture(t, &sent, `{"token":"trace-77","result":0}`)(w, r)
		},
	})

	gw, err := tara.New(merchant,
		core.WithBaseURL(server.URL),
		tara.WithServiceID("service-1"),
		tara.WithDefaultGroup("40", "ارسال"),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount:      core.Toman(50_000),
		OrderID:     "4004",
		CallbackURL: "https://shop.example/callback",
		Mobile:      "09121234567",
		Description: "Grocery basket",
		Metadata:    map[string]string{tara.IPKey: "10.0.0.9"},
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}

	switch {
	case authorization != "Bearer access-1":
		t.Errorf("Authorization = %q", authorization)
	case sent.Amount != 500_000:
		t.Errorf("amount = %d, want Rial", sent.Amount)
	case sent.Mobile != "09121234567":
		t.Errorf("mobile = %q", sent.Mobile)
	case sent.IP != "10.0.0.9":
		t.Errorf("ip = %q", sent.IP)
	case len(sent.Services) != 1 || sent.Services[0].ServiceID != "service-1":
		t.Errorf("serviceAmountList = %+v", sent.Services)
	case len(sent.Items) != 1:
		t.Fatalf("taraInvoiceItemList = %+v", sent.Items)
	case sent.Items[0].Name != "Grocery basket":
		t.Errorf("item name = %q", sent.Items[0].Name)
	case sent.Items[0].Group != "40" || sent.Items[0].GroupTitle != "ارسال":
		t.Errorf("item group = %q/%q", sent.Items[0].Group, sent.Items[0].GroupTitle)
	case sent.Items[0].Unit != 5:
		t.Errorf("item unit = %d", sent.Items[0].Unit)
	case res.Token != "trace-77":
		t.Errorf("token = %q", res.Token)
	}
}

func TestPurchaseRedirectsWithAPostForm(t *testing.T) {
	authPath, authHandler := authenticate()
	server := testutil.NewServer(t, testutil.Routes{
		authPath:        authHandler,
		"/api/getToken": testutil.JSON(`{"token":"trace-77","result":0}`),
	})
	gw, _ := tara.New(merchant, core.WithBaseURL(server.URL))

	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(500_000), OrderID: "1", CallbackURL: "https://shop.example/cb", Mobile: "09120000000",
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	switch {
	case !res.Redirect.IsPost():
		t.Errorf("redirect method = %q, want POST", res.Redirect.Method)
	case !strings.HasSuffix(res.Redirect.URL, "/api/ipgPurchase"):
		t.Errorf("redirect url = %q", res.Redirect.URL)
	case res.Redirect.Params["token"] != "trace-77":
		t.Errorf("form token = %q", res.Redirect.Params["token"])
	case res.Redirect.Params["username"] != "shop":
		t.Errorf("form username = %q", res.Redirect.Params["username"])
	}
}

func TestPurchaseNeedsAMobileNumber(t *testing.T) {
	gw, _ := tara.New(merchant)
	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(500_000), OrderID: "1", CallbackURL: "https://shop.example/cb",
	})
	if !errors.Is(err, core.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestPurchaseFailureCarriesTheProviderDescription(t *testing.T) {
	authPath, authHandler := authenticate()
	server := testutil.NewServer(t, testutil.Routes{
		authPath:        authHandler,
		"/api/getToken": testutil.JSON(`{"result":102,"description":"اعتبار کافی نیست"}`),
	})
	gw, _ := tara.New(merchant, core.WithBaseURL(server.URL))

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(500_000), OrderID: "1", CallbackURL: "https://shop.example/cb", Mobile: "09120000000",
	})
	if !errors.Is(err, core.ErrPaymentFailed) {
		t.Fatalf("error = %v, want ErrPaymentFailed", err)
	}

	var gwErr *core.Error
	if !errors.As(err, &gwErr) || gwErr.Code != "102" || gwErr.Message != "اعتبار کافی نیست" {
		t.Fatalf("error = %+v, want the provider code and description", gwErr)
	}
}

func TestVerify(t *testing.T) {
	var sent struct {
		Token string `json:"token"`
		IP    string `json:"ip"`
	}
	authPath, authHandler := authenticate()
	server := testutil.NewServer(t, testutil.Routes{
		authPath: authHandler,
		"/api/purchaseVerify": testutil.Capture(t, &sent,
			`{"result":0,"rrn":"552211","amount":500000,"cardNumber":"627412******3344"}`),
	})
	gw, _ := tara.New(merchant, core.WithBaseURL(server.URL), tara.WithClientIP("10.0.0.1"))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "trace-77", OrderID: "4004"})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	switch {
	case sent.Token != "trace-77":
		t.Errorf("token = %q", sent.Token)
	case sent.IP != "10.0.0.1":
		t.Errorf("ip = %q", sent.IP)
	case res.ReferenceNumber != "552211":
		t.Errorf("reference = %q", res.ReferenceNumber)
	case res.Amount.Rial() != 500_000:
		t.Errorf("amount = %v", res.Amount)
	case res.CardNumber != "627412******3344":
		t.Errorf("card = %q", res.CardNumber)
	}
}

func TestVerifyRejectsANonZeroResult(t *testing.T) {
	authPath, authHandler := authenticate()
	server := testutil.NewServer(t, testutil.Routes{
		authPath:              authHandler,
		"/api/purchaseVerify": testutil.JSON(`{"result":21,"description":"تراکنش یافت نشد"}`),
	})
	gw, _ := tara.New(merchant, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "trace-77"})
	if !errors.Is(err, core.ErrPaymentFailed) {
		t.Fatalf("error = %v, want ErrPaymentFailed", err)
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := tara.New(merchant)
	form := strings.NewReader("token=trace-77&result=0&channelRefNumber=552211")
	request := httptest.NewRequest(http.MethodPost, "/cb", form)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	switch {
	case !callback.Succeeded:
		t.Error("a result of 0 must be reported as succeeded")
	case callback.Token != "trace-77":
		t.Errorf("token = %q", callback.Token)
	case callback.ReferenceNumber != "552211":
		t.Errorf("reference = %q", callback.ReferenceNumber)
	}
}

func TestParseCallbackReportsACancelledPayment(t *testing.T) {
	gw, _ := tara.New(merchant)
	request := httptest.NewRequest(http.MethodGet, "/cb?token=trace-77&result=-1", nil)

	callback, _ := gw.ParseCallback(request)
	if callback.Succeeded {
		t.Fatal("a non-zero result must not be reported as succeeded")
	}
}

func TestRefundAndInquiryAreNotSupported(t *testing.T) {
	gw, _ := tara.New(merchant)
	if _, err := gw.Refund(context.Background(), core.RefundRequest{Token: "t"}); !errors.Is(err, core.ErrNotSupported) {
		t.Errorf("Refund() error = %v, want ErrNotSupported", err)
	}
	if _, err := gw.Inquiry(context.Background(), core.InquiryRequest{Token: "t"}); !errors.Is(err, core.ErrNotSupported) {
		t.Errorf("Inquiry() error = %v, want ErrNotSupported", err)
	}
}

func TestBadCredentialsAreReportedAsConfigErrors(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/api/v2/authenticate": testutil.Status(http.StatusUnauthorized,
			`{"result":401,"description":"نام کاربری یا رمز عبور اشتباه است"}`),
	})
	gw, _ := tara.New(merchant, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "trace-77"})
	if !errors.Is(err, core.ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
}

func TestNewValidatesTheCredentials(t *testing.T) {
	if _, err := tara.New(core.Config{Username: "shop"}); !errors.Is(err, core.ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
}
