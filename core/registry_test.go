package core_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/amiranmanesh/payvand/core"
)

// stubGateway is a minimal gateway used to exercise the registry.
type stubGateway struct {
	core.Unsupported
	cfg core.Config
}

func (s stubGateway) Name() core.Name                 { return "stub" }
func (s stubGateway) Capabilities() core.Capabilities { return core.Capabilities{Verify: true} }

func (s stubGateway) Purchase(context.Context, core.PurchaseRequest) (core.PurchaseResponse, error) {
	return core.PurchaseResponse{Token: "stub-token-" + s.cfg.MerchantKey}, nil
}

func (s stubGateway) Verify(context.Context, core.VerifyRequest) (core.VerifyResponse, error) {
	return core.VerifyResponse{ReferenceNumber: "stub-ref"}, nil
}

func init() {
	core.Register("stub", func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return stubGateway{Unsupported: core.Unsupported{GatewayName: "stub"}, cfg: cfg}, nil
	})
}

func TestNewBuildsRegisteredGateway(t *testing.T) {
	gw, err := core.New("stub", core.Config{MerchantKey: "k1"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	res, err := gw.Purchase(context.Background(), core.PurchaseRequest{Amount: core.Rial(1000)})
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	if res.Token != "stub-token-k1" {
		t.Errorf("token = %q, want stub-token-k1", res.Token)
	}
}

func TestNewRejectsUnknownGateway(t *testing.T) {
	_, err := core.New("does-not-exist", core.Config{})
	if !errors.Is(err, core.ErrGatewayNotRegistered) {
		t.Fatalf("error = %v, want ErrGatewayNotRegistered", err)
	}
}

func TestRegisteredContainsStub(t *testing.T) {
	if !core.IsRegistered("stub") {
		t.Fatal("the stub gateway must be registered")
	}
	found := false
	for _, name := range core.Registered() {
		if name == "stub" {
			found = true
		}
	}
	if !found {
		t.Fatal("Registered() does not list the stub gateway")
	}
}

func TestUnsupportedOperationsReportNotSupported(t *testing.T) {
	gw, err := core.New("stub", core.Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := gw.Refund(context.Background(), core.RefundRequest{}); !errors.Is(err, core.ErrNotSupported) {
		t.Errorf("Refund error = %v, want ErrNotSupported", err)
	}
	if _, err := gw.Inquiry(context.Background(), core.InquiryRequest{}); !errors.Is(err, core.ErrNotSupported) {
		t.Errorf("Inquiry error = %v, want ErrNotSupported", err)
	}
	if _, err := gw.ParseCallback(&http.Request{}); !errors.Is(err, core.ErrNotSupported) {
		t.Errorf("ParseCallback error = %v, want ErrNotSupported", err)
	}
}

func TestClientMergesOptions(t *testing.T) {
	client := core.Init(core.WithUserAgent("payvand-test")).With(core.WithSandbox(true))
	if _, err := client.Gateway("stub", core.Config{MerchantKey: "k"}); err != nil {
		t.Fatalf("Gateway() error = %v", err)
	}
	if gw := client.MustGateway("stub", core.Config{MerchantKey: "k"}); gw == nil {
		t.Fatal("MustGateway returned nil")
	}
}

func TestErrorCarriesCodeAndUnwraps(t *testing.T) {
	err := core.NewError("stub", "verify", core.ErrPaymentFailed).WithCode("51").WithMessage("insufficient funds")
	if !errors.Is(err, core.ErrPaymentFailed) {
		t.Fatal("the error must unwrap to ErrPaymentFailed")
	}

	var typed *core.Error
	if !errors.As(err, &typed) || typed.Code != "51" {
		t.Fatalf("errors.As did not surface the code, got %#v", typed)
	}
	if got, want := err.Error(), "payvand: stub: verify failed (code 51): insufficient funds"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
