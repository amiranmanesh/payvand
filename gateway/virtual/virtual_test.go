package virtual_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/gateway/virtual"
)

func TestFullCycle(t *testing.T) {
	gw, err := virtual.New(core.Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx := context.Background()

	purchase, err := gw.Purchase(ctx, core.PurchaseRequest{
		Amount:      core.Toman(15_000),
		OrderID:     "1001",
		CallbackURL: "https://shop.example/callback",
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	if purchase.Redirect.URL != "https://shop.example/callback" {
		t.Fatalf("redirect = %q", purchase.Redirect.URL)
	}

	// The payer comes back through the redirect the gateway produced.
	request := httptest.NewRequest(http.MethodGet, purchase.Redirect.String(), nil)
	callback, err := gw.ParseCallback(request)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}
	if !callback.Succeeded || callback.Token != purchase.Token || callback.OrderID != "1001" {
		t.Fatalf("callback = %+v", callback)
	}

	verified, err := gw.Verify(ctx, callback.VerifyRequest(core.Toman(15_000)))
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if verified.ReferenceNumber == "" || verified.CardNumber != virtual.CardNumber {
		t.Fatalf("verified = %+v", verified)
	}

	if _, err := gw.Verify(ctx, callback.VerifyRequest(core.Toman(15_000))); !errors.Is(err, core.ErrAlreadyVerified) {
		t.Fatalf("the second verification error = %v, want ErrAlreadyVerified", err)
	}

	inquiry, err := gw.Inquiry(ctx, core.InquiryRequest{Token: purchase.Token})
	if err != nil {
		t.Fatalf("Inquiry() error = %v", err)
	}
	if inquiry.Status != core.StatusVerified {
		t.Fatalf("status = %v", inquiry.Status)
	}

	if _, err := gw.Refund(ctx, core.RefundRequest{Token: purchase.Token}); err != nil {
		t.Fatalf("Refund() error = %v", err)
	}
	if inquiry, _ = gw.Inquiry(ctx, core.InquiryRequest{Token: purchase.Token}); inquiry.Status != core.StatusRefunded {
		t.Fatalf("status after the refund = %v", inquiry.Status)
	}
}

func TestAmountMismatchIsDetected(t *testing.T) {
	gw, _ := virtual.New(core.Config{})
	purchase, _ := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(150_000), OrderID: "1", CallbackURL: "https://shop.example/cb",
	})

	_, err := gw.Verify(context.Background(), core.VerifyRequest{Token: purchase.Token, Amount: core.Rial(1_000)})
	if !errors.Is(err, core.ErrAmountMismatch) {
		t.Fatalf("error = %v, want ErrAmountMismatch", err)
	}
}

func TestDeclineMode(t *testing.T) {
	gw, _ := virtual.New(core.Config{}, virtual.WithDecline(true))
	purchase, err := gw.Purchase(context.Background(), core.PurchaseRequest{
		Amount: core.Rial(150_000), OrderID: "1", CallbackURL: "https://shop.example/cb",
	})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	if purchase.Redirect.Params[virtual.StatusParam] != "NOK" {
		t.Fatalf("status param = %q, want NOK", purchase.Redirect.Params[virtual.StatusParam])
	}
	if _, err := gw.Verify(context.Background(), core.VerifyRequest{Token: purchase.Token}); !errors.Is(err, core.ErrPaymentFailed) {
		t.Fatalf("error = %v, want ErrPaymentFailed", err)
	}
}

func TestUnknownTokenIsRejected(t *testing.T) {
	gw, _ := virtual.New(core.Config{})
	if _, err := gw.Verify(context.Background(), core.VerifyRequest{Token: "nope"}); !errors.Is(err, core.ErrPaymentFailed) {
		t.Fatalf("error = %v, want ErrPaymentFailed", err)
	}
}
