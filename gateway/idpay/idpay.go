// Package idpay implements the IDPay gateway (REST, api.idpay.ir).
//
// Credentials: [core.Config.MerchantKey] is the API key. [core.WithSandbox]
// switches the terminal to IDPay's sandbox by sending the X-SANDBOX header.
package idpay

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Name is the registry name of this gateway.
const Name core.Name = "idpay"

// Provider endpoints.
const (
	defaultBase  = "https://api.idpay.ir"
	purchasePath = "/v1.1/payment"
	verifyPath   = "/v1.1/payment/verify"
	inquiryPath  = "/v1.1/payment/inquiry"
)

// IDPay transaction status codes.
const (
	statusPaidUnverified  = 10
	statusVerified        = 100
	statusAlreadyVerified = 101
	statusSettled         = 200
)

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the IDPay implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers Refund, which IDPay only offers in its panel.
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	baseURL  string
}

var _ core.Gateway = (*Gateway)(nil)

// New builds an IDPay gateway.
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

// Capabilities reports what IDPay offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Inquiry:        true,
		Callback:       true,
		RedirectMethod: http.MethodGet,
		Currencies:     []core.Currency{core.IRR},
	}
}

// headers builds the authenticated headers of every call.
func (g *Gateway) headers() map[string]string {
	headers := map[string]string{"X-API-KEY": g.cfg.MerchantKey}
	if g.opts.Sandbox {
		headers["X-SANDBOX"] = "1"
	}
	return headers
}

// Purchase creates a payment and returns the IDPay link.
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

	var out purchaseResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, purchasePath), purchaseRequest{
		OrderID:  req.OrderID,
		Amount:   req.Amount.Rial(),
		Name:     req.PayerName,
		Phone:    req.Mobile,
		Mail:     req.Email,
		Desc:     description,
		Callback: req.CallbackURL,
	}, g.headers(), &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if out.ID == "" || out.Link == "" {
		return core.PurchaseResponse{}, gatewayError("purchase", res.Body)
	}

	return core.PurchaseResponse{
		Token:    out.ID,
		OrderID:  req.OrderID,
		Amount:   req.Amount,
		Raw:      res.Body,
		Redirect: core.Redirect{Method: http.MethodGet, URL: out.Link},
	}, nil
}

// Verify settles the payment identified by the IDPay id.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	if req.Token == "" || req.OrderID == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("both id and order id are required")
	}

	var out verifyResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, verifyPath),
		lookupRequest{ID: req.Token, OrderID: req.OrderID}, g.headers(), &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	switch out.Status {
	case statusVerified, statusAlreadyVerified, statusSettled:
	default:
		return core.VerifyResponse{}, gatewayError("verify", res.Body).
			WithCode(strconv.Itoa(out.Status))
	}

	amount, err := core.SettledAmount(Name, req.Amount, core.Rial(out.Amount))
	if err != nil {
		return core.VerifyResponse{}, err
	}

	return core.VerifyResponse{
		ReferenceNumber: strconv.FormatInt(out.TrackID, 10),
		TransactionID:   out.ID,
		OrderID:         out.OrderID,
		CardNumber:      out.Payment.CardNo,
		CardHash:        out.Payment.HashedCardNo,
		Amount:          amount,
		PaidAt:          parseUnix(out.Date),
		Raw:             res.Body,
	}, nil
}

// Inquiry reads the current state of a payment.
func (g *Gateway) Inquiry(ctx context.Context, req core.InquiryRequest) (core.InquiryResponse, error) {
	if req.Token == "" || req.OrderID == "" {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", core.ErrInvalidRequest).
			WithMessage("both id and order id are required")
	}

	var out verifyResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, inquiryPath),
		lookupRequest{ID: req.Token, OrderID: req.OrderID}, g.headers(), &out)
	if err != nil {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", err)
	}
	if out.ID == "" {
		return core.InquiryResponse{}, gatewayError("inquiry", res.Body)
	}

	return core.InquiryResponse{
		Status:          mapStatus(out.Status),
		ReferenceNumber: strconv.FormatInt(out.TrackID, 10),
		CardNumber:      out.Payment.CardNo,
		Amount:          core.Rial(out.Amount),
		PaidAt:          parseUnix(out.Date),
		Message:         strconv.Itoa(out.Status),
		Raw:             res.Body,
	}, nil
}

// ParseCallback reads the fields IDPay posts to the callback URL.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	status := core.FirstValue(values, "status")
	code, _ := strconv.Atoi(status)
	return core.Callback{
		Gateway:         Name,
		Succeeded:       code == statusPaidUnverified || code == statusVerified || code == statusSettled,
		Token:           core.FirstValue(values, "id"),
		OrderID:         core.FirstValue(values, "order_id"),
		ReferenceNumber: core.FirstValue(values, "track_id"),
		CardNumber:      core.FirstValue(values, "card_no"),
		Code:            status,
		Values:          values,
	}, nil
}

// mapStatus translates an IDPay status code into the shared vocabulary.
func mapStatus(status int) core.Status {
	switch {
	case status == statusVerified || status == statusAlreadyVerified || status == statusSettled:
		return core.StatusVerified
	case status == statusPaidUnverified:
		return core.StatusPaid
	case status == 1 || status == 2:
		return core.StatusPending
	case status == 3 || status == 4 || status == 5 || status == 6 || status == 7:
		return core.StatusFailed
	default:
		return core.StatusUnknown
	}
}

// gatewayError turns an IDPay error body into a rich error.
func gatewayError(op, body string) *core.Error {
	var detail errorResponse
	if err := json.Unmarshal([]byte(body), &detail); err == nil && detail.ErrorCode != 0 {
		return core.NewError(Name, op, core.ErrPaymentFailed).
			WithCode(strconv.Itoa(detail.ErrorCode)).WithMessage(detail.ErrorMessage)
	}
	return core.NewError(Name, op, core.ErrPaymentFailed).WithMessage(body)
}

// parseUnix converts an IDPay unix timestamp string.
func parseUnix(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(seconds, 0)
}
