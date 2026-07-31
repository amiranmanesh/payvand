// Package payweb implements the PayWeb IPG (REST, ipg.payweb.ir).
//
// Credentials: [core.Config.MerchantKey] is the bearer token of the terminal.
package payweb

import (
	"context"
	"net/http"
	"strings"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Name is the registry name of this gateway.
const Name core.Name = "payweb"

// Provider endpoints.
const (
	defaultBase = "https://ipg.payweb.ir"
	createPath  = "/Payment/create"
	verifyPath  = "/Payment/verify"
	gatewayPath = "/Gateway"
)

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the PayWeb implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers Refund and Inquiry, which PayWeb does not expose.
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	baseURL  string
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a PayWeb gateway.
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

// Capabilities reports what PayWeb offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Callback:       true,
		RedirectMethod: http.MethodGet,
		Currencies:     []core.Currency{core.IRR},
	}
}

// headers builds the authenticated headers of every call.
func (g *Gateway) headers() map[string]string {
	return map[string]string{"Authorization": "Bearer " + g.cfg.MerchantKey}
}

// Purchase creates a payment and returns the gateway URL.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	if req.Amount.IsZero() {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be greater than zero")
	}
	if req.CallbackURL == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("callback url is required")
	}

	comment := req.Description
	if comment == "" {
		comment = g.settings.comment
	}
	body := createRequest{
		Amount:       req.Amount.Rial(),
		CallbackURL:  req.CallbackURL,
		MobileNumber: req.Mobile,
		NationalCode: req.NationalID,
		Comment:      comment,
		ClientRefID:  req.OrderID,
	}
	if g.settings.restrictToCard && len(req.AllowedCards) > 0 {
		body.CardNumber = req.AllowedCards[0]
	}

	var out createResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, createPath), body, g.headers(), &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if !res.OK() || out.Code == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithMessage(firstNonEmpty(out.Message, res.Body))
	}

	return core.PurchaseResponse{
		Token:   out.Code,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     res.Body,
		Redirect: core.Redirect{
			Method: http.MethodGet,
			URL:    transport.JoinURL(g.baseURL, gatewayPath),
			Params: map[string]string{"id": out.Code},
		},
	}, nil
}

// Verify settles the payment identified by the create code.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	if req.Token == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("token is required")
	}

	var out verifyResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, verifyPath),
		verifyRequest{RefID: req.Token}, g.headers(), &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	if !res.OK() {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithMessage(firstNonEmpty(out.Message, res.Body))
	}

	amount := req.Amount
	if out.Amount > 0 {
		amount = core.Rial(int64(out.Amount))
	}
	orderID := out.ClientRefID
	if orderID == "" {
		orderID = req.OrderID
	}

	return core.VerifyResponse{
		ReferenceNumber: firstNonEmpty(out.TrackingCode, req.Token),
		TransactionID:   req.Token,
		OrderID:         orderID,
		CardNumber:      out.CardNumber,
		Amount:          amount,
		Raw:             res.Body,
	}, nil
}

// ParseCallback reads the identifiers PayWeb returns the payer with.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	status := core.FirstValue(values, "status", "Status")
	return core.Callback{
		Gateway:   Name,
		Succeeded: status == "" || strings.EqualFold(status, "OK") || status == "1",
		Token:     core.FirstValue(values, "refId", "id", "code"),
		OrderID:   core.FirstValue(values, "clientRefId"),
		Code:      status,
		Values:    values,
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
