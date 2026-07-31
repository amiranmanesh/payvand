// Package torobpay implements the TorobPay online credit gateway (REST,
// api.torobpay.com).
//
// TorobPay is Torob's buy-now-pay-later product: the payer settles the invoice
// in four instalments and the merchant is paid in full. The API is shaped like
// an OAuth protected REST service — a payment token, a hosted page, then a
// verification — with reversal and cancellation as separate calls.
//
// Credentials: [core.Config.Username] and [core.Config.Password] are the
// merchant user of the OAuth password grant, while [core.Config.MerchantID]
// and [core.Config.MerchantKey] are the OAuth client id and client secret.
package torobpay

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
	"github.com/amiranmanesh/payvand/internal/tokenauth"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Name is the registry name of this gateway.
const Name core.Name = "torobpay"

// Provider endpoints.
const (
	defaultBaseURL = "https://api.torobpay.com"

	oauthPath  = "/api/online/v1/oauth/token"
	tokenPath  = "/api/online/payment/v1/token"
	verifyPath = "/api/online/payment/v1/verify"
	revertPath = "/api/online/payment/v1/revert"
	statusPath = "/api/online/payment/v1/status"
	cancelPath = "/api/online/payment/v1/cancel"
)

// Defaults applied when the corresponding option is not set.
const (
	defaultPaymentMethod = "ONLINE_CREDIT"
	defaultCategory      = "OTHER"
)

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the TorobPay implementation of [core.Gateway].
type Gateway struct {
	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	tokens   *tokenauth.Cache
	auth     *tokenauth.Client

	baseURL string
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a TorobPay gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	switch {
	case strings.TrimSpace(cfg.Username) == "" || strings.TrimSpace(cfg.Password) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("Username and Password of the merchant user are required")
	case strings.TrimSpace(cfg.MerchantID) == "" || strings.TrimSpace(cfg.MerchantKey) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("MerchantID (client id) and MerchantKey (client secret) are required")
	}

	baseURL := defaultBaseURL
	if options.BaseURL != "" {
		baseURL = options.BaseURL
	}

	gw := &Gateway{
		cfg:      cfg,
		opts:     options,
		settings: gwopt.From[config](options, string(Name)),
		client:   transport.New(options),
		tokens:   &tokenauth.Cache{},
		baseURL:  baseURL,
	}
	gw.auth = &tokenauth.Client{Cache: gw.tokens, Transport: gw.client, Fetch: gw.fetchToken}
	return gw, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what TorobPay offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Refund:         true,
		Inquiry:        true,
		Callback:       true,
		RedirectMethod: http.MethodGet,
		Currencies:     []core.Currency{core.IRR},
	}
}

// Purchase creates a credit payment and returns the TorobPay page the payer is
// sent to.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	switch {
	case req.Amount.Rial() <= 0:
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be positive")
	case req.CallbackURL == "":
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("callback url is required")
	case req.OrderID == "":
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("order id is required; TorobPay uses it as transactionId")
	}

	body := tokenRequest{
		Amount:               req.Amount.Rial(),
		Mobile:               req.Mobile,
		PaymentMethodTypeDto: g.paymentMethod(),
		TransactionID:        req.OrderID,
		ReturnURL:            req.CallbackURL,
		CartList:             g.carts(req),
	}

	var out tokenResponse
	res, err := g.auth.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, tokenPath), body, nil, &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if !res.OK() || out.Response.PaymentToken == "" {
		return core.PurchaseResponse{}, failure("purchase", out.envelope, "torobpay rejected the payment request")
	}

	return core.PurchaseResponse{
		Token:    out.Response.PaymentToken,
		OrderID:  req.OrderID,
		Amount:   req.Amount,
		Raw:      res.Body,
		Redirect: core.Redirect{Method: http.MethodGet, URL: out.Response.PaymentPageURL},
	}, nil
}

// Verify confirms a completed payment. TorobPay settles the credit itself, so
// a successful verification is the end of the flow.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	if req.Token == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("token is required")
	}

	verified, res, err := g.call(ctx, "verify", verifyPath, req.Token)
	if err != nil {
		return core.VerifyResponse{}, err
	}

	amount, err := core.SettledAmount(Name, req.Amount, core.Rial(verified.Amount))
	if err != nil {
		return core.VerifyResponse{}, err
	}
	reference := verified.ReferenceNumber
	if reference == "" {
		reference = req.ReferenceNumber
	}

	return core.VerifyResponse{
		ReferenceNumber: reference,
		TransactionID:   verified.TransactionID,
		OrderID:         req.OrderID,
		CardNumber:      req.CardNumber,
		Amount:          amount,
		Raw:             res.Body,
	}, nil
}

// Refund reverts a verified payment, cancelling the payer's instalment plan.
func (g *Gateway) Refund(ctx context.Context, req core.RefundRequest) (core.RefundResponse, error) {
	if req.Token == "" {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrInvalidRequest).
			WithMessage("token is required")
	}

	reverted, res, err := g.call(ctx, "refund", revertPath, req.Token)
	if err != nil {
		return core.RefundResponse{}, err
	}

	amount := req.Amount
	if reverted.Amount > 0 {
		amount = core.Rial(reverted.Amount)
	}
	return core.RefundResponse{RefundID: reverted.TransactionID, Amount: amount, Raw: res.Body}, nil
}

// Cancel drops a payment the payer never completed, freeing the credit
// TorobPay reserved for it.
func (g *Gateway) Cancel(ctx context.Context, paymentToken string) error {
	_, _, err := g.call(ctx, "cancel", cancelPath, paymentToken)
	return err
}

