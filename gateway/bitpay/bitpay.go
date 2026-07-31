// Package bitpay implements the BitPay.ir gateway (REST, bitpay.ir).
//
// Credentials: [core.Config.MerchantKey] is the API key. The literal
// "bitpay.ir-demo-api-key" is BitPay's own sandbox key and is used when the
// gateway is built with [core.WithSandbox].
package bitpay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Name is the registry name of this gateway.
const Name core.Name = "bitpay"

// Provider endpoints.
const (
	defaultBase = "https://bitpay.ir"
	sendPath    = "/payment/gateway-send"
	verifyPath  = "/payment/gateway-result-second"

	// sandboxAPIKey is BitPay's public demo key.
	sandboxAPIKey = "bitpay.ir-demo-api-key"
)

// BitPay verification statuses that mean the payment is settled.
const (
	statusVerified        = 1
	statusAlreadyVerified = 11
)

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the BitPay.ir implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers Refund and Inquiry, which BitPay does not expose.
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	baseURL  string
	apiKey   string
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a BitPay.ir gateway.
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

// Capabilities reports what BitPay offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Callback:       true,
		RedirectMethod: http.MethodGet,
		Currencies:     []core.Currency{core.IRR},
	}
}

// Purchase creates a payment and returns the BitPay redirect URL.
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

	form := url.Values{}
	form.Set("api", g.apiKey)
	form.Set("amount", strconv.FormatInt(req.Amount.Rial(), 10))
	form.Set("redirect", req.CallbackURL)
	if req.OrderID != "" {
		form.Set("factorId", req.OrderID)
	}
	if req.PayerName != "" {
		form.Set("name", req.PayerName)
	}
	if req.Email != "" {
		form.Set("email", req.Email)
	}
	if description != "" {
		form.Set("description", description)
	}

	res, err := g.client.Form(ctx, transport.JoinURL(g.baseURL, sendPath), form, nil, nil)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}

	id := parseID(res.Body)
	if id <= 0 {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithCode(strconv.FormatInt(id, 10)).WithMessage(Message(id))
	}

	token := strconv.FormatInt(id, 10)
	return core.PurchaseResponse{
		Token:   token,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     res.Body,
		Redirect: core.Redirect{
			Method: http.MethodGet,
			URL:    transport.JoinURL(g.baseURL, "/payment/gateway-"+token+"-get"),
		},
	}, nil
}

// Verify settles the payment. BitPay identifies it by the pair (id_get,
// trans_id); the transaction id comes from the callback.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	transID := firstNonEmpty(req.ReferenceNumber, req.Get("trans_id"))
	if req.Token == "" || transID == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("both id_get and trans_id are required")
	}

	form := url.Values{}
	form.Set("api", g.apiKey)
	form.Set("id_get", req.Token)
	form.Set("trans_id", transID)

	var out verifyResponse
	res, err := g.client.Form(ctx, transport.JoinURL(g.baseURL, verifyPath), form, nil, &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	switch out.Status {
	case statusVerified:
	case statusAlreadyVerified:
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrAlreadyVerified).
			WithCode(strconv.Itoa(out.Status)).WithMessage(firstNonEmpty(out.Message, out.Description))
	default:
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Status)).WithMessage(firstNonEmpty(out.Message, out.Description))
	}

	var reported core.Money
	if parsed, convErr := strconv.ParseInt(out.Amount, 10, 64); convErr == nil {
		reported = core.Rial(parsed)
	}
	amount, err := core.SettledAmount(Name, req.Amount, reported)
	if err != nil {
		return core.VerifyResponse{}, err
	}
	orderID := out.FactorID
	if orderID == "" {
		orderID = req.OrderID
	}

	return core.VerifyResponse{
		ReferenceNumber: transID,
		TransactionID:   transID,
		OrderID:         orderID,
		CardNumber:      out.CardNum,
		Amount:          amount,
		Raw:             res.Body,
	}, nil
}

// ParseCallback reads the id_get/trans_id pair BitPay returns the payer with.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	transID := core.FirstValue(values, "trans_id")
	return core.Callback{
		Gateway:         Name,
		Succeeded:       transID != "" && transID != "0",
		Token:           core.FirstValue(values, "id_get"),
		OrderID:         core.FirstValue(values, "factorId", "factor_id"),
		ReferenceNumber: transID,
		Values:          values,
	}, nil
}

// parseID reads the payment id out of the send response, which BitPay returns
// either as a bare number or as a small JSON object.
func parseID(body string) int64 {
	trimmed := strings.TrimSpace(body)
	if id, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return id
	}
	var out sendResponse
	if err := json.Unmarshal([]byte(trimmed), &out); err == nil {
		return out.IDGet
	}
	return 0
}

// Message returns the meaning of a BitPay error code.
func Message(code int64) string {
	switch code {
	case -1:
		return "the api key is missing"
	case -2:
		return "the amount is not a valid number"
	case -3:
		return "the amount is below the minimum of 1000 Rial"
	case -4:
		return "the api key is invalid"
	case -5:
		return "the callback url is missing"
	default:
		return "bitpay error " + strconv.FormatInt(code, 10)
	}
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
