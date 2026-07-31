// Command basic runs one full payment cycle — purchase, callback, verify,
// refund — against the in-memory virtual gateway, so it works offline.
//
//	go run ./examples/basic
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http/httptest"
	"time"

	"github.com/amiranmanesh/payvand"
)

func main() {
	ctx := context.Background()

	// 1. Initialise once, at start-up, with the settings shared by the whole
	//    application.
	pv := payvand.Init(
		payvand.WithTimeout(20*time.Second),
		payvand.WithUserAgent("payvand-example/1.0"),
	)

	// 2. Build the gateway of the terminal you are charging on. Replacing
	//    payvand.Virtual with payvand.Zarinpal, payvand.Mellat, … is the only
	//    change needed to go live.
	gw, err := pv.Gateway(payvand.Virtual, payvand.Config{MerchantKey: "merchant-key"})
	if err != nil {
		log.Fatalf("building the gateway: %v", err)
	}
	fmt.Printf("gateway %s, capabilities %+v\n\n", gw.Name(), gw.Capabilities())

	// 3. Create the payment and send the payer to the bank.
	amount := payvand.Toman(15_000)
	purchase, err := gw.Purchase(ctx, payvand.PurchaseRequest{
		Amount:      amount,
		OrderID:     "1001",
		CallbackURL: "https://shop.example/payments/callback",
		Description: "Wallet top-up",
		Mobile:      "09120000000",
	})
	if err != nil {
		log.Fatalf("creating the payment: %v", err)
	}
	fmt.Println("token:      ", purchase.Token)
	fmt.Println("redirect to:", purchase.Redirect.String())

	// 4. The payer comes back. In a real program this is an HTTP handler; here
	//    the request the bank would send is built by hand.
	request := httptest.NewRequest("GET", purchase.Redirect.String(), nil)
	callback, err := gw.ParseCallback(request)
	if err != nil {
		log.Fatalf("parsing the callback: %v", err)
	}
	if !callback.Succeeded {
		log.Fatal("the payer canceled the payment")
	}

	// 5. Verify. Until this succeeds the money is not yours: most Iranian
	//    gateways reverse an unverified transaction automatically.
	//    The amount comes from your own order, never from the callback.
	verified, err := gw.Verify(ctx, callback.VerifyRequest(amount))
	switch {
	case errors.Is(err, payvand.ErrAlreadyVerified):
		fmt.Println("the payment was already verified, nothing to do")
	case err != nil:
		log.Fatalf("verifying the payment: %v", err)
	default:
		fmt.Println("\nverified")
		fmt.Println("reference number:", verified.ReferenceNumber)
		fmt.Println("card:            ", verified.CardNumber)
		fmt.Println("amount:          ", verified.Amount.Rial(), "Rial")
	}

	// 6. Refund, when the provider supports it.
	if !gw.Capabilities().Refund {
		fmt.Println("\nthis provider has no refund API")
		return
	}
	if _, err := gw.Refund(ctx, payvand.RefundRequest{
		Token:           purchase.Token,
		OrderID:         purchase.OrderID,
		ReferenceNumber: verified.ReferenceNumber,
		Amount:          amount,
	}); err != nil {
		log.Fatalf("refunding the payment: %v", err)
	}
	fmt.Println("\nrefunded")

	// 7. Ask the provider what it thinks the transaction looks like now.
	if gw.Capabilities().Inquiry {
		inquiry, err := gw.Inquiry(ctx, payvand.InquiryRequest{Token: purchase.Token, OrderID: purchase.OrderID})
		if err != nil {
			log.Fatalf("inquiring the transaction: %v", err)
		}
		fmt.Println("status:", inquiry.Status)
	}
}
