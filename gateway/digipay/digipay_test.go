package digipay_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/digipay"
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
	return "/digipay/api/oauth/token",
		testutil.JSON(`{"access_token":"access-1","token_type":"bearer","expires_in":3600}`)
}

func TestPurchase(t *testing.T) {
	var sent struct {
		Amount      int64  `json:"amount"`
		CellNumber  string `json:"cellNumber"`
		ProviderID  string `json:"providerId"`
		CallbackURL string `json:"callbackUrl"`
		Basket      struct {
			Total int64 `json:"total"`
		} `json:"basketDetailsDto"`
	}
	var query, agent, version string

	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath: oauthHandler,
		"/digipay/api/tickets/business": func(w http.ResponseWriter, r *http.Request) {
			query, agent, version = r.URL.RawQuery, r.Header.Get("Agent"), r.Header.Get("Digipay-Version")
			testutil.Capture(t, &sent, `{"ticket":"tk-1","redirectUrl":"https://api.mydigipay.com/pay/tk-1",
				"result":{"status":0,"message":"با موفقیت انجام شد"}}`)(w, r)
		},
	})

	gw, err := digipay.New(merchant,
		core.WithBaseURL(server.URL),
		digipay.WithTicketType(digipay.TypeBNPL),
		digipay.WithAgent("MOBILE"),
		digipay.WithBasket(map[string]any{"total": 3_000_000}),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount:      core.Toman(300_000),
		OrderID:     "3003",
		CallbackURL: "https://shop.example/callback",
		Mobile:      "09121234567",
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}

	switch {
	case query != "type=13":
		t.Errorf("query = %q, want the BNPL ticket type", query)
	case agent != "MOBILE":
		t.Errorf("Agent = %q", agent)
	case version == "":
		t.Error("Digipay-Version header is missing")
	case sent.Amount != 3_000_000:
		t.Errorf("amount = %d, want Rial", sent.Amount)
	case sent.ProviderID != "3003":
		t.Errorf("providerId = %q", sent.ProviderID)
	case sent.Basket.Total != 3_000_000:
		t.Errorf("basketDetailsDto = %+v", sent.Basket)
	case res.Token != "tk-1":
		t.Errorf("token = %q", res.Token)
	case res.Get(digipay.TicketTypeKey) != "13":
		t.Errorf("extra ticket type = %q", res.Get(digipay.TicketTypeKey))
	}
}

func TestPurchaseFailureCarriesTheProviderMessage(t *testing.T) {
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath: oauthHandler,
		"/digipay/api/tickets/business": testutil.Status(http.StatusUnprocessableEntity,
			`{"result":{"status":9000,"message":"مبلغ نامعتبر است"}}`),
	})
	gw, _ := digipay.New(merchant, core.WithBaseURL(server.URL))

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(1_000), OrderID: "1", CallbackURL: "https://shop.example/cb",
	})
	if !errors.Is(err, core.ErrPaymentFailed) {
		t.Fatalf("error = %v, want ErrPaymentFailed", err)
	}

	var gwErr *core.Error
	if !errors.As(err, &gwErr) || gwErr.Code != "9000" {
		t.Fatalf("error = %+v, want the provider status", gwErr)
	}
}

func TestVerifyUsesTheTicketTypeFromTheCallback(t *testing.T) {
	var query string
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath: oauthHandler,
		"/digipay/api/purchases/verify/trk-9": func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.RawQuery
			testutil.JSON(`{"trackingCode":"trk-9","providerId":"3003","amount":"3000000","rrn":"889900",
				"result":{"status":0}}`)(w, r)
		},
	})
	gw, _ := digipay.New(merchant, core.WithBaseURL(server.URL), digipay.WithTicketType(digipay.TypeWallet))

	callback, _ := gw.ParseCallback(httptest.NewRequest(http.MethodGet,
		"/cb?result=SUCCESS&trackingCode=trk-9&providerId=3003&type=13&amount=3000000", nil))

	res, err := gw.Verify(context.Background(), callback.VerifyRequest(core.Rial(3_000_000)))
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	switch {
	case query != "type=13":
		t.Errorf("query = %q, want the type reported by the callback", query)
	case res.ReferenceNumber != "889900":
		t.Errorf("reference = %q", res.ReferenceNumber)
	case res.Amount.Rial() != 3_000_000:
		t.Errorf("amount = %v", res.Amount)
	}
}

func TestRefund(t *testing.T) {
	var sent struct {
		ProviderID       string `json:"providerId"`
		Amount           int64  `json:"amount"`
		SaleTrackingCode string `json:"saleTrackingCode"`
	}
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath:              oauthHandler,
		"/digipay/api/refunds": testutil.Capture(t, &sent, `{"trackingCode":"rf-1","result":{"status":0}}`),
	})
	gw, _ := digipay.New(merchant, core.WithBaseURL(server.URL))

	res, err := gw.Refund(context.Background(), core.RefundRequest{
		OrderID:       "refund-3003",
		TransactionID: "trk-9",
		Amount:        core.Toman(100_000),
	})
	if err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
	switch {
	case sent.ProviderID != "refund-3003":
		t.Errorf("providerId = %q", sent.ProviderID)
	case sent.Amount != 1_000_000:
		t.Errorf("amount = %d, want Rial", sent.Amount)
	case sent.SaleTrackingCode != "trk-9":
		t.Errorf("saleTrackingCode = %q", sent.SaleTrackingCode)
	case res.RefundID != "rf-1":
		t.Errorf("refundId = %q", res.RefundID)
	}
}

