// Package mellat implements the Behpardakht Mellat IPG (SOAP,
// bpm.shaparak.ir).
//
// Credentials: [core.Config.TerminalID], [core.Config.Username] and
// [core.Config.Password] are the terminal id, user name and password issued by
// Behpardakht.
package mellat

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
	"github.com/amiranmanesh/payvand/internal/soap"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Name is the registry name of this gateway.
const Name core.Name = "mellat"

// Provider endpoints.
const (
	defaultBase  = "https://bpm.shaparak.ir"
	servicePath  = "/pgwchannel/services/pgw"
	startPayPath = "/pgwchannel/startpay.mellat"

	namespace = "http://interfaces.core.sw.bps.com/"
)

// Result codes that mean an earlier attempt already did the work.
const (
	// resultOK is the success code returned by every Mellat operation.
	resultOK = "0"
	// resultAlreadyVerified means a previous bpVerifyRequest went through.
	resultAlreadyVerified = "43"
	// resultAlreadySettled means a previous bpSettleRequest went through.
	resultAlreadySettled = "45"
	// resultNotSettled means the transaction is verified but not settled yet.
	resultNotSettled = "46"
)

// defaultPayerID is sent when the merchant does not use payer identifiers.
const defaultPayerID = "0"

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the Behpardakht Mellat implementation of [core.Gateway].
type Gateway struct {
	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	baseURL  string
	now      func() time.Time
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a Mellat gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	switch {
	case strings.TrimSpace(cfg.TerminalID) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).WithMessage("TerminalID is required")
	case strings.TrimSpace(cfg.Username) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).WithMessage("Username is required")
	case strings.TrimSpace(cfg.Password) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).WithMessage("Password is required")
	}

	baseURL := defaultBase
	if options.BaseURL != "" {
		baseURL = options.BaseURL
	}

	return &Gateway{
		cfg:      cfg,
		opts:     options,
		settings: gwopt.From[config](options, string(Name)),
		client:   transport.New(options),
		baseURL:  baseURL,
		now:      time.Now,
	}, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what Mellat offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Refund:         true,
		Inquiry:        true,
		Callback:       true,
		RedirectMethod: http.MethodPost,
		Currencies:     []core.Currency{core.IRR},
	}
}

// Purchase creates a payment and returns the form the payer is posted with.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	if req.Amount.IsZero() {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be greater than zero")
	}
	if req.OrderID == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("order id is required")
	}

	payerID := g.settings.payerID
	if payerID == "" {
		payerID = defaultPayerID
	}
	now := g.now()

	var out soap.StringResult
	res, err := soap.Do(ctx, g.client, soap.Call{
		Endpoint:  transport.JoinURL(g.baseURL, servicePath),
		Action:    "bpPayRequest",
		Namespace: namespace,
		Payload: payRequest{
			TerminalID:     g.cfg.TerminalID,
			UserName:       g.cfg.Username,
			UserPassword:   g.cfg.Password,
			OrderID:        req.OrderID,
			Amount:         req.Amount.Rial(),
			LocalDate:      now.Format("20060102"),
			LocalTime:      now.Format("150405"),
			AdditionalData: g.settings.additionalData,
			CallBackURL:    req.CallbackURL,
			PayerID:        payerID,
		},
	}, &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}

	code, refID := split(out.Return)
	if code != resultOK || refID == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithCode(code).WithMessage(Message(code))
	}

	return core.PurchaseResponse{
		Token:   refID,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     res.Body,
		Redirect: core.Redirect{
			Method: http.MethodPost,
			URL:    transport.JoinURL(g.baseURL, startPayPath),
			Params: map[string]string{"RefId": refID},
		},
	}, nil
}

