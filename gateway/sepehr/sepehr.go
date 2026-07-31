// Package sepehr implements the Sepehr / Bank Saderat (Mabna) IPG
// (REST, sepehr.shaparak.ir).
//
// Credentials: [core.Config.TerminalID] is the terminal id.
//
// The settlement step is called "advice" by the provider and is keyed on the
// digital receipt that arrives with the callback, so pass it in
// [core.VerifyRequest.Extra] or let [core.Callback.VerifyRequest] do it.
package sepehr

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Name is the registry name of this gateway.
const Name core.Name = "sepehr"

// Provider endpoints. The API and the payment page live on different hosts.
const (
	defaultAPIBase = "https://sepehr.shaparak.ir:8081"
	defaultPayBase = "https://sepehr.shaparak.ir"

	tokenPath    = "/V1/PeymentApi/GetToken"
	advicePath   = "/V1/PeymentApi/Advice"
	rollbackPath = "/V1/PeymentApi/Rollback"
	gatewayPath  = "/_ipgw_/payment/"
)

// Provider result markers.
const (
	// statusTokenOK is the success status of the token endpoint.
	statusTokenOK = 0
	// statusAdviceOK is the success status of the advice endpoint.
	statusAdviceOK = "ok"
	// statusAdviceDuplicate means the receipt was already settled.
	statusAdviceDuplicate = "duplicate"
	// callbackOK is the response code of a successful payment.
	callbackOK = "0"
)

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the Sepehr implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers Inquiry, which Sepehr does not expose.
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	apiBase  string
	payBase  string
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a Sepehr gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	if strings.TrimSpace(cfg.TerminalID) == "" {
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("TerminalID is required")
	}

	apiBase, payBase := defaultAPIBase, defaultPayBase
	if options.BaseURL != "" {
		apiBase, payBase = options.BaseURL, options.BaseURL
	}

	return &Gateway{
		Unsupported: core.Unsupported{GatewayName: Name},
		cfg:         cfg,
		opts:        options,
		settings:    gwopt.From[config](options, string(Name)),
		client:      transport.New(options),
		apiBase:     apiBase,
		payBase:     payBase,
	}, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what Sepehr offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Refund:         true,
		Callback:       true,
		RedirectMethod: http.MethodPost,
		Currencies:     []core.Currency{core.IRR},
	}
}

// Purchase creates a payment token and returns the form the payer is posted
// with.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	if req.Amount.IsZero() {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be greater than zero")
	}
	if req.OrderID == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("order id is required")
	}

	body := tokenRequest{
		Amount:      req.Amount.Rial(),
		CallbackURL: req.CallbackURL,
		InvoiceID:   req.OrderID,
		TerminalID:  g.cfg.TerminalID,
		Payload:     g.settings.payload,
	}
	if g.settings.sendPayerDetails {
		body.MobileNo = req.Mobile
		body.NationalCode = req.NationalID
	}

	var out tokenResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.apiBase, tokenPath), body, nil, &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if out.Status != statusTokenOK || out.AccessToken == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Status)).WithMessage(out.Message)
	}

	return core.PurchaseResponse{
		Token:   out.AccessToken,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     res.Body,
		Redirect: core.Redirect{
			Method: http.MethodPost,
			URL:    transport.JoinURL(g.payBase, gatewayPath),
			Params: map[string]string{
				"TerminalID": g.cfg.TerminalID,
				"token":      out.AccessToken,
			},
		},
	}, nil
}