func TestRefundNeedsAnAmount(t *testing.T) {
	gw, _ := digipay.New(merchant)
	_, err := gw.Refund(context.Background(), core.RefundRequest{OrderID: "r-1", TransactionID: "trk-9"})
	if !errors.Is(err, core.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestReverse(t *testing.T) {
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath:              oauthHandler,
		"/digipay/api/reverse": testutil.JSON(`{"trackingCode":"trk-9","result":{"status":0}}`),
	})
	gw, _ := digipay.New(merchant, core.WithBaseURL(server.URL))

	if err := gw.Reverse(context.Background(), "trk-9", "3003", digipay.TypeIPG); err != nil {
		t.Fatalf("Reverse() error = %v", err)
	}
}

func TestDeliver(t *testing.T) {
	var sent struct {
		DeliveryDate  string   `json:"deliveryDate"`
		InvoiceNumber string   `json:"invoiceNumber"`
		TrackingCode  string   `json:"trackingCode"`
		Products      []string `json:"products"`
	}
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath:                        oauthHandler,
		"/digipay/api/purchases/deliver": testutil.Capture(t, &sent, `{"result":{"status":0}}`),
	})
	gw, _ := digipay.New(merchant, core.WithBaseURL(server.URL))

	err := gw.Deliver(context.Background(), digipay.DeliverRequest{
		TrackingCode:  "trk-9",
		InvoiceNumber: "INV-1",
		DeliveryDate:  time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
		Products:      []string{"sku-1"},
		TicketType:    digipay.TypeBNPL,
	})
	if err != nil {
		t.Fatalf("Deliver() error = %v", err)
	}
	if sent.DeliveryDate != "2026-07-31" || sent.InvoiceNumber != "INV-1" {
		t.Fatalf("sent = %+v", sent)
	}
}

func TestDeliverRejectsAWalletTicket(t *testing.T) {
	gw, _ := digipay.New(merchant)
	err := gw.Deliver(context.Background(), digipay.DeliverRequest{
		TrackingCode: "trk-9", InvoiceNumber: "INV-1", Products: []string{"sku-1"}, TicketType: digipay.TypeWallet,
	})
	if !errors.Is(err, core.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestRefundStatus(t *testing.T) {
	oauthPath, oauthHandler := oauth()
	server := testutil.NewServer(t, testutil.Routes{
		oauthPath:                   oauthHandler,
		"/digipay/api/refunds/rf-1": testutil.JSON(`{"trackingCode":"rf-1","status":0,"result":{"status":0}}`),
	})
	gw, _ := digipay.New(merchant, core.WithBaseURL(server.URL))

	res, err := gw.RefundStatus(context.Background(), "rf-1", digipay.TypeIPG)
	if err != nil {
		t.Fatalf("RefundStatus() error = %v", err)
	}
	if res.Status != core.StatusRefunded {
		t.Fatalf("status = %v", res.Status)
	}
}

func TestInquiryIsNotSupported(t *testing.T) {
	gw, _ := digipay.New(merchant)
	if _, err := gw.Inquiry(context.Background(), core.InquiryRequest{Token: "trk-9"}); !errors.Is(err, core.ErrNotSupported) {
		t.Fatalf("error = %v, want ErrNotSupported", err)
	}
}

func TestOAuthPostsMultipartCredentials(t *testing.T) {
	var contentType, authorization, body string
	server := testutil.NewServer(t, testutil.Routes{
		"/digipay/api/oauth/token": func(w http.ResponseWriter, r *http.Request) {
			contentType = r.Header.Get("Content-Type")
			authorization = r.Header.Get("Authorization")
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("ParseMultipartForm() error = %v", err)
			}
			body = r.FormValue("username") + "/" + r.FormValue("grant_type")
			testutil.JSON(`{"access_token":"a","expires_in":3600}`)(w, r)
		},
		"/digipay/api/reverse": testutil.JSON(`{"result":{"status":0}}`),
	})
	gw, _ := digipay.New(merchant, core.WithBaseURL(server.URL))

	if err := gw.Reverse(context.Background(), "trk-9", "3003", digipay.TypeIPG); err != nil {
		t.Fatalf("Reverse() error = %v", err)
	}
	switch {
	case !strings.HasPrefix(contentType, "multipart/form-data"):
		t.Errorf("Content-Type = %q", contentType)
	case !strings.HasPrefix(authorization, "Basic "):
		t.Errorf("Authorization = %q", authorization)
	case body != "merchant-user/password":
		t.Errorf("form = %q", body)
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := digipay.New(merchant)
	form := strings.NewReader("result=SUCCESS&trackingCode=trk-9&providerId=3003&type=13&amount=3000000&rrn=889900")
	request := httptest.NewRequest(http.MethodPost, "/cb", form)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	switch {
	case !callback.Succeeded:
		t.Error("callback should have succeeded")
	case callback.Token != "trk-9":
		t.Errorf("token = %q, want the tracking code", callback.Token)
	case callback.OrderID != "3003":
		t.Errorf("order = %q", callback.OrderID)
	case callback.ReferenceNumber != "889900":
		t.Errorf("reference = %q", callback.ReferenceNumber)
	}
}

func TestSandboxUsesTheUATHost(t *testing.T) {
	gw, err := digipay.New(merchant, core.WithSandbox(true))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if gw.Name() != digipay.Name {
		t.Fatalf("name = %q", gw.Name())
	}
}

func TestNewValidatesTheCredentials(t *testing.T) {
	if _, err := digipay.New(core.Config{Username: "u", Password: "p"}); !errors.Is(err, core.ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
}
