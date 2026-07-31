package payvand_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/amiranmanesh/payvand"
	"github.com/amiranmanesh/payvand/gateway/zibal"
)

// The virtual gateway keeps the examples runnable without a merchant account;
// swapping it for a real name is the only change a production program needs.
func Example() {
	pv := payvand.Init(payvand.WithTimeout(20 * time.Second))

	gw, err := pv.Gateway(payvand.Virtual, payvand.Config{MerchantKey: "merchant-key"})
	if err != nil {
		log.Fatal(err)
	}

	purchase, err := gw.Purchase(context.Background(), payvand.PurchaseRequest{
		Amount:      payvand.Toman(15_000),
		OrderID:     "1001",
		CallbackURL: "https://shop.example/payments/callback",
		Description: "Wallet top-up",
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("gateway:", gw.Name())
	fmt.Println("amount:", purchase.Amount.Rial(), "Rial")
	fmt.Println("redirect method:", purchase.Redirect.Method)
	// Output:
	// gateway: virtual
	// amount: 150000 Rial
	// redirect method: GET
}

// Provider specific options are ordinary [payvand.Option] values, so they
// compose with the shared ones in a single list.
func ExampleClient_Gateway() {
	pv := payvand.Init(payvand.WithTimeout(15 * time.Second))

	gw, err := pv.Gateway(payvand.Zibal, payvand.Config{MerchantKey: "zibal"},
		payvand.WithSandbox(true),
		zibal.WithFeeMode(1),
		zibal.WithMultiplexing(zibal.Share{
			BankAccount: "IR000000000000000000000001",
			Amount:      50_000,
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(gw.Name(), gw.Capabilities().Multiplexing)
	// Output: zibal true
}

// Operations a provider does not offer report [payvand.ErrNotSupported], so a
// caller can branch on capability instead of on provider name.
func ExampleGateway_refund() {
	gw, err := payvand.New(payvand.Zarinpal, payvand.Config{MerchantKey: "merchant-id"})
	if err != nil {
		log.Fatal(err)
	}

	if !gw.Capabilities().Refund {
		fmt.Println("refunds must be issued from the provider panel")
	}

	_, err = gw.Refund(context.Background(), payvand.RefundRequest{Token: "A1"})
	fmt.Println(errors.Is(err, payvand.ErrNotSupported))
	// Output:
	// refunds must be issued from the provider panel
	// true
}

// A callback handler is written once and works for every provider: the parsed
// callback carries the token, and the amount always comes from the merchant's
// own records.
func ExampleGateway_parseCallback() {
	gw, err := payvand.New(payvand.Virtual, payvand.Config{})
	if err != nil {
		log.Fatal(err)
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		callback, err := gw.ParseCallback(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !callback.Succeeded {
			http.Error(w, "the payer canceled the payment", http.StatusPaymentRequired)
			return
		}

		// The amount is read from the order, never from the query string.
		verified, err := gw.Verify(r.Context(), callback.VerifyRequest(payvand.Toman(15_000)))
		if err != nil {
			http.Error(w, err.Error(), http.StatusPaymentRequired)
			return
		}
		fmt.Fprintln(w, "reference number:", verified.ReferenceNumber)
	}

	_ = handler
	fmt.Println("ready")
	// Output: ready
}
