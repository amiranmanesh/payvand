package payvand_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amiranmanesh/payvand"
)

// checkout is the kind of function an application writes once: it knows the
// [payvand.Gateway] interface and nothing about the provider behind it.
func checkout(ctx context.Context, gw payvand.Gateway, amount payvand.Money) (payvand.PurchaseResponse, error) {
	return gw.Purchase(ctx, payvand.PurchaseRequest{
		Amount:      amount,
		OrderID:     "1001",
		CallbackURL: "https://shop.example/payments/callback",
		Description: "Wallet top-up",
	})
}

func TestEveryGatewayIsRegistered(t *testing.T) {
	want := []payvand.Name{
		payvand.AsanPardakht, payvand.BitPay, payvand.IDPay, payvand.IranKish, payvand.Mellat,
		payvand.NextPay, payvand.Parsian, payvand.Pasargad, payvand.PayIr, payvand.PayPing,
		payvand.PayWeb, payvand.Sadad, payvand.Saman, payvand.Sepehr, payvand.Top,
		payvand.Vandar, payvand.Virtual, payvand.YekPay, payvand.Zarinpal, payvand.Zibal,
	}
	for _, name := range want {
		if !payvand.IsRegistered(name) {
			t.Errorf("gateway %q is not registered", name)
		}
	}
	if got := len(payvand.Registered()); got < len(want) {
		t.Fatalf("Registered() returned %d gateways, want at least %d", got, len(want))
	}
}

func TestSwitchingGatewayDoesNotChangeTheCallSite(t *testing.T) {
	// Two very different providers: an in-memory one and a REST one served by
	// a fake host. The checkout function above is used unchanged for both.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"result":100,"trackId":4242}`))
	}))
	defer server.Close()

	pv := payvand.Init(payvand.WithUserAgent("payvand-test"))

	gateways := map[payvand.Name][]payvand.Option{
		payvand.Virtual: nil,
		payvand.Zibal:   {payvand.WithBaseURL(server.URL)},
	}

	for name, opts := range gateways {
		gw, err := pv.Gateway(name, payvand.Config{MerchantKey: "key"}, opts...)
		if err != nil {
			t.Fatalf("Gateway(%q) error = %v", name, err)
		}
		res, err := checkout(context.Background(), gw, payvand.Toman(15_000))
		if err != nil {
			t.Fatalf("checkout with %q error = %v", name, err)
		}
		if res.Token == "" {
			t.Errorf("gateway %q returned no token", name)
		}
		if gw.Name() != name {
			t.Errorf("Name() = %q, want %q", gw.Name(), name)
		}
	}
}

func TestUnknownGatewayIsReported(t *testing.T) {
	_, err := payvand.New("bank-of-nowhere", payvand.Config{})
	if !errors.Is(err, payvand.ErrGatewayNotRegistered) {
		t.Fatalf("error = %v, want ErrGatewayNotRegistered", err)
	}
}

func TestCapabilitiesDescribeTheProvider(t *testing.T) {
	cases := map[payvand.Name]struct {
		refund   bool
		callback bool
	}{
		payvand.Zarinpal: {refund: false, callback: true},
		payvand.Mellat:   {refund: true, callback: true},
		payvand.Top:      {refund: false, callback: false},
	}

	for name, want := range cases {
		gw, err := payvand.New(name, payvand.Config{
			MerchantKey: "key", TerminalID: "1", Username: "u", Password: "p", MerchantID: "m",
		})
		if err != nil {
			t.Fatalf("New(%q) error = %v", name, err)
		}
		got := gw.Capabilities()
		if got.Refund != want.refund || got.Callback != want.callback {
			t.Errorf("%q capabilities = %+v, want refund=%v callback=%v", name, got, want.refund, want.callback)
		}
	}
}

func TestMustNewPanicsOnBadConfig(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustNew must panic on an invalid configuration")
		}
	}()
	payvand.MustNew(payvand.Zarinpal, payvand.Config{})
}

func TestRegisterCustomGateway(t *testing.T) {
	payvand.Register("in-house", func(cfg payvand.Config, opts ...payvand.Option) (payvand.Gateway, error) {
		gw, err := payvand.New(payvand.Virtual, cfg, opts...)
		return gw, err
	})

	gw, err := payvand.New("in-house", payvand.Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := checkout(context.Background(), gw, payvand.Rial(10_000)); err != nil {
		t.Fatalf("checkout error = %v", err)
	}
}
