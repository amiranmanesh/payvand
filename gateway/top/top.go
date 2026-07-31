// Package top implements the TOP (Taban Ati Pardaz) in-app gateway
// (REST, merchantapi.top.ir).
//
// Unlike browser gateways, the payer completes the payment inside the TOP
// application: the merchant server issues a token, the mobile client uses it,
// and the server confirms afterwards. There is therefore no redirect URL and
// no HTTP callback, so [Gateway.ParseCallback] reports [core.ErrNotSupported].
//
// Credentials: [core.Config.MerchantKey] is the EShop "Pin" of the merchant.
package top

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
const Name core.Name = "top"

// Provider endpoints.
const (
	defaultBase = "https://merchantapi.top.ir/api/EShop"
	tokenPath   = "/GetToken"
	confirmPath = "/Confirm"
	setDataPath = "/SetData"
	inquiryPath = "/Inquiry"
)

// statusOK is the success status of every TOP response envelope.
const statusOK = 0

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the TOP implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers Refund and ParseCallback: an unconfirmed TOP
	// transaction is refunded automatically about thirty minutes after the
	// deduction, and the result never reaches the merchant server directly.
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	baseURL  string
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a TOP gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	if strings.TrimSpace(cfg.MerchantKey) == "" {
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("MerchantKey (EShop pin) is required")
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

// Capabilities reports what TOP offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:     true,
		Inquiry:    true,
		Callback:   false,
		Currencies: []core.Currency{core.IRR},
	}
}

// Purchase issues the token the TOP mobile client pays with. The returned
// redirect is empty on purpose: the payment happens inside the app.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	if req.Amount.IsZero() {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be greater than zero")
	}
	orderID, err := strconv.ParseInt(req.OrderID, 10, 64)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("order id must be a unique number")
	}

	body := getTokenRequest{
		Amount:         req.Amount.Rial(),
		OrderID:        orderID,
		Pin:            g.cfg.MerchantKey,
		UserID:         g.settings.userID,
		AdditionalInfo: g.settings.additionalInfo,
		CallBackURL:    req.CallbackURL,
	}
	// TOP expects the mobile number as a number, so the leading zero is
	// dropped by the conversion, exactly as the provider expects.
	if mobile, convErr := strconv.ParseInt(req.Mobile, 10, 64); convErr == nil {
		body.MobileNo = mobile
	}

	var out getTokenResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, tokenPath), body, nil, &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if out.Status != statusOK || out.Data.Token == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Status)).WithMessage(out.Message)
	}

	return core.PurchaseResponse{
		Token:   out.Data.Token,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     res.Body,
	}, nil
}

// Verify confirms the in-app payment. When [WithSetData] is configured, the
// supplementary payload is pushed afterwards; a failure there is logged and
// never fails the already settled payment.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	if req.Token == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("token is required")
	}

	var out confirmResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, confirmPath),
		tokenBody{Token: req.Token}, nil, &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	if out.Status != statusOK {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Status)).WithMessage(out.Message)
	}

	if g.settings.setData != nil {
		if setErr := g.setData(ctx, req.Token, *g.settings.setData); setErr != nil {
			g.opts.Logger.Error(ctx, "payvand: top SetData failed", setErr,
				map[string]string{"gateway": string(Name), "token": req.Token})
		}
	}

	return core.VerifyResponse{
		ReferenceNumber: out.Data.InvoiceNumber,
		TransactionID:   req.Token,
		OrderID:         req.OrderID,
		// TOP does not echo the amount on confirmation, so the requested one
		// is carried forward for the caller's amount check.
		Amount: req.Amount,
		Raw:    res.Body,
	}, nil
}

// Inquiry reads the state of an in-app transaction.
func (g *Gateway) Inquiry(ctx context.Context, req core.InquiryRequest) (core.InquiryResponse, error) {
	if req.Token == "" {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", core.ErrInvalidRequest).
			WithMessage("token is required")
	}

	var out inquiryResponseBody
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, inquiryPath),
		tokenBody{Token: req.Token}, nil, &out)
	if err != nil {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", err)
	}
	if out.Status != statusOK {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Status)).WithMessage(out.Message)
	}

	return core.InquiryResponse{
		Status:          mapStatus(out.Data.Status),
		ReferenceNumber: out.Data.RRN,
		Amount:          core.Rial(out.Data.Amount),
		Message:         out.Message,
		Raw:             res.Body,
	}, nil
}

// setData pushes the supplementary payload of a confirmed transaction.
func (g *Gateway) setData(ctx context.Context, token string, data AdditionalData) error {
	var out setDataResponse
	if _, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, setDataPath),
		setDataRequest{Token: token, AdditionalData: data}, nil, &out); err != nil {
		return err
	}
	if out.Status != statusOK {
		return core.NewError(Name, "setdata", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Status)).WithMessage(out.Message)
	}
	return nil
}

// mapStatus translates a TOP transaction status into the shared vocabulary.
func mapStatus(status int) core.Status {
	switch status {
	case 0:
		return core.StatusVerified
	case 1:
		return core.StatusPaid
	case 2:
		return core.StatusPending
	default:
		return core.StatusFailed
	}
}