// Verify settles the payment: bpVerifyRequest confirms it and bpSettleRequest
// releases the money to the merchant. Both are required, unless the merchant
// settles separately (see [WithoutSettle]).
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	saleReferenceID := req.ReferenceNumber
	if req.OrderID == "" || saleReferenceID == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("order id and sale reference id are required")
	}
	saleOrderID := firstNonEmpty(req.Get("SaleOrderId"), req.Get("saleOrderId"), req.OrderID)

	var verified soap.StringResult
	res, err := soap.Do(ctx, g.client, soap.Call{
		Endpoint:  transport.JoinURL(g.baseURL, servicePath),
		Action:    "bpVerifyRequest",
		Namespace: namespace,
		Payload: verifyRequest{
			TerminalID:      g.cfg.TerminalID,
			UserName:        g.cfg.Username,
			UserPassword:    g.cfg.Password,
			OrderID:         req.OrderID,
			SaleOrderID:     saleOrderID,
			SaleReferenceID: saleReferenceID,
		},
	}, &verified)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	verifyCode, _ := split(verified.Return)
	switch verifyCode {
	case resultOK:
	case resultAlreadyVerified:
		// An earlier attempt verified the transaction and lost the answer on
		// the way back. Settling is the step that keeps the money — Mellat
		// reverses a verified but unsettled transaction — so the flow carries
		// on to it instead of stopping one call short.
	default:
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithCode(verifyCode).WithMessage(Message(verifyCode))
	}

	if !g.settings.skipSettle {
		var settled soap.StringResult
		if _, err := soap.Do(ctx, g.client, soap.Call{
			Endpoint:  transport.JoinURL(g.baseURL, servicePath),
			Action:    "bpSettleRequest",
			Namespace: namespace,
			Payload: settleRequest{
				TerminalID:      g.cfg.TerminalID,
				UserName:        g.cfg.Username,
				UserPassword:    g.cfg.Password,
				OrderID:         req.OrderID,
				SaleOrderID:     saleOrderID,
				SaleReferenceID: saleReferenceID,
			},
		}, &settled); err != nil {
			return core.VerifyResponse{}, core.NewError(Name, "verify", err)
		}
		settleCode, _ := split(settled.Return)
		switch settleCode {
		case resultOK:
		case resultAlreadySettled:
			// Verified and settled before this call: the money is safe, but
			// this is not the settlement of a fresh payment and a caller that
			// ships on every successful verification must not ship twice.
			return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrAlreadyVerified).
				WithCode(settleCode).WithMessage(Message(settleCode))
		default:
			return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
				WithCode(settleCode).WithMessage(Message(settleCode))
		}
	} else if verifyCode == resultAlreadyVerified {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrAlreadyVerified).
			WithCode(verifyCode).WithMessage(Message(verifyCode))
	}

	return core.VerifyResponse{
		ReferenceNumber: saleReferenceID,
		TransactionID:   req.Token,
		OrderID:         req.OrderID,
		CardNumber:      req.CardNumber,
		Amount:          req.Amount,
		Raw:             res.Body,
	}, nil
}

// Refund reverses a transaction with bpReversalRequest.
func (g *Gateway) Refund(ctx context.Context, req core.RefundRequest) (core.RefundResponse, error) {
	if req.OrderID == "" || req.ReferenceNumber == "" {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrInvalidRequest).
			WithMessage("order id and sale reference id are required")
	}
	saleOrderID := firstNonEmpty(req.Get("SaleOrderId"), req.OrderID)

	var out soap.StringResult
	res, err := soap.Do(ctx, g.client.NoRetry(), soap.Call{
		Endpoint:  transport.JoinURL(g.baseURL, servicePath),
		Action:    "bpReversalRequest",
		Namespace: namespace,
		Payload: reversalRequest{
			TerminalID:      g.cfg.TerminalID,
			UserName:        g.cfg.Username,
			UserPassword:    g.cfg.Password,
			OrderID:         req.OrderID,
			SaleOrderID:     saleOrderID,
			SaleReferenceID: req.ReferenceNumber,
		},
	}, &out)
	if err != nil {
		return core.RefundResponse{}, core.NewError(Name, "refund", err)
	}
	if code, _ := split(out.Return); code != resultOK {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrPaymentFailed).
			WithCode(code).WithMessage(Message(code))
	}

	return core.RefundResponse{RefundID: req.ReferenceNumber, Amount: req.Amount, Raw: res.Body}, nil
}