// Verify settles the payment through the advice endpoint.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	receipt := receiptOf(req.Extra, req.Token)
	if receipt == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("the digital receipt of the callback is required")
	}
	// The advice call carries the receipt and the terminal and nothing else, so
	// Sepehr will settle any receipt issued to the terminal against any order.
	// What the callback claimed the payment was for is checked against the
	// caller's own record first; it does not prove the receipt belongs to this
	// order, but it does stop a receipt landing on an order it never paid for
	// unless the callback was rewritten to match.
	if err := matchesCallback(req); err != nil {
		return core.VerifyResponse{}, err
	}

	var out adviceResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.apiBase, advicePath),
		adviceRequest{DigitalReceipt: receipt, Tid: g.cfg.TerminalID}, nil, &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	status := strings.ToLower(strings.TrimSpace(out.Status))
	switch {
	case status == statusAdviceDuplicate:
		// The receipt was settled by an earlier advice. That is a normal
		// answer to a retried callback and a wrong one to a fresh order, and
		// only the caller knows which it is holding, so it is reported rather
		// than passed off as this payment's settlement.
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrAlreadyVerified).
			WithCode(out.Status).WithMessage(firstNonEmpty(out.Message, out.Description))
	case status != statusAdviceOK:
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithCode(out.Status).WithMessage(firstNonEmpty(out.Message, out.Description))
	}

	return core.VerifyResponse{
		ReferenceNumber: firstNonEmpty(numeric(out.ReturnID), req.ReferenceNumber, receipt),
		TransactionID:   receipt,
		OrderID:         req.OrderID,
		CardNumber:      req.CardNumber,
		Amount:          req.Amount,
		Raw:             res.Body,
	}, nil
}

// Refund rolls a settled receipt back.
func (g *Gateway) Refund(ctx context.Context, req core.RefundRequest) (core.RefundResponse, error) {
	receipt := receiptOf(req.Extra, firstNonEmpty(req.TransactionID, req.Token))
	if receipt == "" {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrInvalidRequest).
			WithMessage("the digital receipt is required")
	}

	var out adviceResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.apiBase, rollbackPath),
		adviceRequest{DigitalReceipt: receipt, Tid: g.cfg.TerminalID}, nil, &out)
	if err != nil {
		return core.RefundResponse{}, core.NewError(Name, "refund", err)
	}
	if strings.ToLower(strings.TrimSpace(out.Status)) != statusAdviceOK {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrPaymentFailed).
			WithCode(out.Status).WithMessage(firstNonEmpty(out.Message, out.Description))
	}

	return core.RefundResponse{RefundID: receipt, Amount: req.Amount, Raw: res.Body}, nil
}

// ParseCallback reads the form Sepehr posts to the callback URL.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	code := core.FirstValue(values, "respcode", "RespCode")

	callback := core.Callback{
		Gateway:         Name,
		Succeeded:       code == callbackOK,
		Token:           core.FirstValue(values, "digitalreceipt", "DigitalReceipt"),
		OrderID:         core.FirstValue(values, "invoiceid", "InvoiceId"),
		ReferenceNumber: core.FirstValue(values, "rrn", "RRN"),
		TraceNumber:     core.FirstValue(values, "tracenumber", "TraceNumber"),
		CardNumber:      core.FirstValue(values, "cardnumber", "CardNumber"),
		Code:            code,
		Message:         core.FirstValue(values, "respmsg", "RespMsg"),
		Values:          values,
	}
	if amount, convErr := strconv.ParseInt(core.FirstValue(values, "amount", "Amount"), 10, 64); convErr == nil {
		callback.Amount = core.Rial(amount)
	}
	return callback, nil
}

// matchesCallback checks the invoice and amount Sepehr posted to the callback
// against the ones the caller is verifying. Both sides of the comparison are
// only as trustworthy as the callback itself, so this catches a receipt that
// wandered onto the wrong order rather than a forged one; the advice endpoint
// offers nothing better to bind against.
func matchesCallback(req core.VerifyRequest) error {
	if invoice := core.FirstValue(req.Extra, "invoiceid", "InvoiceId", "invoiceID"); invoice != "" &&
		req.OrderID != "" && invoice != req.OrderID {
		return core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("the callback reports invoice " + invoice + ", not " + req.OrderID)
	}
	reported, err := strconv.ParseInt(core.FirstValue(req.Extra, "amount", "Amount"), 10, 64)
	if err != nil {
		return nil
	}
	if _, err := core.SettledAmount(Name, req.Amount, core.Rial(reported)); err != nil {
		return err
	}
	return nil
}

// receiptOf finds the digital receipt among the extra callback values.
func receiptOf(extra map[string]string, fallback string) string {
	if extra != nil {
		if receipt := core.FirstValue(extra, "digitalreceipt", "DigitalReceipt", "digitalReceipt"); receipt != "" {
			return receipt
		}
	}
	return fallback
}

// numeric renders a JSON value that may arrive as a number or as a string.
func numeric(value any) string {
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

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
