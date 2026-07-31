// Package vandar implements the Vandar IPG (REST, ipg.vandar.io).
//
// Credentials: [core.Config.MerchantKey] is the API key of the IPG, and
// [core.Config.MerchantID] is the business name used by the refund endpoint.
package vandar

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Name is the registry name of this gateway.
const Name core.Name = "vandar"

// Provider endpoints.
const (
	defaultIPGBase      = "https://ipg.vandar.io"
	defaultBusinessBase = "https://api.vandar.io"

	sendPath        = "/api/v4/send"
	transactionPath = "/api/v4/transaction"
	verifyPath      = "/api/v4/verify"
	redirectPath    = "/v4/"
)

// statusOK is the success flag of every Vandar response.
const statusOK = 1

// minAmount is the smallest payment Vandar accepts, in Rial.
const minAmount = 1000

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the Vandar implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers Inquiry, which Vandar does not expose separately;
	// the transaction endpoint is used inside Verify instead.
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client

	ipgBase      string
	businessBase string
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a Vandar gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	if strings.TrimSpace(cfg.MerchantKey) == "" {
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("MerchantKey (api key) is required")
	}

	ipgBase, businessBase := defaultIPGBase, defaultBusinessBase
	if options.BaseURL != "" {
		ipgBase, businessBase = options.BaseURL, options.BaseURL
	}

	return &Gateway{
		Unsupported:  core.Unsupported{GatewayName: Name},
		cfg:          cfg,
		opts:         options,
		settings:     gwopt.From[config](options, string(Name)),
		client:       transport.New(options),
		ipgBase:      ipgBase,
		businessBase: businessBase,
	}, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what Vandar offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Refund:         true,
		Callback:       true,
		RedirectMethod: http.MethodGet,
		Currencies:     []core.Currency{core.IRR},
	}
}

// Purchase creates a payment and returns the token plus the redirect URL.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	if req.Amount.Rial() < minAmount {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be at least 1000 Rial")
	}
	if req.CallbackURL == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("callback url is required")
	}

	description := req.Description
	if description == "" {
		description = g.settings.description
	}
	factorNumber := ""
	if g.settings.sendOrderAsFactor {
		factorNumber = req.OrderID
	}

	var out sendResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.ipgBase, sendPath), sendRequest{
		APIKey:          g.cfg.MerchantKey,
		Amount:          req.Amount.Rial(),
		CallbackURL:     req.CallbackURL,
		MobileNumber:    req.Mobile,
		FactorNumber:    factorNumber,
		Description:     description,
		NationalCode:    req.NationalID,
		ValidCardNumber: req.AllowedCards,
		Port:            g.settings.port,
		Comment:         g.settings.comment,
	}, nil, &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if out.Status != statusOK || out.Token == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Status)).WithMessage(firstError(out.Errors, "vandar rejected the payment request"))
	}

	return core.PurchaseResponse{
		Token:   out.Token,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     res.Body,
		Redirect: core.Redirect{
			Method: http.MethodGet,
			URL:    transport.JoinURL(g.ipgBase, redirectPath+out.Token),
		},
	}, nil
}

// Verify runs the two step Vandar settlement: read the transaction, then
// verify it. Both calls are required; skipping the second one lets Vandar roll
// the payment back.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	if req.Token == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("token is required")
	}
	body := tokenRequest{APIKey: g.cfg.MerchantKey, Token: req.Token}

	var transaction transactionResponse
	if _, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.ipgBase, transactionPath), body, nil, &transaction); err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	if transaction.Status != statusOK {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(transaction.Status)).
			WithMessage(firstError(transaction.Errors, transaction.Message))
	}

	var verified verifyResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.ipgBase, verifyPath), body, nil, &verified)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	if verified.Status != statusOK {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(verified.Status)).
			WithMessage(firstError(verified.Errors, verified.Message))
	}

	amount, err := core.SettledAmount(Name, req.Amount, core.Rial(rials(verified.Amount)))
	if err != nil {
		return core.VerifyResponse{}, err
	}
	var fee int64
	if verified.Wage != nil {
		fee = *verified.Wage
	}

	return core.VerifyResponse{
		ReferenceNumber: transaction.RefNumber,
		TransactionID:   strconv.FormatInt(verified.TransID, 10),
		OrderID:         req.OrderID,
		CardNumber:      deref(verified.CardNumber),
		Amount:          amount,
		Fee:             fee,
		PaidAt:          parseTime(deref(verified.PaymentDate)),
		Raw:             res.Body,
	}, nil
}

// Refund reverses a settled transaction through the Vandar business API.
func (g *Gateway) Refund(ctx context.Context, req core.RefundRequest) (core.RefundResponse, error) {
	transactionID := req.TransactionID
	if transactionID == "" {
		transactionID = req.ReferenceNumber
	}
	if transactionID == "" {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrInvalidRequest).
			WithMessage("transaction id is required")
	}
	if g.cfg.MerchantID == "" {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrInvalidConfig).
			WithMessage("MerchantID (business name) is required for refunds")
	}

	headers := map[string]string{}
	if g.settings.accessToken != "" {
		headers["Authorization"] = "Bearer " + g.settings.accessToken
	}
	endpoint := transport.JoinURL(g.businessBase,
		"/v3/business/"+g.cfg.MerchantID+"/transaction/"+transactionID+"/refund")

	var out refundResponse
	res, err := g.client.JSON(ctx, http.MethodPost, endpoint, refundRequest{
		APIKey:        g.cfg.MerchantKey,
		TransactionID: transactionID,
		Amount:        req.Amount.Rial(),
		Comment:       req.Reason,
	}, headers, &out)
	if err != nil {
		return core.RefundResponse{}, core.NewError(Name, "refund", err)
	}
	if out.Status != statusOK {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Status)).WithMessage(firstError(out.Errors, out.Message))
	}

	return core.RefundResponse{RefundID: transactionID, Amount: req.Amount, Raw: res.Body}, nil
}

// ParseCallback reads the token/payment_status pair Vandar returns with.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	status := core.FirstValue(values, "payment_status", "status")
	return core.Callback{
		Gateway:   Name,
		Succeeded: strings.EqualFold(status, "OK"),
		Token:     core.FirstValue(values, "token"),
		Code:      status,
		Values:    values,
	}, nil
}

// rials reads the amount Vandar reports, which arrives as a decimal string
// ("1000.00") and would be rejected outright by an integer parse. The fraction
// is dropped rather than rounded: Vandar settles whole Rial and the digits
// after the point are always zero.
func rials(value string) int64 {
	if point := strings.IndexByte(value, '.'); point >= 0 {
		value = value[:point]
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

// parseTime converts a Vandar payment date ("2006-01-02 15:04:05").
func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse("2006-01-02 15:04:05", value); err == nil {
		return parsed
	}
	return time.Time{}
}