// Inquiry reads the state of a transaction with bpInquiryRequest.
func (g *Gateway) Inquiry(ctx context.Context, req core.InquiryRequest) (core.InquiryResponse, error) {
	if req.OrderID == "" || req.ReferenceNumber == "" {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", core.ErrInvalidRequest).
			WithMessage("order id and sale reference id are required")
	}
	saleOrderID := firstNonEmpty(req.Get("SaleOrderId"), req.OrderID)

	var out soap.StringResult
	res, err := soap.Do(ctx, g.client, soap.Call{
		Endpoint:  transport.JoinURL(g.baseURL, servicePath),
		Action:    "bpInquiryRequest",
		Namespace: namespace,
		Payload: inquiryRequest{
			TerminalID:      g.cfg.TerminalID,
			UserName:        g.cfg.Username,
			UserPassword:    g.cfg.Password,
			OrderID:         req.OrderID,
			SaleOrderID:     saleOrderID,
			SaleReferenceID: req.ReferenceNumber,
		},
	}, &out)
	if err != nil {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", err)
	}

	// Inquiry is the recovery path for a lost callback, so the codes that mean
	// "this already happened" are answers about a live transaction, not
	// failures. Reporting them as failed is what would have a merchant cancel
	// an order the payer paid for.
	code, _ := split(out.Return)
	var status core.Status
	switch code {
	case resultOK, resultAlreadySettled:
		status = core.StatusVerified
	case resultAlreadyVerified, resultNotSettled:
		status = core.StatusPaid
	default:
		status = core.StatusFailed
	}

	return core.InquiryResponse{
		Status:          status,
		ReferenceNumber: req.ReferenceNumber,
		Amount:          req.Amount,
		Message:         Message(code),
		Raw:             res.Body,
	}, nil
}

// ParseCallback reads the form Mellat posts to the callback URL.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	code := core.FirstValue(values, "ResCode", "rescode")
	return core.Callback{
		Gateway:         Name,
		Succeeded:       code == resultOK,
		Token:           core.FirstValue(values, "RefId", "refId"),
		OrderID:         core.FirstValue(values, "saleOrderId", "SaleOrderId"),
		ReferenceNumber: core.FirstValue(values, "SaleReferenceId", "saleReferenceId"),
		CardNumber:      core.FirstValue(values, "CardHolderPan", "cardHolderPan"),
		Code:            code,
		Message:         Message(code),
		Values:          values,
	}, nil
}

// split cuts a Mellat answer of the form "code,payload" in two.
func split(value string) (code, payload string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", ""
	}
	parts := strings.SplitN(trimmed, ",", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
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

// Message returns the English meaning of a Mellat result code.
func Message(code string) string {
	messages := map[string]string{
		"0":   "approved",
		"11":  "the card number is invalid",
		"12":  "insufficient funds",
		"13":  "the PIN is wrong",
		"14":  "too many wrong PIN attempts",
		"15":  "the card is invalid",
		"16":  "the withdrawal limit is exceeded",
		"17":  "the payer abandoned the payment",
		"18":  "the card expiry date is in the past",
		"19":  "the amount exceeds the withdrawal limit",
		"111": "the card issuer is unknown",
		"112": "the card issuer had an internal error",
		"113": "the card issuer did not answer",
		"114": "the card holder may not perform this transaction",
		"21":  "the merchant is invalid",
		"23":  "a security error occurred",
		"24":  "the merchant credentials are invalid",
		"25":  "the amount is invalid",
		"31":  "invalid response",
		"32":  "the request data format is invalid",
		"33":  "the account number is invalid",
		"34":  "system error",
		"35":  "the date is invalid",
		"41":  "the request number is duplicated",
		"42":  "no sale transaction was found",
		"43":  "the transaction was already verified",
		"44":  "no verify request was found",
		"45":  "the transaction was already settled",
		"46":  "the transaction is not settled",
		"47":  "no settle transaction was found",
		"48":  "the transaction was already reversed",
		"49":  "no refund transaction was found",
		"412": "the bill id is wrong",
		"413": "the payment id is wrong",
		"414": "the bill issuer is invalid",
		"415": "the session expired",
		"416": "an error occurred while writing the transaction",
		"417": "the payer id is invalid",
		"418": "an error occurred while defining the customer",
		"419": "the number of data entry attempts is exceeded",
		"421": "the ip address is invalid",
		"51":  "duplicate transaction",
		"54":  "the reference transaction is missing",
		"55":  "the transaction is invalid",
		"61":  "an error occurred while settling the transaction",
	}
	if message, ok := messages[code]; ok {
		return message
	}
	return "mellat error " + code
}
