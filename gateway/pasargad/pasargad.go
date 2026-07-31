// Package pasargad implements the Bank Pasargad IPG (REST with RSA signed
// bodies, pep.shaparak.ir).
//
// Credentials: [core.Config.MerchantID] is the merchant code,
// [core.Config.TerminalID] is the terminal code and [core.Config.MerchantKey]
// is the RSA private key. Both the PEM form and the .NET <RSAKeyValue> XML
// form Pasargad distributes are accepted.
//
// Every call is signed and the invoice date must be identical in the token and
// the verification request, so persist [core.PurchaseResponse.Extra] under
// [InvoiceDateKey] together with the order.
package pasargad

import (
	"context"
	"crypto/rsa"
	"encoding/json"
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
const Name core.Name = "pasargad"

// InvoiceDateKey is the key under which the invoice date is returned in
// [core.PurchaseResponse.Extra] and expected back in
// [core.VerifyRequest.Extra].
const InvoiceDateKey = "invoice_date"

// Provider endpoints.
const (
	defaultBase = "https://pep.shaparak.ir"

	tokenPath    = "/Api/v1/Payment/GetToken"
	checkPath    = "/Api/v1/Payment/CheckTransactionResult"
	verifyPath   = "/Api/v1/Payment/VerifyPayment"
	refundPath   = "/Api/v1/Payment/RefundPayment"
	gatewayPath  = "/payment.aspx"
	dateTimeForm = "2006/01/02 15:04:05"
)

// defaultAction is the Pasargad operation code of a normal purchase.
const defaultAction = "1003"

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the Pasargad implementation of [core.Gateway].
type Gateway struct {
	cfg        core.Config
	opts       *core.Options
	settings   *config
	client     *transport.Client
	baseURL    string
	privateKey *rsa.PrivateKey
	now        func() time.Time
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a Pasargad gateway. The private key is parsed once here, so a
// malformed key fails at wiring time rather than during a payment.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	switch {
	case strings.TrimSpace(cfg.MerchantID) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).WithMessage("MerchantID (merchant code) is required")
	case strings.TrimSpace(cfg.TerminalID) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).WithMessage("TerminalID (terminal code) is required")
	case strings.TrimSpace(cfg.MerchantKey) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).WithMessage("MerchantKey (RSA private key) is required")
	}

	privateKey, err := cryptox.ParseRSAPrivateKey(cfg.MerchantKey)
	if err != nil {
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).WithMessage(err.Error())
	}

	baseURL := defaultBase
	if options.BaseURL != "" {
		baseURL = options.BaseURL
	}

	return &Gateway{
		cfg:        cfg,
		opts:       options,
		settings:   gwopt.From[config](options, string(Name)),
		client:     transport.New(options),
		baseURL:    baseURL,
		privateKey: privateKey,
		now:        time.Now,
	}, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what Pasargad offers.
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

// signedCall marshals the body, signs it with the terminal key and posts it.
func (g *Gateway) signedCall(ctx context.Context, op, path string, body, out any) (transport.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return transport.Response{}, core.NewError(Name, op, err)
	}
	signature, err := cryptox.SignPKCS1v15SHA1(g.privateKey, payload)
	if err != nil {
		return transport.Response{}, core.NewError(Name, op, err)
	}

	res, err := g.client.Do(ctx, http.MethodPost, transport.JoinURL(g.baseURL, path), payload, map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
		"Sign":         signature,
	})
	if err != nil {
		return res, core.NewError(Name, op, err)
	}
	if out != nil && strings.TrimSpace(res.Body) != "" {
		if err := json.Unmarshal([]byte(res.Body), out); err != nil {
			return res, core.NewError(Name, op, core.ErrUnexpectedResponse).WithMessage(res.Body)
		}
	}
	return res, nil
}

// Purchase creates a payment and returns the Pasargad payment page URL.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	if req.Amount.IsZero() {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be greater than zero")
	}
	if req.OrderID == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("order id is required")
	}

	now := g.now().Format(dateTimeForm)
	action := g.settings.action
	if action == "" {
		action = defaultAction
	}

	body := tokenRequest{
		InvoiceNumber:   req.OrderID,
		InvoiceDate:     now,
		Amount:          req.Amount.Rial(),
		TerminalCode:    g.cfg.TerminalID,
		MerchantCode:    g.cfg.MerchantID,
		RedirectAddress: req.CallbackURL,
		Timestamp:       now,
		Action:          action,
	}
	if g.settings.sendPayerDetails {
		body.Mobile = req.Mobile
		body.Email = req.Email
	}

	var out tokenResponse
	res, err := g.signedCall(ctx, "purchase", tokenPath, body, &out)
	if err != nil {
		return core.PurchaseResponse{}, err
	}
	if !out.IsSuccess || out.Token == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithMessage(firstNonEmpty(out.Message, res.Body))
	}

	return core.PurchaseResponse{
		Token:   out.Token,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Extra:   map[string]string{InvoiceDateKey: now},
		Raw:     res.Body,
		Redirect: core.Redirect{
			Method: http.MethodGet,
			URL:    transport.JoinURL(g.baseURL, gatewayPath),
			Params: map[string]string{"n": out.Token},
		},
	}, nil
}

