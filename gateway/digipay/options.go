package digipay

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// Ticket types. The type decides which Digipay product pays the order and it
// travels as the "type" query parameter of every call.
const (
	// TypeIPG is a normal card payment through Digipay's own IPG.
	TypeIPG = 0
	// TypeCredit is the credit (instalment) product.
	TypeCredit = 5
	// TypeWallet is the Digipay wallet, the default of the API.
	TypeWallet = 11
	// TypeBNPL is the buy-now-pay-later product.
	TypeBNPL = 13
	// TypeCreditCard is the Digipay credit card product.
	TypeCreditCard = 24
)

// Preferred gateways, sent with a wallet ticket to pre-select how the payer
// tops the wallet up.
const (
	// GatewayWallet asks Digipay to pay from the wallet balance.
	GatewayWallet = 0
	// GatewayIPG asks Digipay to open the card gateway.
	GatewayIPG = 2
)

// BasketBuilder turns a purchase request into the basketDetailsDto object
// Digipay is shown. The returned value is marshalled as-is, so it can be any
// struct or map matching the shape in Digipay's documentation.
type BasketBuilder func(req core.PurchaseRequest) any

// config holds the Digipay specific options.
type config struct {
	// ticketType is the Digipay product used by Purchase.
	ticketType int
	// ticketTypeSet records whether ticketType was chosen explicitly, so that
	// the zero value TypeIPG stays selectable.
	ticketTypeSet bool
	// agent is the Agent header: WEB, MOBILE or the value in your contract.
	agent string
	// apiVersion is the Digipay-Version header.
	apiVersion string
	// preferredGateway pre-selects the funding source of a wallet ticket.
	preferredGateway *int
	// basketBuilder produces the optional basket of a purchase.
	basketBuilder BasketBuilder
	// splitDetails is the optional split settlement description.
	splitDetails any
}

// settings returns the Digipay option state of o, creating it on first use.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithTicketType selects the Digipay product: [TypeIPG], [TypeCredit],
// [TypeWallet], [TypeBNPL] or [TypeCreditCard]. It defaults to [TypeWallet].
func WithTicketType(ticketType int) core.Option {
	return func(o *core.Options) {
		c := settings(o)
		c.ticketType, c.ticketTypeSet = ticketType, true
	}
}

// WithAgent overrides the Agent header sent with a ticket request.
func WithAgent(agent string) core.Option {
	return func(o *core.Options) { settings(o).agent = agent }
}

// WithAPIVersion overrides the Digipay-Version header, which pins the contract
// version the merchant was onboarded on.
func WithAPIVersion(version string) core.Option {
	return func(o *core.Options) { settings(o).apiVersion = version }
}

// WithPreferredGateway pre-selects how a wallet ticket is funded:
// [GatewayWallet] or [GatewayIPG].
func WithPreferredGateway(gateway int) core.Option {
	return func(o *core.Options) { settings(o).preferredGateway = &gateway }
}

// WithBasketBuilder installs the function that produces the optional
// basketDetailsDto of a purchase. Digipay requires it for the credit and BNPL
// products, which lend against the goods.
func WithBasketBuilder(builder BasketBuilder) core.Option {
	return func(o *core.Options) { settings(o).basketBuilder = builder }
}

// WithBasket sends a fixed basketDetailsDto with every purchase.
func WithBasket(basket any) core.Option {
	return func(o *core.Options) {
		settings(o).basketBuilder = func(core.PurchaseRequest) any { return basket }
	}
}

// WithSplitDetails sets the splitDetailsList sent with every purchase, which
// is how a marketplace splits one payment between several providers. The value
// is marshalled as-is.
func WithSplitDetails(details any) core.Option {
	return func(o *core.Options) { settings(o).splitDetails = details }
}
