// Command multigateway shows the point of the package: a table of terminals
// read from configuration, every one of them driven by the same code.
//
//	go run ./examples/multigateway
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/amiranmanesh/payvand"
	"github.com/amiranmanesh/payvand/gateway/jibit"
	"github.com/amiranmanesh/payvand/gateway/parsian"
	"github.com/amiranmanesh/payvand/gateway/snapppay"
	"github.com/amiranmanesh/payvand/gateway/zarinpal"
)

// terminal is what an application typically stores per merchant: a gateway
// name, its credentials and the provider specific switches.
type terminal struct {
	Gateway payvand.Name
	Config  payvand.Config
	Options []payvand.Option
}

func main() {
	pv := payvand.Init(payvand.WithTimeout(20 * time.Second))

	terminals := []terminal{
		{Gateway: payvand.Virtual, Config: payvand.Config{}},
		{
			Gateway: payvand.Zarinpal,
			Config:  payvand.Config{MerchantKey: "00000000-0000-0000-0000-000000000000"},
			Options: []payvand.Option{zarinpal.WithDefaultDescription("Wallet top-up")},
		},
		{
			Gateway: payvand.Parsian,
			Config:  payvand.Config{MerchantKey: "login-account", IBAN: "IR000000000000000000000001"},
			Options: []payvand.Option{parsian.WithSettlementToIBAN(true)},
		},
		{
			Gateway: payvand.Mellat,
			Config:  payvand.Config{TerminalID: "1234567", Username: "user", Password: "pass"},
		},
		{
			// A buy-now-pay-later provider is wired exactly like the others;
			// only the basket builder is specific to it.
			Gateway: payvand.SnappPay,
			Config: payvand.Config{
				Username: "merchant", Password: "pass",
				MerchantID: "client-id", MerchantKey: "client-secret",
			},
			Options: []payvand.Option{snapppay.WithDefaultCategory("DIGITAL")},
		},
		{
			Gateway: payvand.Jibit,
			Config:  payvand.Config{MerchantKey: "api-key", Password: "secret-key"},
			Options: []payvand.Option{jibit.WithDefaultDescription("Wallet top-up")},
		},
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "GATEWAY\tREDIRECT\tVERIFY\tREFUND\tINQUIRY\tCALLBACK\tMULTIPLEXING")

	for _, item := range terminals {
		gw, err := pv.Gateway(item.Gateway, item.Config, item.Options...)
		if err != nil {
			fmt.Fprintf(writer, "%s\t(misconfigured: %v)\n", item.Gateway, err)
			continue
		}
		capabilities := gw.Capabilities()
		fmt.Fprintf(writer, "%s\t%s\t%v\t%v\t%v\t%v\t%v\n",
			gw.Name(), orNone(capabilities.RedirectMethod), capabilities.Verify, capabilities.Refund,
			capabilities.Inquiry, capabilities.Callback, capabilities.Multiplexing)
	}
	_ = writer.Flush()

	// Only the virtual terminal can be exercised without a real merchant
	// account, but the code below is the code every terminal would run.
	gw, err := pv.Gateway(payvand.Virtual, payvand.Config{})
	if err != nil {
		fmt.Println("building the virtual gateway:", err)
		return
	}
	if err := runCycle(context.Background(), gw); err != nil {
		fmt.Println("payment cycle:", err)
	}
}

// runCycle is written against the interface, so it accepts any provider.
func runCycle(ctx context.Context, gw payvand.Gateway) error {
	amount := payvand.Toman(25_000)

	purchase, err := gw.Purchase(ctx, payvand.PurchaseRequest{
		Amount:      amount,
		OrderID:     "2001",
		CallbackURL: "https://shop.example/payments/callback",
	})
	if err != nil {
		return fmt.Errorf("purchase: %w", err)
	}

	verified, err := gw.Verify(ctx, payvand.VerifyRequest{
		Token:   purchase.Token,
		OrderID: purchase.OrderID,
		Amount:  amount,
	})
	switch {
	case errors.Is(err, payvand.ErrNotSupported):
		fmt.Println("\nthis provider verifies payments elsewhere")
		return nil
	case err != nil:
		return fmt.Errorf("verify: %w", err)
	}

	fmt.Printf("\n%s settled order %s with reference %s\n",
		gw.Name(), verified.OrderID, verified.ReferenceNumber)
	return nil
}

// orNone renders an empty redirect method, which in-app gateways report.
func orNone(method string) string {
	if method == "" {
		return "in-app"
	}
	return method
}
