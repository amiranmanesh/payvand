// Package sadad implements the Sadad / Bank Melli IPG (REST + 3DES signature,
// sadad.shaparak.ir).
//
// Credentials: [core.Config.TerminalID], [core.Config.MerchantID] and
// [core.Config.MerchantKey], the last one being the base64 terminal key used
// to build the 3DES signature of every call.
package sadad

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/cryptox"
	"github.com/amiranmanesh/payvand/internal/gwopt"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Name is the registry name of this gateway.
const Name core.Name = "sadad"

// Provider endpoints.
const (
	defaultBase  = "https://sadad.shaparak.ir"
	purchasePath = "/vpg/api/v0/Request/PaymentRequest"
	verifyPath   = "/vpg/api/v0/Advice/Verify"
	gatewayPath  = "/VPG/Purchase"
)

// resCodeOK is the success code of every Sadad response.
const resCodeOK = "0"

// localDateTimeLayout is the timestamp format Sadad expects.
const localDateTimeLayout = "01/02/2006 3:04:05 PM"

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the Sadad implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers Refund and Inquiry, which Sadad only offers through
	// the merchant panel.
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	baseURL  string
	now      func() time.Time
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a Sadad gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	switch {
	case strings.TrimSpace(cfg.TerminalID) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).WithMessage("TerminalID is required")
	case strings.TrimSpace(cfg.MerchantID) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).WithMessage("MerchantID is required")
	case strings.TrimSpace(cfg.MerchantKey) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("MerchantKey (base64 terminal key) is required")
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
		now:         time.Now,
	}, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what Sadad offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Callback:       true,
		RedirectMethod: http.MethodGet,
		Currencies:     []core.Currency{core.IRR},
	}
}

// Purchase creates a payment and returns the Sadad purchase URL.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	if req.Amount.IsZero() {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be greater than zero")
	}
	if req.OrderID == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("order id is required")
	}

	// Sadad signs the triple terminal;order;amount with the terminal key.
	signData, err := cryptox.TripleDESEncryptECB(
		[]byte(fmt.Sprintf("%s;%s;%d", g.cfg.TerminalID, req.OrderID, req.Amount.Rial())),
		g.cfg.MerchantKey,
	)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}

	body := paymentRequest{
		TerminalID:      g.cfg.TerminalID,
		MerchantID:      g.cfg.MerchantID,
		Amount:          req.Amount.Rial(),
		SignData:        signData,
		ReturnURL:       req.CallbackURL,
		LocalDateTime:   g.now().Format(localDateTimeLayout),
		OrderID:         req.OrderID,
		ApplicationName: g.settings.applicationName,
		AdditionalData:  g.settings.additionalData,
	}
	if g.settings.sendMobileAsUserID {
		body.UserID = req.Mobile
	}

	var out paymentResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, purchasePath), body, nil, &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if code := resCode(out.ResCode); code != resCodeOK || out.Token == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithCode(code).WithMessage(out.Description)
	}

	return core.PurchaseResponse{
		Token:   out.Token,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     res.Body,
		Redirect: core.Redirect{
			Method: http.MethodGet,
			URL:    transport.JoinURL(g.baseURL, gatewayPath),
			Params: map[string]string{"Token": out.Token},
		},
	}, nil
}

// Verify settles the payment identified by the token.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	if req.Token == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("token is required")
	}

	signData, err := cryptox.TripleDESEncryptECB([]byte(req.Token), g.cfg.MerchantKey)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}

	var out verifyResponseBody
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, verifyPath),
		verifyRequestBody{Token: req.Token, SignData: signData}, nil, &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	if code := resCode(out.ResCode); code != resCodeOK {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithCode(code).WithMessage(out.Description)
	}

	amount := req.Amount
	if out.Amount > 0 {
		amount = core.Rial(out.Amount)
	}
	orderID := req.OrderID
	if out.OrderID > 0 {
		orderID = strconv.FormatInt(out.OrderID, 10)
	}

	return core.VerifyResponse{
		ReferenceNumber: out.RetrivalRefNo,
		TransactionID:   out.SystemTraceNo,
		OrderID:         orderID,
		CardNumber:      out.CardNumberMasked,
		Amount:          amount,
		Raw:             res.Body,
	}, nil
}

// ParseCallback reads the fields Sadad posts to the return URL.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	code := core.FirstValue(values, "ResCode", "rescode")
	return core.Callback{
		Gateway:         Name,
		Succeeded:       code == resCodeOK,
		Token:           core.FirstValue(values, "token", "Token"),
		OrderID:         core.FirstValue(values, "OrderId", "orderId"),
		ReferenceNumber: core.FirstValue(values, "RetrivalRefNo"),
		TraceNumber:     core.FirstValue(values, "SystemTraceNo", "TraceNo"),
		Code:            code,
		Message:         core.FirstValue(values, "Description"),
		Values:          values,
	}, nil
}

// resCode normalises the ResCode field, which Sadad answers with either as a
// JSON number or as a string depending on the endpoint.
func resCode(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}
