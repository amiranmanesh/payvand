// Package saman implements the Saman Bank (SEP) IPG (REST, sep.shaparak.ir).
//
// Credentials: [core.Config.TerminalID] is the terminal number.
package saman

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
const Name core.Name = "saman"

// Provider endpoints.
const (
	defaultBase = "https://sep.shaparak.ir"

	tokenPath   = "/onlinepg/onlinepg"
	gatewayPath = "/OnlinePG/OnlinePG"
	verifyPath  = "/verifyTxnRandomSessionkey/ipg/VerifyTransaction"
	reversePath = "/verifyTxnRandomSessionkey/ipg/ReverseTransaction"
)

// statusOK is the success flag of the token endpoint.
const statusOK = 1

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the Saman implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers Inquiry, which SEP does not expose to merchants.
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	baseURL  string
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a Saman gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	if strings.TrimSpace(cfg.TerminalID) == "" {
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("TerminalID is required")
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

// Capabilities reports what Saman offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Refund:         true,
		Callback:       true,
		RedirectMethod: http.MethodGet,
		Currencies:     []core.Currency{core.IRR},
	}
}

// Purchase creates a payment token and returns the SEP payment page URL.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	if req.Amount.IsZero() {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be greater than zero")
	}
	if req.OrderID == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("order id is required")
	}
	if req.CallbackURL == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("callback url is required")
	}

	body := tokenRequest{
		Action:      "token",
		TerminalID:  g.cfg.TerminalID,
		Amount:      req.Amount.Rial(),
		ResNum:      req.OrderID,
		RedirectURL: req.CallbackURL,
	}
	if g.settings.sendMobile {
		body.CellNumber = req.Mobile
	}

	var out tokenResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, tokenPath), body, nil, &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if out.Status != statusOK || out.Token == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.ErrorCode)).WithMessage(out.ErrorDesc)
	}

	params := map[string]string{"token": out.Token}
	if g.settings.getMethod {
		params["GetMethod"] = "true"
	}

	return core.PurchaseResponse{
		Token:   out.Token,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     res.Body,
		Redirect: core.Redirect{
			Method: http.MethodGet,
			URL:    transport.JoinURL(g.baseURL, gatewayPath),
			Params: params,
		},
	}, nil
}

// Verify settles the payment identified by the callback reference number.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	refNum := firstNonEmpty(req.ReferenceNumber, req.Get("RefNum"), req.Token)
	if refNum == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("RefNum is required")
	}

	var out transactionResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, verifyPath),
		transactionRequest{RefNum: refNum, TerminalNumber: g.cfg.TerminalID}, nil, &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	if !out.Success {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.ResultCode)).
			WithMessage(firstNonEmpty(out.ResultDescription, Message(out.ResultCode)))
	}

	// SEP reports the amount the card was actually charged next to the one the
	// transaction was created for; the charged one is what the merchant is owed.
	reported := out.TransactionDetail.AffectiveAmount
	if reported <= 0 {
		reported = out.TransactionDetail.OriginalAmount
	}
	amount, err := core.SettledAmount(Name, req.Amount, core.Rial(reported))
	if err != nil {
		return core.VerifyResponse{}, err
	}

	return core.VerifyResponse{
		ReferenceNumber: firstNonEmpty(out.TransactionDetail.RRN, refNum),
		TransactionID:   refNum,
		OrderID:         req.OrderID,
		CardNumber:      firstNonEmpty(out.TransactionDetail.MaskedPan, req.CardNumber),
		CardHash:        out.TransactionDetail.HashedPan,
		Amount:          amount,
		Raw:             res.Body,
	}, nil
}

// Refund reverses a transaction that has not been settled yet.
func (g *Gateway) Refund(ctx context.Context, req core.RefundRequest) (core.RefundResponse, error) {
	refNum := firstNonEmpty(req.ReferenceNumber, req.TransactionID, req.Token)
	if refNum == "" {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrInvalidRequest).
			WithMessage("RefNum is required")
	}

	var out transactionResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, reversePath),
		transactionRequest{RefNum: refNum, TerminalNumber: g.cfg.TerminalID}, nil, &out)
	if err != nil {
		return core.RefundResponse{}, core.NewError(Name, "refund", err)
	}
	if !out.Success {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.ResultCode)).
			WithMessage(firstNonEmpty(out.ResultDescription, Message(out.ResultCode)))
	}

	return core.RefundResponse{RefundID: refNum, Amount: req.Amount, Raw: res.Body}, nil
}

// ParseCallback reads the form SEP posts to the redirect URL.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	status := core.FirstValue(values, "Status", "status")
	state := core.FirstValue(values, "State", "state")

	callback := core.Callback{
		Gateway:         Name,
		Succeeded:       strings.EqualFold(state, "OK") || status == "2",
		Token:           core.FirstValue(values, "Token", "token"),
		OrderID:         core.FirstValue(values, "ResNum", "resNum"),
		ReferenceNumber: core.FirstValue(values, "RefNum", "refNum"),
		TraceNumber:     core.FirstValue(values, "TraceNo", "traceNo"),
		CardNumber:      core.FirstValue(values, "SecurePan", "securePan"),
		Code:            status,
		Message:         state,
		Values:          values,
	}
	if amount, convErr := strconv.ParseInt(core.FirstValue(values, "Amount", "amount"), 10, 64); convErr == nil {
		callback.Amount = core.Rial(amount)
	}
	return callback, nil
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

// Message returns the English meaning of a SEP verification result code.
func Message(code int) string {
	switch code {
	case 0:
		return "approved"
	case -1:
		return "an error occurred while processing the request"
	case -2:
		return "the terminal is not allowed to perform this transaction"
	case -3:
		return "the reference number is invalid"
	case -4:
		return "the transaction was already reversed"
	case -5:
		return "the transaction was already verified"
	case -6:
		return "the transaction was not found"
	default:
		return "saman error " + strconv.Itoa(code)
	}
}
