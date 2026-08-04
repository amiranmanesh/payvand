package payping_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/payping"
	"github.com/amiranmanesh/payvand/internal/testutil"
)

func TestPurchaseSendsToman(t *testing.T) {
	var authorization string
	var sent struct {
		Amount       int64  `json:"amount"`
		ClientRefID  string `json:"clientRefId"`
		ReturnURL    string `json:"returnUrl"`
		IsReversible bool   `json:"isReversible"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/v3/pay": func(w http.ResponseWriter, r *http.Request) {
			authorization = r.Header.Get("Authorization")
			testutil.Capture(t, &sent, `{"paymentCode":"pp-1","url":"https://payping.example/go/pp-1","gatewayAmount":15100}`)(w, r)
		},
	})

	gw, err := payping.New(core.Config{MerchantKey: "token-1"}, core.WithBaseURL(server.URL))
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
	case authorization != "Bearer token-1":
		t.Errorf("Authorization = %q", authorization)
	case sent.Amount != 15_000:
		t.Errorf("amount = %d, want the Rial amount converted to Toman", sent.Amount)
	case sent.ClientRefID != "1001":
		t.Errorf("clientRefId = %q", sent.ClientRefID)
	case sent.IsReversible:
		t.Errorf("isReversible = true without WithReversible")
	case res.Token != "pp-1":
		t.Errorf("token = %q, want the payment code", res.Token)
	case res.Redirect.URL != "https://payping.example/go/pp-1":
		t.Errorf("redirect = %q, want the url PayPing returned", res.Redirect.URL)
	case res.Get("gatewayAmount") != "15100":
		t.Errorf("gatewayAmount = %q", res.Get("gatewayAmount"))
	}
}

func TestPurchaseFallsBackToStartURL(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v3/pay": testutil.JSON(`{"paymentCode":"pp-2"}`),
	})
	gw, _ := payping.New(core.Config{MerchantKey: "t"}, core.WithBaseURL(server.URL))

	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Toman(15_000), OrderID: "1001", CallbackURL: "https://shop.example/callback",
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	if res.Redirect.URL != server.URL+"/v3/pay/start/pp-2" {
		t.Fatalf("redirect = %q", res.Redirect.URL)
	}
}

func TestPurchaseReversibleAndShared(t *testing.T) {
	var sent struct {
		Amount       int64 `json:"amount"`
		IsReversible bool  `json:"isReversible"`
		Items        []struct {
			UserIdentity string `json:"userIdentity"`
			Amount       int64  `json:"amount"`
		} `json:"items"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/v3/pay/shared": testutil.Capture(t, &sent, `{"paymentCode":"pp-3"}`),
	})
	gw, _ := payping.New(core.Config{MerchantKey: "t"},
		core.WithBaseURL(server.URL),
		payping.WithReversible(),
		payping.WithMultiplexing(
			payping.Share{UserIdentity: "09120000000", Amount: 10_000},
			payping.Share{UserIdentity: "seller@example.com", Amount: 5_000},
		))

	if _, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(150_000), OrderID: "1001", CallbackURL: "https://shop.example/callback",
	}); err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	switch {
	case sent.Amount != 0:
		t.Errorf("amount = %d, want no total on a shared payment", sent.Amount)
	case !sent.IsReversible:
		t.Errorf("isReversible = false despite WithReversible")
	case len(sent.Items) != 2 || sent.Items[0].Amount != 10_000 || sent.Items[1].UserIdentity != "seller@example.com":
		t.Errorf("items = %+v", sent.Items)
	}
}

