// Package payping implements the PayPing gateway (REST, api.payping.ir).
//
// Credentials: [core.Config.MerchantKey] is the bearer token of the terminal.
// PayPing works in Toman, and Payvand converts the amount for you.
package payping

import (
	"context"
	"net/http"
	"strings"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Name is the registry name of this gateway.
const Name core.Name = "payping"

// Provider endpoints.
const (
	defaultBase  = "https://api.payping.ir"
	payPath      = "/v2/pay"
	verifyPath   = "/v2/pay/verify"
	redirectPath = "/v2/pay/gotoipg/"
)

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the PayPing implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers Refund and Inquiry, which PayPing keeps in its panel.
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	baseURL  string
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a PayPing gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	if strings.TrimSpace(cfg.MerchantKey) == "" {
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("MerchantKey (bearer token) is required")
	}
	baseURL := defaultBase
	if options.BaseURL != "" {
		baseURL = options.BaseURL
	}
	return &Gateway{
		Unsupported: core.Unsupported{GatewayName: Name},
		cfg:         cfg,
		opts:        options,
		settings:    gwopt.From[config](options, string(Name)),
		client:      transport.New(options),
		baseURL:     baseURL,
	}, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what PayPing offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Callback:       true,
		RedirectMethod: http.MethodGet,
		Currencies:     []core.Currency{core.IRT},
	}
}

// headers builds the authenticated headers of every call.
func (g *Gateway) headers() map[string]string {
	return map[string]string{"Authorization": "Bearer " + g.cfg.MerchantKey}
}

// Purchase creates a payment and returns the PayPing redirect URL.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	if req.Amount.Toman() <= 0 {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be at least 1 Toman")
	}
	if req.CallbackURL == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("callback url is required")
	}

	description := req.Description
	if description == "" {
		description = g.settings.description
	}
	identity := g.settings.payerIdentity
	if identity == "" {
		identity = firstNonEmpty(req.Mobile, req.Email)
	}

	var out payResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, payPath), payRequest{
		Amount:        req.Amount.Toman(),
		ReturnURL:     req.CallbackURL,
		PayerName:     req.PayerName,
		PayerIdentity: identity,
		Description:   description,
		ClientRefID:   req.OrderID,
	}, g.headers(), &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if !res.OK() || out.Code == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithMessage(res.Body)
	}

	return core.PurchaseResponse{
		Token:   out.Code,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     res.Body,
		Redirect: core.Redirect{
			Method: http.MethodGet,
			URL:    transport.JoinURL(g.baseURL, redirectPath+out.Code),
		},
	}, nil
}

// Verify settles the payment identified by the callback refId.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	refID := firstNonEmpty(req.ReferenceNumber, req.Get("refid"), req.Get("refId"), req.Token)
	if refID == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("refId is required")
	}

	var out verifyResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, verifyPath), verifyRequest{
		RefID:  refID,
		Amount: req.Amount.Toman(),
	}, g.headers(), &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	if !res.OK() {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithMessage(res.Body)
	}

	amount := req.Amount
	if out.Amount > 0 {
		amount = core.Toman(out.Amount)
	}
	orderID := out.ClientRefID
	if orderID == "" {
		orderID = req.OrderID
	}

	return core.VerifyResponse{
		ReferenceNumber: refID,
		TransactionID:   refID,
		OrderID:         orderID,
		CardNumber:      out.CardNumber,
		CardHash:        out.CardHashPan,
		Amount:          amount,
		Raw:             res.Body,
	}, nil
}

// ParseCallback reads the refId PayPing returns the payer with.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	refID := core.FirstValue(values, "refid", "refId", "RefID")
	return core.Callback{
		Gateway:         Name,
		Succeeded:       refID != "",
		Token:           refID,
		ReferenceNumber: refID,
		OrderID:         core.FirstValue(values, "clientrefid", "clientRefId"),
		CardNumber:      core.FirstValue(values, "cardnumber", "cardNumber"),
		Values:          values,
	}, nil
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