// Verify checks the transaction and then settles it. Both calls need the
// invoice date used when the payment was created.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	invoiceDate := req.Get(InvoiceDateKey)
	if req.OrderID == "" || invoiceDate == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("order id and the invoice date of the purchase are required")
	}

	var checked checkResponse
	if !g.settings.skipTransactionCheck {
		reference := firstNonEmpty(req.ReferenceNumber, req.Get("tref"))
		if reference == "" {
			return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
				WithMessage("the transaction reference (tref) of the callback is required")
		}
		if _, err := g.signedCall(ctx, "verify", checkPath, checkRequest{
			TransactionReferenceID: reference,
			InvoiceNumber:          req.OrderID,
			InvoiceDate:            invoiceDate,
			MerchantCode:           g.cfg.MerchantID,
			TerminalCode:           g.cfg.TerminalID,
		}, &checked); err != nil {
			return core.VerifyResponse{}, err
		}
		if !checked.IsSuccess {
			return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
				WithMessage(checked.Message)
		}
		if checked.Amount > 0 && req.Amount.Rial() > 0 && checked.Amount != req.Amount.Rial() {
			return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrAmountMismatch).
				WithMessage("the bank reported " + strconv.FormatInt(checked.Amount, 10) + " Rial")
		}
	}

	var settled settlementResponse
	res, err := g.signedCall(ctx, "verify", verifyPath, settlementRequest{
		InvoiceNumber: req.OrderID,
		InvoiceDate:   invoiceDate,
		Amount:        req.Amount.Rial(),
		TerminalCode:  g.cfg.TerminalID,
		MerchantCode:  g.cfg.MerchantID,
		Timestamp:     g.now().Format(dateTimeForm),
	}, &settled)
	if err != nil {
		return core.VerifyResponse{}, err
	}
	if !settled.IsSuccess {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithMessage(firstNonEmpty(settled.Message, res.Body))
	}

	reference := settled.ShaparakRefNumber
	if reference == "" && checked.ReferenceNumber > 0 {
		reference = strconv.FormatInt(checked.ReferenceNumber, 10)
	}

	return core.VerifyResponse{
		ReferenceNumber: reference,
		TransactionID:   strconv.FormatInt(checked.TraceNumber, 10),
		OrderID:         req.OrderID,
		CardNumber:      firstNonEmpty(settled.MaskedCardNumber, checked.MaskedCardNumber, req.CardNumber),
		CardHash:        firstNonEmpty(settled.HashedCardNumber, checked.HashedCardNumber),
		Amount:          req.Amount,
		Raw:             res.Body,
	}, nil
}

// Refund returns a settled payment to the payer.
func (g *Gateway) Refund(ctx context.Context, req core.RefundRequest) (core.RefundResponse, error) {
	invoiceDate := req.Get(InvoiceDateKey)
	if req.OrderID == "" || invoiceDate == "" {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrInvalidRequest).
			WithMessage("order id and the invoice date of the purchase are required")
	}

	var out settlementResponse
	res, err := g.signedCall(ctx, "refund", refundPath, settlementRequest{
		InvoiceNumber: req.OrderID,
		InvoiceDate:   invoiceDate,
		Amount:        req.Amount.Rial(),
		TerminalCode:  g.cfg.TerminalID,
		MerchantCode:  g.cfg.MerchantID,
		Timestamp:     g.now().Format(dateTimeForm),
	}, &out)
	if err != nil {
		return core.RefundResponse{}, err
	}
	if !out.IsSuccess {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrPaymentFailed).
			WithMessage(firstNonEmpty(out.Message, res.Body))
	}

	return core.RefundResponse{RefundID: req.OrderID, Amount: req.Amount, Raw: res.Body}, nil
}

// Inquiry reads the state of a transaction through CheckTransactionResult.
func (g *Gateway) Inquiry(ctx context.Context, req core.InquiryRequest) (core.InquiryResponse, error) {
	invoiceDate := req.Get(InvoiceDateKey)
	if req.OrderID == "" || invoiceDate == "" {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", core.ErrInvalidRequest).
			WithMessage("order id and the invoice date of the purchase are required")
	}

	var out checkResponse
	res, err := g.signedCall(ctx, "inquiry", checkPath, checkRequest{
		TransactionReferenceID: firstNonEmpty(req.ReferenceNumber, req.Get("tref")),
		InvoiceNumber:          req.OrderID,
		InvoiceDate:            invoiceDate,
		MerchantCode:           g.cfg.MerchantID,
		TerminalCode:           g.cfg.TerminalID,
	}, &out)
	if err != nil {
		return core.InquiryResponse{}, err
	}

	status := core.StatusFailed
	if out.IsSuccess {
		status = core.StatusPaid
	}

	return core.InquiryResponse{
		Status:          status,
		ReferenceNumber: strconv.FormatInt(out.ReferenceNumber, 10),
		CardNumber:      out.MaskedCardNumber,
		Amount:          core.Rial(out.Amount),
		Message:         out.Message,
		Raw:             res.Body,
	}, nil
}

// ParseCallback reads the iN/iD/tref trio Pasargad returns the payer with.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	reference := core.FirstValue(values, "tref", "TRef", "transactionReferenceID")
	if invoiceDate := core.FirstValue(values, "iD", "id"); invoiceDate != "" {
		values[InvoiceDateKey] = invoiceDate
	}
	return core.Callback{
		Gateway:         Name,
		Succeeded:       reference != "",
		Token:           reference,
		OrderID:         core.FirstValue(values, "iN", "in"),
		ReferenceNumber: reference,
		Values:          values,
	}, nil
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
