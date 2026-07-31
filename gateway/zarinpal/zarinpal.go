// Package zarinpal implements the Zarinpal payment gateway (REST, api.zarinpal.com).
//
// Credentials: [core.Config.MerchantKey] is the 36 character merchant id.
//
//	gw, err := zarinpal.New(core.Config{MerchantKey: merchantID},
//	    zarinpal.WithDefaultDescription("Wallet top-up"))
package zarinpal

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
const Name core.Name = "zarinpal"

// Provider endpoints. The sandbox host mirrors the production API and is
// selected with [core.WithSandbox].
const (
	productionAPI     = "https://api.zarinpal.com"
	productionPayment = "https://payment.zarinpal.com"
	sandboxAPI        = "https://sandbox.zarinpal.com"
	sandboxPayment    = "https://sandbox.zarinpal.com"

	purchasePath = "/pg/v4/payment/request.json"
	verifyPath   = "/pg/v4/payment/verify.json"
	inquiryPath  = "/pg/v4/payment/inquiry.json"
	startPayPath = "/pg/StartPay/"
)

// Zarinpal status codes that mean success.
const (
	codeSuccess         = 100
	codeAlreadyVerified = 101
)

// register makes the gateway reachable through core.New("zarinpal", ...).
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the Zarinpal implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers the operations Zarinpal's public API lacks
	// (refunds require the merchant panel).
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client

	apiBase     string
	paymentBase string
}

// compile time proof that the gateway satisfies the package interface.
var _ core.Gateway = (*Gateway)(nil)

// New builds a Zarinpal gateway. It fails when the merchant id is missing.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	if strings.TrimSpace(cfg.MerchantKey) == "" {
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("MerchantKey (merchant id) is required")
	}

	apiBase, paymentBase := productionAPI, productionPayment
	if options.Sandbox {
		apiBase, paymentBase = sandboxAPI, sandboxPayment
	}
	if options.BaseURL != "" {
		apiBase, paymentBase = options.BaseURL, options.BaseURL
	}

	return &Gateway{
		Unsupported: core.Unsupported{GatewayName: Name},
		cfg:         cfg,
		opts:        options,
		settings:    gwopt.From[config](options, string(Name)),
		client:      transport.New(options),
		apiBase:     apiBase,
		paymentBase: paymentBase,
	}, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what Zarinpal offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Inquiry:        true,
		Callback:       true,
		RedirectMethod: http.MethodGet,
		Multiplexing:   true,
		Currencies:     []core.Currency{core.IRR, core.IRT},
	}
}

// amount converts the request amount to the unit the terminal is configured
// with and returns it together with the Zarinpal currency code.
func (g *Gateway) amount(m core.Money) (int64, string) {
	if g.settings.currencySet && g.settings.currency == core.IRT {
		return m.Toman(), core.IRT.String()
	}
	return m.Rial(), ""
}

// Purchase creates a payment and returns the authority plus the StartPay URL.
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
		description = g.settings.defaultDescription
	}
	if description == "" {
		description = "Payment " + req.OrderID
	}

	amount, currency := g.amount(req.Amount)
	body := purchaseRequest{
		MerchantID:  g.cfg.MerchantKey,
		Amount:      amount,
		CallbackURL: req.CallbackURL,
		Description: description,
		Currency:    currency,
		Wages:       g.settings.wages,
	}
	if req.Mobile != "" || req.Email != "" || req.OrderID != "" {
		body.Metadata = map[string]string{}
		if req.Mobile != "" {
			body.Metadata["mobile"] = req.Mobile
		}
		if req.Email != "" {
			body.Metadata["email"] = req.Email
		}
		if req.OrderID != "" {
			body.Metadata["order_id"] = req.OrderID
		}
	}

	var out purchaseResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.apiBase, purchasePath), body, nil, &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if out.Data.Code != codeSuccess || out.Data.Authority == "" {
		code, message := errorMessage(out.Errors)
		if code == "" {
			code = strconv.Itoa(out.Data.Code)
			message = out.Data.Message
		}
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithCode(code).WithMessage(message)
	}

	return core.PurchaseResponse{
		Token:   out.Data.Authority,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     res.Body,
		Redirect: core.Redirect{
			Method: http.MethodGet,
			URL:    transport.JoinURL(g.paymentBase, startPayPath+out.Data.Authority),
		},
	}, nil
}

// Verify settles the payment identified by the authority.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	if req.Token == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("authority is required")
	}

	amount, _ := g.amount(req.Amount)
	var out verifyResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.apiBase, verifyPath), verifyRequest{
		MerchantID: g.cfg.MerchantKey,
		Amount:     amount,
		Authority:  req.Token,
	}, nil, &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}

	switch out.Data.Code {
	case codeSuccess, codeAlreadyVerified:
	default:
		code, message := errorMessage(out.Errors)
		if code == "" {
			code = strconv.Itoa(out.Data.Code)
			message = out.Data.Message
		}
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithCode(code).WithMessage(message)
	}

	return core.VerifyResponse{
		ReferenceNumber: strconv.FormatInt(out.Data.RefID, 10),
		TransactionID:   req.Token,
		OrderID:         req.OrderID,
		CardNumber:      out.Data.CardPan,
		CardHash:        out.Data.CardHash,
		Amount:          req.Amount,
		Fee:             out.Data.Fee,
		Raw:             res.Body,
	}, nil
}

// Inquiry asks Zarinpal for the current state of an authority.
func (g *Gateway) Inquiry(ctx context.Context, req core.InquiryRequest) (core.InquiryResponse, error) {
	if req.Token == "" {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", core.ErrInvalidRequest).
			WithMessage("authority is required")
	}

	var out inquiryResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.apiBase, inquiryPath), inquiryRequest{
		MerchantID: g.cfg.MerchantKey,
		Authority:  req.Token,
	}, nil, &out)
	if err != nil {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", err)
	}
	if out.Data.Code != codeSuccess {
		code, message := errorMessage(out.Errors)
		if code == "" {
			code = strconv.Itoa(out.Data.Code)
		}
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", core.ErrPaymentFailed).
			WithCode(code).WithMessage(message)
	}

	return core.InquiryResponse{
		Status:  mapStatus(out.Data.Status),
		Amount:  core.Rial(out.Data.Amount),
		Message: out.Data.Status,
		Raw:     res.Body,
	}, nil
}

// mapStatus translates the Zarinpal inquiry status into the shared vocabulary.
func mapStatus(status string) core.Status {
	switch strings.ToUpper(status) {
	case "VERIFIED":
		return core.StatusVerified
	case "PAID":
		return core.StatusPaid
	case "IN_BANK", "INBANK":
		return core.StatusPending
	case "REVERSED", "REFUNDED":
		return core.StatusRefunded
	case "FAILED", "CANCELED":
		return core.StatusFailed
	default:
		return core.StatusUnknown
	}
}

// ParseCallback reads the Authority/Status pair Zarinpal appends to the
// callback URL.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	status := core.FirstValue(values, "Status", "status")
	return core.Callback{
		Gateway:   Name,
		Succeeded: strings.EqualFold(status, "OK"),
		Token:     core.FirstValue(values, "Authority", "authority"),
		OrderID:   core.FirstValue(values, "order_id", "OrderId"),
		Code:      status,
		Values:    values,
	}, nil
}
