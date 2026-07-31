// Package tara implements the Tara club credit gateway (REST,
// pay.tara360.ir).
//
// Tara pays from a club credit rather than from a card, so a purchase carries
// the payer's mobile number and an itemised invoice: the credit is granted per
// product group, and Tara refuses lines whose group the contract may not sell.
// The payer is sent to the Tara page with an HTML form rather than a plain
// redirect, which [core.Redirect] handles for the caller.
//
// Credentials: [core.Config.Username] and [core.Config.Password] are the
// merchant credentials of the Tara panel; Payvand exchanges them for a bearer
// token on the first call and renews it automatically.
package tara

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
	"github.com/amiranmanesh/payvand/internal/tokenauth"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Name is the registry name of this gateway.
const Name core.Name = "tara"

// Provider endpoints.
const (
	defaultBaseURL = "https://pay.tara360.ir/pay"

	authenticatePath = "/api/v2/authenticate"
	tokenPath        = "/api/getToken"
	purchasePath     = "/api/ipgPurchase"
	verifyPath       = "/api/purchaseVerify"
)

// tokenLifetime is how long a Tara access token is assumed to last. The
// provider does not always report one, and an expired token is recovered from
// by re-authenticating on the next 401.
const tokenLifetime = 30 * time.Minute

// Defaults applied when the corresponding option is not set.
const (
	defaultUnit       = 5
	defaultGroup      = "26"
	defaultGroupTitle = "سایر"
)

// IPKey is the [core.PurchaseRequest.Metadata] key carrying the payer's IP
// address, which Tara uses for its fraud checks.
const IPKey = "ip"

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the Tara implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers Refund and Inquiry, which Tara does not expose to
	// merchants; reversals are handled from the Tara panel.
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	tokens   *tokenauth.Cache
	auth     *tokenauth.Client

	baseURL string
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a Tara gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	if strings.TrimSpace(cfg.Username) == "" || strings.TrimSpace(cfg.Password) == "" {
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("Username and Password of the Tara merchant are required")
	}

	baseURL := defaultBaseURL
	if options.BaseURL != "" {
		baseURL = options.BaseURL
	}

	gw := &Gateway{
		Unsupported: core.Unsupported{GatewayName: Name},
		cfg:         cfg,
		opts:        options,
		settings:    gwopt.From[config](options, string(Name)),
		client:      transport.New(options),
		tokens:      &tokenauth.Cache{Fallback: tokenLifetime},
		baseURL:     baseURL,
	}
	gw.auth = &tokenauth.Client{Cache: gw.tokens, Transport: gw.client, Fetch: gw.fetchToken}
	return gw, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what Tara offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Callback:       true,
		RedirectMethod: http.MethodPost,
		Currencies:     []core.Currency{core.IRR},
	}
}

// Purchase creates a payment and returns the form that posts the payer to the
// Tara page. The payer's mobile number is mandatory: it is the identity Tara
// grants the credit to.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	switch {
	case req.Amount.Rial() <= 0:
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be positive")
	case req.CallbackURL == "":
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("callback url is required")
	case req.Mobile == "":
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("mobile is required; Tara pays the club credit of that number")
	}

	body := tokenRequest{
		AdditionalData:      req.Description,
		Mobile:              req.Mobile,
		CallBackURL:         req.CallbackURL,
		Amount:              req.Amount.Rial(),
		TaraInvoiceItemList: g.invoice(req),
		IP:                  g.clientIP(req),
	}
	if g.settings.serviceID != "" {
		body.ServiceAmountList = []serviceAmount{{
			ServiceID: g.settings.serviceID,
			Amount:    req.Amount.Rial(),
		}}
	}

	var out tokenResponse
	res, err := g.auth.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, tokenPath), body, nil, &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if !res.OK() || out.Token == "" || !succeeded(out.Result) {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithCode(out.Result.String()).
			WithMessage(describe(out.Description, out.Message, "tara rejected the payment request"))
	}

	return core.PurchaseResponse{
		Token:   out.Token,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     res.Body,
		Redirect: core.Redirect{
			Method: http.MethodPost,
			URL:    transport.JoinURL(g.baseURL, purchasePath),
			Params: map[string]string{
				"username": g.cfg.Username,
				"token":    out.Token,
			},
		},
	}, nil
}