// Inquiry reads the current state of a payment, which is how a lost callback
// is recovered from.
func (g *Gateway) Inquiry(ctx context.Context, req core.InquiryRequest) (core.InquiryResponse, error) {
	if req.Token == "" {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", core.ErrInvalidRequest).
			WithMessage("token is required")
	}
	endpoint := transport.JoinURL(g.baseURL, statusPath) + "?paymentToken=" + url.QueryEscape(req.Token)

	var out statusResponse
	res, err := g.auth.JSON(ctx, http.MethodGet, endpoint, nil, nil, &out)
	if err != nil {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", err)
	}
	if !res.OK() || !out.Successful {
		return core.InquiryResponse{}, failure("inquiry", out.envelope, "torobpay rejected the status request")
	}

	return core.InquiryResponse{
		Status:  mapStatus(out.Response.Status),
		Amount:  core.Rial(out.Response.Amount),
		Message: out.Response.Status,
		Raw:     res.Body,
	}, nil
}

// ParseCallback reads the parameters TorobPay returns the payer with. The
// payment is only final once [Gateway.Verify] succeeded.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}

	status := core.FirstValue(values, "status", "state", "result")
	token := core.FirstValue(values, "paymentToken", "payment_token", "token")

	return core.Callback{
		Gateway:   Name,
		Succeeded: succeeded(status, token),
		Token:     token,
		OrderID:   core.FirstValue(values, "transactionId", "transaction_id"),
		Code:      status,
		Message:   core.FirstValue(values, "message", "errorMessage"),
		Values:    values,
	}, nil
}

// call performs one of the paymentToken shaped POST endpoints.
func (g *Gateway) call(ctx context.Context, op, path, paymentToken string) (verifyData, transport.Response, error) {
	if paymentToken == "" {
		return verifyData{}, transport.Response{}, core.NewError(Name, op, core.ErrInvalidRequest).
			WithMessage("payment token is required")
	}

	var out verifyResponse
	res, err := g.auth.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, path),
		paymentTokenRequest{PaymentToken: paymentToken}, nil, &out)
	if err != nil {
		return verifyData{}, res, core.NewError(Name, op, err)
	}
	if !res.OK() || !out.Successful {
		return verifyData{}, res, failure(op, out.envelope, "torobpay rejected the "+op)
	}
	return out.Response, res, nil
}

// fetchToken runs the OAuth password grant and returns the bearer token.
func (g *Gateway) fetchToken(ctx context.Context) (string, time.Duration, error) {
	form := url.Values{
		"grant_type": {"password"},
		"username":   {g.cfg.Username},
		"password":   {g.cfg.Password},
	}
	headers := map[string]string{
		"Authorization": "Basic " + basicAuth(g.cfg.MerchantID, g.cfg.MerchantKey),
	}

	var out oauthResponse
	if _, err := g.client.Form(ctx, transport.JoinURL(g.baseURL, oauthPath), form, headers, &out); err != nil {
		return "", 0, core.NewError(Name, "token", err)
	}
	if out.AccessToken == "" {
		message := out.Description
		if message == "" {
			message = "torobpay refused the merchant credentials"
		}
		return "", 0, core.NewError(Name, "token", core.ErrInvalidConfig).
			WithCode(out.Error).WithMessage(message)
	}
	return out.AccessToken, time.Duration(out.ExpiresIn) * time.Second, nil
}

// paymentMethod returns the configured TorobPay payment method type.
func (g *Gateway) paymentMethod() string {
	if g.settings.paymentMethod != "" {
		return g.settings.paymentMethod
	}
	return defaultPaymentMethod
}

// carts builds the basket list of a purchase, falling back to a single line
// covering the whole order.
func (g *Gateway) carts(req core.PurchaseRequest) []Cart {
	if g.settings.cartBuilder != nil {
		return g.settings.cartBuilder(req)
	}

	category := g.settings.defaultCategory
	if category == "" {
		category = defaultCategory
	}
	name := req.Description
	if name == "" {
		name = "Order " + req.OrderID
	}

	return []Cart{{
		CartID:      req.OrderID,
		TotalAmount: req.Amount.Rial(),
		CartItems: []CartItem{{
			ID:       req.OrderID,
			Name:     name,
			Count:    1,
			Amount:   req.Amount.Rial(),
			Category: category,
		}},
	}}
}

// failure turns a rejected envelope into the package error type.
func failure(op string, env envelope, fallback string) error {
	e := env.failure()
	return core.NewError(Name, op, core.ErrPaymentFailed).
		WithCode(e.code()).
		WithMessage(e.message(fallback))
}

// succeeded interprets the status of a callback.
func succeeded(status, token string) bool {
	switch strings.ToUpper(status) {
	case "OK", "SUCCESS", "SUCCESSFUL", "TRUE", "1", "COMPLETED", "VERIFY":
		return true
	case "":
		return token != ""
	default:
		return false
	}
}

// mapStatus converts a TorobPay payment state into the normalised status.
func mapStatus(status string) core.Status {
	switch strings.ToUpper(status) {
	case "INIT", "PENDING", "IN_PROGRESS":
		return core.StatusPending
	case "VERIFY", "VERIFIED", "SUCCESS", "SUCCESSFUL":
		return core.StatusVerified
	case "REVERT", "REVERTED", "REFUNDED":
		return core.StatusRefunded
	case "CANCEL", "CANCELED", "CANCELLED":
		return core.StatusCanceled
	case "FAILED", "FAILURE", "EXPIRED":
		return core.StatusFailed
	default:
		return core.StatusUnknown
	}
}

// basicAuth builds the HTTP basic credential of the OAuth client.
func basicAuth(clientID, clientSecret string) string {
	return base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
}
