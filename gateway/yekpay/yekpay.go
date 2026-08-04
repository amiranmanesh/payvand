// Package yekpay implements the YekPay gateway (REST, gate.yekpay.com).
//
// Credentials: [core.Config.MerchantKey] is the merchant id. YekPay is
// multi-currency; see [WithCurrencies].
//
// [core.WithSandbox] switches the terminal to YekPay's public sandbox, which
// runs the whole cycle against the same merchant id and lets the tester pick
// the outcome on the payment page. The sandbox is a different host *and* a
// different path set, not a mirror of the production paths.
package yekpay

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Name is the registry name of this gateway.
const Name core.Name = "yekpay"

// Provider hosts. The sandbox is a host of its own rather than a flag on the
// production one.
const (
	productionBase = "https://gate.yekpay.com"
	sandboxBase    = "https://api.ypsapi.com"
)

// endpoints is the path set of one YekPay environment. The sandbox does not
// mirror the production paths, so the host and the paths are always chosen
// together.
type endpoints struct {
	// request creates the payment and returns the authority.
	request string
	// verify settles it.
	verify string
	// redirect is the prefix the authority is appended to.
	redirect string
}

var (
	// productionPaths are the REST paths of the live gateway. The neighbouring
	// /api/payment/server is the SOAP endpoint and answers a JSON body with a
	// SOAP fault, so it is not the one to post to.
	productionPaths = endpoints{
		request:  "/api/payment/request",
		verify:   "/api/payment/verify",
		redirect: "/api/payment/start/",
	}
	// sandboxPaths are the same three calls in YekPay's test environment.
	sandboxPaths = endpoints{
		request:  "/api/sandbox/request",
		verify:   "/api/sandbox/verify",
		redirect: "/api/sandbox/payment/",
	}
)

// codeSuccess is the success code of every YekPay response.
const codeSuccess = 100

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the YekPay implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers Refund and Inquiry, which YekPay keeps in its panel.
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	baseURL  string
	paths    endpoints
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a YekPay gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	if strings.TrimSpace(cfg.MerchantKey) == "" {
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("MerchantKey (merchant id) is required")
	}

	baseURL, paths := productionBase, productionPaths
	if options.Sandbox {
		baseURL, paths = sandboxBase, sandboxPaths
	}
	// A pinned base URL replaces the host only: the caller is redirecting the
	// chosen environment somewhere else, not asking for the other one's paths.
	if options.BaseURL != "" {
		baseURL = options.BaseURL
	}

	gateway := &Gateway{
		Unsupported: core.Unsupported{GatewayName: Name},
		cfg:         cfg,
		opts:        options,
		settings:    gwopt.From[config](options, string(Name)),
		client:      transport.New(options),
		baseURL:     baseURL,
		paths:       paths,
	}
	if gateway.settings.fromCurrency == 0 {
		gateway.settings.fromCurrency = CurrencyIRR
	}
	if gateway.settings.toCurrency == 0 {
		gateway.settings.toCurrency = CurrencyIRR
	}
	return gateway, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what YekPay offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Callback:       true,
		RedirectMethod: http.MethodGet,
		Currencies:     []core.Currency{core.IRR},
	}
}

// Purchase creates a payment and returns the YekPay start URL.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	if req.Amount.IsZero() {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be greater than zero")
	}
	if req.CallbackURL == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("callback url is required")
	}

	description := req.Description
	if description == "" {
		description = g.settings.description
	}
	first, last := splitName(req.PayerName)

	var out requestResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, g.paths.request), requestBody{
		MerchantID:       g.cfg.MerchantKey,
		Amount:           g.amount(req.Amount),
		FromCurrencyCode: g.settings.fromCurrency,
		ToCurrencyCode:   g.settings.toCurrency,
		OrderNumber:      req.OrderID,
		Callback:         req.CallbackURL,
		FirstName:        first,
		LastName:         last,
		Email:            req.Email,
		Mobile:           req.Mobile,
		Address:          g.settings.address,
		Description:      description,
	}, nil, &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if out.Code != codeSuccess || out.Authority == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Code)).WithMessage(out.Description)
	}

	return core.PurchaseResponse{
		Token:   out.Authority,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     res.Body,
		Redirect: core.Redirect{
			Method: http.MethodGet,
			URL:    transport.JoinURL(g.baseURL, g.paths.redirect+out.Authority),
		},
	}, nil
}

// Verify settles the payment identified by the authority.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	if req.Token == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("authority is required")
	}

	var out verifyResponseBody
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, g.paths.verify), verifyBody{
		MerchantID: g.cfg.MerchantKey,
		Authority:  req.Token,
	}, nil, &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	if out.Code != codeSuccess {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Code)).WithMessage(out.Description)
	}

	// YekPay states the settled amount in the currency the payer was charged
	// in. That is a [core.Money] only when the terminal sells in Rial; for the
	// other currencies the figures are compared as the provider counts them,
	// since core.Money carries no unit for them.
	amount := req.Amount
	if g.settings.fromCurrency == CurrencyIRR {
		settled, err := core.SettledAmount(Name, req.Amount, core.Rial(out.Amount))
		if err != nil {
			return core.VerifyResponse{}, err
		}
		amount = settled
	} else if out.Amount > 0 && !req.Amount.IsZero() && out.Amount != g.amount(req.Amount) {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrAmountMismatch).
			WithMessage("the provider settled " + strconv.FormatInt(out.Amount, 10) +
				" in currency " + strconv.Itoa(g.settings.fromCurrency))
	}

	orderID := out.OrderNumber
	if orderID == "" {
		orderID = req.OrderID
	}

	return core.VerifyResponse{
		ReferenceNumber: out.Reference,
		TransactionID:   req.Token,
		OrderID:         orderID,
		Amount:          amount,
		Raw:             res.Body,
	}, nil
}

// ParseCallback reads the authority YekPay returns the payer with.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	status := core.FirstValue(values, "status", "Status")
	authority := core.FirstValue(values, "authority", "Authority")
	return core.Callback{
		Gateway:   Name,
		Succeeded: authority != "" && !strings.EqualFold(status, "NOK") && status != "-1",
		Token:     authority,
		OrderID:   core.FirstValue(values, "order_number", "orderNumber"),
		Code:      status,
		Values:    values,
	}, nil
}

// amount converts the payment amount for the configured source currency:
// YekPay expects Rial for the Iranian currency and major units elsewhere.
func (g *Gateway) amount(m core.Money) int64 {
	if g.settings.fromCurrency == CurrencyIRR {
		return m.Rial()
	}
	return m.Amount
}

// splitName splits a full name into the first/last pair YekPay expects.
func splitName(fullName string) (first, last string) {
	fields := strings.Fields(fullName)
	switch len(fields) {
	case 0:
		return "", ""
	case 1:
		return fields[0], ""
	default:
		return fields[0], strings.Join(fields[1:], " ")
	}
}