// Verify settles a payment the payer completed. Tara reverses a payment that
// is never verified, so call it as soon as the callback arrives.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	if req.Token == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("token is required")
	}

	body := verifyRequest{Token: req.Token, IP: g.settings.clientIP}
	if ip := req.Get(IPKey); ip != "" {
		body.IP = ip
	}

	var out verifyResponse
	res, err := g.auth.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, verifyPath), body, nil, &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	if !res.OK() || !succeeded(out.Result) || out.RRN == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithCode(out.Result.String()).
			WithMessage(describe(out.Description, out.Message, "tara did not verify the payment"))
	}

	var reported core.Money
	if parsed, convErr := strconv.ParseInt(out.Amount.String(), 10, 64); convErr == nil {
		reported = core.Rial(parsed)
	}
	amount, err := core.SettledAmount(Name, req.Amount, reported)
	if err != nil {
		return core.VerifyResponse{}, err
	}
	card := out.CardNumber
	if card == "" {
		card = req.CardNumber
	}

	return core.VerifyResponse{
		ReferenceNumber: out.RRN,
		TransactionID:   req.Token,
		OrderID:         req.OrderID,
		CardNumber:      card,
		Amount:          amount,
		Raw:             res.Body,
	}, nil
}

// ParseCallback reads the form Tara posts back to the merchant. A result of
// zero means the payer approved the payment; anything else means they did not.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}

	code := core.FirstValue(values, "result", "status")
	return core.Callback{
		Gateway:         Name,
		Succeeded:       code == "0",
		Token:           core.FirstValue(values, "token"),
		ReferenceNumber: core.FirstValue(values, "channelRefNumber", "rrn"),
		TraceNumber:     core.FirstValue(values, "traceNumber"),
		Code:            code,
		Message:         core.FirstValue(values, "description", "message"),
		Values:          values,
	}, nil
}

// fetchToken exchanges the merchant credentials for a bearer token.
func (g *Gateway) fetchToken(ctx context.Context) (string, time.Duration, error) {
	var out authenticateResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, authenticatePath),
		authenticateRequest{Username: g.cfg.Username, Password: g.cfg.Password}, nil, &out)
	if err != nil {
		return "", 0, core.NewError(Name, "token", err)
	}
	if !res.OK() || out.AccessToken == "" {
		return "", 0, core.NewError(Name, "token", core.ErrInvalidConfig).
			WithCode(out.Result.String()).
			WithMessage(describe(out.Description, out.Message, "tara refused the merchant credentials"))
	}

	lifetime := tokenLifetime
	if seconds, err := strconv.ParseInt(out.ExpiresIn.String(), 10, 64); err == nil && seconds > 0 {
		lifetime = time.Duration(seconds) * time.Second
	}
	return out.AccessToken, lifetime, nil
}

// invoice builds the invoice lines of a purchase, falling back to a single
// line covering the whole order.
func (g *Gateway) invoice(req core.PurchaseRequest) []InvoiceItem {
	if g.settings.invoiceBuilder != nil {
		return g.settings.invoiceBuilder(req)
	}

	name := req.Description
	if name == "" {
		name = "Order " + req.OrderID
	}
	code := req.OrderID
	if code == "" {
		code = "1"
	}
	group, groupTitle := g.settings.group, g.settings.groupTitle
	if group == "" {
		group, groupTitle = defaultGroup, defaultGroupTitle
	}
	unit := g.settings.unit
	if unit == 0 {
		unit = defaultUnit
	}

	return []InvoiceItem{{
		Name:       name,
		Code:       code,
		Count:      1,
		Unit:       unit,
		Fee:        req.Amount.Rial(),
		Group:      group,
		GroupTitle: groupTitle,
		Data:       req.Description,
	}}
}

// clientIP picks the payer IP address of a purchase.
func (g *Gateway) clientIP(req core.PurchaseRequest) string {
	if ip, ok := req.Metadata[IPKey]; ok && ip != "" {
		return ip
	}
	return g.settings.clientIP
}
