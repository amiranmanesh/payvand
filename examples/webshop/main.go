// Command webshop is a miniature shop showing the two HTTP handlers a payment
// integration needs: one that starts a payment and one that finishes it.
//
//	go run ./examples/webshop                 # in-memory virtual gateway
//	PAYVAND_GATEWAY=zarinpal PAYVAND_MERCHANT_KEY=… go run ./examples/webshop
//
// Then open http://localhost:8080/pay.
package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/amiranmanesh/payvand"
)

// order is the merchant side record of a payment. A real shop keeps this in
// its database; the amount stored here is the only amount ever trusted.
type order struct {
	ID     string
	Amount payvand.Money
	Token  string
	Status string
	// Reference is the bank reference number, kept so a refreshed callback can
	// be answered without verifying the payment again.
	Reference string
}

// shop holds the gateway and the orders.
type shop struct {
	gateway payvand.Gateway
	baseURL string

	mu     sync.Mutex
	orders map[string]*order // keyed by payment token
	nextID int
}

func main() {
	baseURL := env("PAYVAND_BASE_CALLBACK", "http://localhost:8080")

	gateway, err := payvand.Init(
		payvand.WithTimeout(20*time.Second),
		payvand.WithLogger(payvand.NopLogger{}),
	).Gateway(
		payvand.Name(env("PAYVAND_GATEWAY", string(payvand.Virtual))),
		payvand.Config{
			MerchantKey: os.Getenv("PAYVAND_MERCHANT_KEY"),
			MerchantID:  os.Getenv("PAYVAND_MERCHANT_ID"),
			TerminalID:  os.Getenv("PAYVAND_TERMINAL_ID"),
			Username:    os.Getenv("PAYVAND_USERNAME"),
			Password:    os.Getenv("PAYVAND_PASSWORD"),
		},
		payvand.WithSandbox(os.Getenv("PAYVAND_SANDBOX") == "1"),
	)
	if err != nil {
		log.Fatalf("building the gateway: %v", err)
	}

	s := &shop{gateway: gateway, baseURL: baseURL, orders: map[string]*order{}, nextID: 1000}

	http.HandleFunc("/pay", s.startPayment)
	http.HandleFunc("/payments/callback", s.finishPayment)

	log.Printf("payvand webshop on %s using the %s gateway", baseURL, gateway.Name())
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// startPayment creates the payment and hands the payer over to the bank.
func (s *shop) startPayment(w http.ResponseWriter, r *http.Request) {
	amount := payvand.Toman(15_000)

	s.mu.Lock()
	s.nextID++
	record := &order{ID: strconv.Itoa(s.nextID), Amount: amount, Status: "pending"}
	s.mu.Unlock()

	purchase, err := s.gateway.Purchase(r.Context(), payvand.PurchaseRequest{
		Amount:      amount,
		OrderID:     record.ID,
		CallbackURL: s.baseURL + "/payments/callback",
		Description: "Order " + record.ID,
	})
	if err != nil {
		http.Error(w, "the payment could not be created: "+err.Error(), http.StatusBadGateway)
		return
	}

	// Persist the token before the redirect: it is the only handle on the
	// payment once the payer is on the bank page.
	record.Token = purchase.Token
	s.mu.Lock()
	s.orders[purchase.Token] = record
	s.mu.Unlock()

	// Send() emits a plain redirect for GET gateways and an auto-submitting
	// form for the ones that need a POST.
	if err := purchase.Redirect.Send(w, r); err != nil {
		log.Printf("sending the payer to the gateway: %v", err)
	}
}

// finishPayment settles the payment the payer just came back from.
func (s *shop) finishPayment(w http.ResponseWriter, r *http.Request) {
	callback, err := s.gateway.ParseCallback(r)
	if err != nil {
		http.Error(w, "the callback could not be read: "+err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	record := s.orders[callback.Token]
	s.mu.Unlock()
	if record == nil {
		http.Error(w, "unknown payment token", http.StatusNotFound)
		return
	}

	if !callback.Succeeded {
		s.setStatus(record, "canceled")
		http.Error(w, "the payer canceled the payment", http.StatusPaymentRequired)
		return
	}

	// The amount comes from the order, never from the callback: the browser
	// carried that request and could have rewritten every field in it.
	verified, err := s.gateway.Verify(r.Context(), callback.VerifyRequest(record.Amount))
	switch {
	case errors.Is(err, payvand.ErrAlreadyVerified):
		// A refreshed callback page is normal; the order is already paid, and
		// the goods must not be shipped a second time. The provider tells us
		// nothing else about the payment here, so the receipt is written from
		// what was stored the first time round.
		s.mu.Lock()
		reference := record.Reference
		s.mu.Unlock()
		writeReceipt(w, record.ID, reference, "")
		return
	case errors.Is(err, payvand.ErrAmountMismatch):
		// The payment settled for something other than what was ordered.
		s.setStatus(record, "mismatch")
		http.Error(w, "the settled amount does not match the order", http.StatusPaymentRequired)
		return
	case err != nil:
		s.setStatus(record, "failed")
		http.Error(w, "the payment could not be verified: "+err.Error(), http.StatusPaymentRequired)
		return
	}

	s.mu.Lock()
	record.Status, record.Reference = "paid", verified.ReferenceNumber
	s.mu.Unlock()

	writeReceipt(w, record.ID, verified.ReferenceNumber, verified.CardNumber)
}

// setStatus records the outcome of a payment under the shop's lock, which the
// callback handler shares with the checkout handler.
func (s *shop) setStatus(record *order, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record.Status = status
}

// writeReceipt answers the payer with what the shop knows about the payment.
func writeReceipt(w http.ResponseWriter, orderID, reference, card string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "order %s paid\nreference number: %s\ncard: %s\n", orderID, reference, card)
}

// env reads an environment variable with a fallback.
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
