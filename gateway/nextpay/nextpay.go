// Package nextpay implements the NextPay gateway (REST, nextpay.org).
//
// Credentials: [core.Config.MerchantKey] is the API key of the terminal.
package nextpay

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
const Name core.Name = "nextpay"

// Provider endpoints.
const (
	defaultBase  = "https://nextpay.org"
	tokenPath    = "/nx/gateway/token"
	verifyPath   = "/nx/gateway/verify"
	redirectPath = "/nx/gateway/payment/"
)

// NextPay result codes.
const (
	// codeTokenIssued is the success code of the token endpoint.
	codeTokenIssued = -1
	// codeVerified is the success code of the verify endpoint.
	codeVerified = 0
	// codeAlreadyVerified means the transaction was settled before.
	codeAlreadyVerified = -49
	// codeRefunded is the success code of a refund. The refund travels through
	// the verify endpoint but does not answer with its success code: NextPay
	// documents -90 as "the transaction was refunded and cancelled", and any
	// other value as "not cancelled".
	codeRefunded = -90
)

// refundFlag is the magic value that turns a verify call into a refund.
const refundFlag = "yes_money_back"

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the NextPay implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers Inquiry, which NextPay does not expose.
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	baseURL  string
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a NextPay gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	if strings.TrimSpace(cfg.MerchantKey) == "" {
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
	}, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what NextPay offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Refund:         true,
		Callback:       true,
		RedirectMethod: http.MethodGet,
		Currencies:     []core.Currency{core.IRR, core.IRT},
	}
}

// amount converts the request amount to the unit the terminal expects.
func (g *Gateway) amount(m core.Money) int64 {
	if g.settings.currency == core.IRT.String() {
		return m.Toman()
	}
	return m.Rial()
}

// Purchase creates a payment and returns the NextPay redirect URL.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	if req.Amount.IsZero() {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be greater than zero")
	}
	if req.OrderID == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("order id is required")
	}

	description := req.Description
	if description == "" {
		description = g.settings.description
	}
	body := tokenRequest{
		APIKey:        g.cfg.MerchantKey,
		OrderID:       req.OrderID,
		Amount:        g.amount(req.Amount),
		CallbackURI:   req.CallbackURL,
		Currency:      g.settings.currency,
		CustomerPhone: req.Mobile,
		PayerName:     req.PayerName,
		PayerDesc:     description,
	}
	if g.settings.autoVerify {
		body.AutoVerify = "yes"
	}
	if len(req.AllowedCards) > 0 {
		body.AllowedCard = req.AllowedCards[0]
	}

	var out tokenResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, tokenPath), body, nil, &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if out.Code != codeTokenIssued || out.TransID == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Code)).WithMessage(Message(out.Code))
	}

	return core.PurchaseResponse{
		Token:   out.TransID,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     res.Body,
		Redirect: core.Redirect{
			Method: http.MethodGet,
			URL:    transport.JoinURL(g.baseURL, redirectPath+out.TransID),
		},
	}, nil
}

// Verify settles the payment identified by the transaction id.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	if req.Token == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("trans_id is required")
	}

	var out verifyResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, verifyPath), verifyRequest{
		APIKey:  g.cfg.MerchantKey,
		TransID: req.Token,
		Amount:  g.amount(req.Amount),
	}, nil, &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	switch out.Code {
	case codeVerified, codeAlreadyVerified:
	default:
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Code)).WithMessage(Message(out.Code))
	}

	amount := req.Amount
	if out.Amount > 0 {
		amount = core.Money{Amount: out.Amount, Currency: req.Amount.Currency}
	}
	orderID := out.OrderID
	if orderID == "" {
		orderID = req.OrderID
	}

	return core.VerifyResponse{
		ReferenceNumber: out.ShaparakRefID,
		TransactionID:   req.Token,
		OrderID:         orderID,
		CardNumber:      out.CardHolder,
		Amount:          amount,
		Raw:             res.Body,
	}, nil
}

// Refund returns a settled payment to the payer's card. NextPay performs it
// through the verification endpoint with the refund flag set.
func (g *Gateway) Refund(ctx context.Context, req core.RefundRequest) (core.RefundResponse, error) {
	token := req.Token
	if token == "" {
		token = req.TransactionID
	}
	if token == "" {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrInvalidRequest).
			WithMessage("trans_id is required")
	}

	var out verifyResponse
	res, err := g.client.NoRetry().JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, verifyPath), verifyRequest{
		APIKey:        g.cfg.MerchantKey,
		TransID:       token,
		Amount:        g.amount(req.Amount),
		RefundRequest: refundFlag,
	}, nil, &out)
	if err != nil {
		return core.RefundResponse{}, core.NewError(Name, "refund", err)
	}
	if out.Code != codeRefunded {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Code)).WithMessage(Message(out.Code))
	}

	return core.RefundResponse{RefundID: token, Amount: req.Amount, Raw: res.Body}, nil
}

// ParseCallback reads the fields NextPay posts to the callback URL.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	return core.Callback{
		Gateway:   Name,
		Succeeded: core.FirstValue(values, "trans_id") != "",
		Token:     core.FirstValue(values, "trans_id"),
		OrderID:   core.FirstValue(values, "order_id"),
		Values:    values,
	}, nil
}

// Message returns the meaning of a NextPay result code.
func Message(code int) string {
	switch code {
	case -1:
		return "token issued"
	case 0:
		return "payment approved"
	case -2:
		return "the transaction was declined by the card issuer"
	case -3:
		return "the payment is still pending"
	case -4:
		return "the amount is invalid"
	case -20:
		return "the api key is missing"
	case -21:
		return "the transaction id is missing"
	case -30:
		return "the transaction was already refunded"
	case -33:
		return "the amount does not match the transaction"
	case -49:
		return "the transaction was already verified"
	case -51:
		return "the transaction failed"
	case -90:
		return "the transaction was refunded and cancelled"
	default:
		return "nextpay error " + strconv.Itoa(code)
	}
}