func TestPurchaseRejectsSharesThatDoNotAddUp(t *testing.T) {
	gw, _ := payping.New(core.Config{MerchantKey: "t"},
		payping.WithMultiplexing(payping.Share{UserIdentity: "09120000000", Amount: 9_000}))

	_, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Toman(15_000), OrderID: "1001", CallbackURL: "https://shop.example/callback",
	})
	if !errors.Is(err, core.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestVerify(t *testing.T) {
	var sent struct {
		PaymentRefID int64  `json:"paymentRefId"`
		PaymentCode  string `json:"paymentCode"`
		Amount       int64  `json:"amount"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/v3/pay/verify": testutil.Capture(t, &sent,
			`{"amount":15000,"cardNumber":"603799******1234","cardHashPan":"HASH","clientRefId":"1001",
			  "paymentRefId":1111004003,"code":"pp-1","businessWage":100,"payedDate":"2024-08-03 10:30:00Z"}`),
	})
	gw, _ := payping.New(core.Config{MerchantKey: "t"}, core.WithBaseURL(server.URL))

	res, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token: "pp-1", ReferenceNumber: "1111004003", Amount: core.Rial(150_000),
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	switch {
	case sent.PaymentRefID != 1111004003 || sent.PaymentCode != "pp-1" || sent.Amount != 15_000:
		t.Errorf("request = %+v", sent)
	case res.Amount.Toman() != 15_000:
		t.Errorf("amount = %v", res.Amount)
	case res.ReferenceNumber != "1111004003" || res.TransactionID != "pp-1":
		t.Errorf("references = %q / %q", res.ReferenceNumber, res.TransactionID)
	case res.CardHash != "HASH" || res.OrderID != "1001":
		t.Errorf("response = %+v", res)
	case res.Fee != 1_000:
		t.Errorf("fee = %d, want the Toman business wage in Rial", res.Fee)
	case res.PaidAt.IsZero():
		t.Errorf("paidAt not parsed from %q", "2024-08-03 10:30:00Z")
	}
}

func TestVerifyRejectsAmountMismatch(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v3/pay/verify": testutil.JSON(`{"amount":9000,"code":"pp-1","paymentRefId":1}`),
	})
	gw, _ := payping.New(core.Config{MerchantKey: "t"}, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token: "pp-1", ReferenceNumber: "1", Amount: core.Toman(15_000),
	})
	if !errors.Is(err, core.ErrAmountMismatch) {
		t.Fatalf("error = %v, want ErrAmountMismatch", err)
	}
}

func TestVerifyRejectsAnotherOrder(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v3/pay/verify": testutil.JSON(`{"amount":15000,"code":"pp-1","paymentRefId":1,"clientRefId":"9999"}`),
	})
	gw, _ := payping.New(core.Config{MerchantKey: "t"}, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token: "pp-1", ReferenceNumber: "1", OrderID: "1001", Amount: core.Toman(15_000),
	})
	if !errors.Is(err, core.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestVerifyAlreadyVerified(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v3/pay/verify": testutil.Status(http.StatusConflict,
			`{"status":409,"metaData":{"code":110,"message":{"Amount":15000,"Code":"pp-1","PaymentRefId":1111004003}}}`),
	})
	gw, _ := payping.New(core.Config{MerchantKey: "t"}, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token: "pp-1", ReferenceNumber: "1111004003", Amount: core.Toman(15_000),
	})
	if !errors.Is(err, core.ErrAlreadyVerified) {
		t.Fatalf("error = %v, want ErrAlreadyVerified", err)
	}
}

func TestVerifyPending(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v3/pay/verify": testutil.Status(http.StatusAccepted, ""),
	})
	gw, _ := payping.New(core.Config{MerchantKey: "t"}, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token: "pp-1", ReferenceNumber: "1", Amount: core.Toman(15_000),
	})
	if !errors.Is(err, core.ErrVerificationPending) {
		t.Fatalf("error = %v, want ErrVerificationPending", err)
	}
}

func TestVerifyFailure(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v3/pay/verify": testutil.Status(http.StatusBadRequest,
			`{"title":"ValidationException","status":400,"metaData":{"code":101,"errors":[{"message":"amount mismatch"}]}}`),
	})
	gw, _ := payping.New(core.Config{MerchantKey: "t"}, core.WithBaseURL(server.URL))

	_, err := gw.Verify(context.Background(), core.VerifyRequest{
		Token: "pp-1", ReferenceNumber: "1", Amount: core.Toman(15_000),
	})
	if !errors.Is(err, core.ErrPaymentFailed) {
		t.Fatalf("error = %v, want ErrPaymentFailed", err)
	}
	var gwErr *core.Error
	if !errors.As(err, &gwErr) || gwErr.Code != "101" || gwErr.Message != "amount mismatch" {
		t.Fatalf("error detail = %+v", gwErr)
	}
}

func TestVerifyNeedsPaymentCode(t *testing.T) {
	gw, _ := payping.New(core.Config{MerchantKey: "t"})
	if _, err := gw.Verify(context.Background(), core.VerifyRequest{
		ReferenceNumber: "1", Amount: core.Toman(15_000),
	}); !errors.Is(err, core.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestRefund(t *testing.T) {
	var sent struct {
		PaymentRefID int64  `json:"paymentRefId"`
		PaymentCode  string `json:"paymentCode"`
	}
	server := testutil.NewServer(t, testutil.Routes{
		"/v3/pay/reverse": testutil.Capture(t, &sent, `{"amount":15000,"code":"pp-1","paymentRefId":1111004003}`),
	})
	gw, _ := payping.New(core.Config{MerchantKey: "t"}, core.WithBaseURL(server.URL))

	res, err := gw.Refund(context.Background(), core.RefundRequest{
		Token: "pp-1", ReferenceNumber: "1111004003",
	})
	if err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
	switch {
	case sent.PaymentRefID != 1111004003 || sent.PaymentCode != "pp-1":
		t.Errorf("request = %+v", sent)
	case res.Amount.Toman() != 15_000 || res.RefundID != "pp-1":
		t.Errorf("response = %+v", res)
	}
}

func TestInquiry(t *testing.T) {
	server := testutil.NewServer(t, testutil.Routes{
		"/v1/report/pp-1": testutil.JSON(
			`{"amount":15000,"isPaid":false,"paymentStatus":5,"refId":"1111004003","payDate":"2024-08-03T10:30:00Z"}`),
	})
	gw, _ := payping.New(core.Config{MerchantKey: "t"}, core.WithBaseURL(server.URL))

	res, err := gw.Inquiry(context.Background(), core.InquiryRequest{Token: "pp-1"})
	if err != nil {
		t.Fatalf("Inquiry() error = %v", err)
	}
	switch {
	case res.Status != core.StatusPaid:
		t.Errorf("status = %v, want paid and awaiting verification", res.Status)
	case res.ReferenceNumber != "1111004003":
		t.Errorf("reference = %q", res.ReferenceNumber)
	case res.Amount.Toman() != 15_000:
		t.Errorf("amount = %v", res.Amount)
	}
}

func TestParseCallback(t *testing.T) {
	gw, _ := payping.New(core.Config{MerchantKey: "t"})
	form := url.Values{
		"status": {"1"},
		"data": {`{"clientRefId":"1001","paymentCode":"pp-1","paymentRefId":1111004003,
		           "amount":15000,"gatewayAmount":15100,"cardNumber":"603799******1234","cardHashPan":"HASH"}`},
	}
	request := httptest.NewRequest(http.MethodPost, "/cb", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	switch {
	case !callback.Succeeded:
		t.Errorf("succeeded = false")
	case callback.Token != "pp-1":
		t.Errorf("token = %q, want the payment code", callback.Token)
	case callback.ReferenceNumber != "1111004003":
		t.Errorf("reference = %q", callback.ReferenceNumber)
	case callback.OrderID != "1001":
		t.Errorf("orderID = %q", callback.OrderID)
	case callback.CardNumber != "603799******1234":
		t.Errorf("cardNumber = %q", callback.CardNumber)
	case callback.Amount.Toman() != 15_000:
		t.Errorf("amount = %v", callback.Amount)
	case callback.Get("gatewayAmount") != "15100":
		t.Errorf("values = %+v", callback.Values)
	}

	// The verification request the callback builds must carry both keys v3
	// needs, with the amount coming from the merchant's own records.
	verify := callback.VerifyRequest(core.Toman(15_000))
	if verify.Token != "pp-1" || verify.ReferenceNumber != "1111004003" {
		t.Fatalf("verify request = %+v", verify)
	}
}

func TestParseCallbackUnpaid(t *testing.T) {
	gw, _ := payping.New(core.Config{MerchantKey: "t"})
	form := url.Values{"status": {"0"}, "errorCode": {"12"}, "data": {`{"clientRefId":"1001","paymentCode":"pp-1"}`}}
	request := httptest.NewRequest(http.MethodPost, "/cb", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if callback.Succeeded || callback.Code != "12" || callback.Token != "pp-1" {
		t.Fatalf("callback = %+v", callback)
	}
}
