// Package payir implements the Pay.ir gateway (REST, pay.ir).
//
// Credentials: [core.Config.MerchantKey] is the API key. [core.WithSandbox]
// replaces it with the literal "test" key Pay.ir reserves for sandboxes.
package payir

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Name is the registry name of this gateway.
const Name core.Name = "payir"

// Provider endpoints.
const (
	defaultBase  = "https://pay.ir"
	sendPath     = "/pg/send"
	verifyPath   = "/pg/verify"
	redirectPath = "/pg/"

	// sandboxAPIKey is the key Pay.ir accepts on its test terminal.
	sandboxAPIKey = "test"
)

// statusOK is the success flag of every Pay.ir response.
const statusOK = 1

// minAmount is the smallest payment Pay.ir accepts, in Rial.
const minAmount = 1000

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the Pay.ir implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers Refund and Inquiry, which Pay.ir does not expose.
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	baseURL  string
	apiKey   string
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a Pay.ir gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)

	apiKey := strings.TrimSpace(cfg.MerchantKey)
	if options.Sandbox {
		apiKey = sandboxAPIKey
	}
	if apiKey == "" {
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("MerchantKey (api key) is required")
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
		apiKey:      apiKey,
	}, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what Pay.ir offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Callback:       true,
		RedirectMethod: http.MethodGet,
		Currencies:     []core.Currency{core.IRR},
	}
}

// Purchase creates a payment and returns the redirect URL.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	if req.Amount.Rial() < minAmount {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be at least 1000 Rial")
	}
	if req.CallbackURL == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("callback url is required")
	}

	description := req.Description
	if description == "" {
		description = g.settings.description
	}

	form := url.Values{}
	form.Set("api", g.apiKey)
	form.Set("amount", strconv.FormatInt(req.Amount.Rial(), 10))
	form.Set("redirect", req.CallbackURL)
	if req.Mobile != "" {
		form.Set("mobile", req.Mobile)
	}
	if description != "" {
		form.Set("description", description)
	}
	if g.settings.sendOrderAsFactor && req.OrderID != "" {
		form.Set("factorNumber", req.OrderID)
	}
	if len(req.AllowedCards) > 0 {
		form.Set("validCardNumber", req.AllowedCards[0])
	}

	var out sendResponse
	res, err := g.client.Form(ctx, transport.JoinURL(g.baseURL, sendPath), form, nil, &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if out.Status != statusOK || out.Token == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.ErrorCode)).WithMessage(out.ErrorMessage)
	}

	return core.PurchaseResponse{
		Token:   out.Token,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     res.Body,
		Redirect: core.Redirect{
			Method: http.MethodGet,
			URL:    transport.JoinURL(g.baseURL, redirectPath+out.Token),
		},
	}, nil
}

// Verify settles the payment identified by the token.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	if req.Token == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("token is required")
	}

	form := url.Values{}
	form.Set("api", g.apiKey)
	form.Set("token", req.Token)

	var out verifyResponse
	res, err := g.client.Form(ctx, transport.JoinURL(g.baseURL, verifyPath), form, nil, &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	if out.Status != statusOK {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.ErrorCode)).WithMessage(out.ErrorMessage)
	}

	amount, err := core.SettledAmount(Name, req.Amount, core.Rial(out.Amount))
	if err != nil {
		return core.VerifyResponse{}, err
	}
	orderID := req.OrderID
	if out.FactorNumber != "" {
		orderID = out.FactorNumber
	}

	return core.VerifyResponse{
		ReferenceNumber: strconv.FormatInt(out.TransID, 10),
		TransactionID:   strconv.FormatInt(out.TransID, 10),
		OrderID:         orderID,
		CardNumber:      out.CardNumber,
		Amount:          amount,
		Raw:             res.Body,
	}, nil
}

// ParseCallback reads the token/status pair Pay.ir returns the payer with.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	status := core.FirstValue(values, "status")
	return core.Callback{
		Gateway:   Name,
		Succeeded: status == "1",
		Token:     core.FirstValue(values, "token"),
		Code:      status,
		Values:    values,
	}, nil
}
