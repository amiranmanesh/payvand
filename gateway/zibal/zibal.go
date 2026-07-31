// Package zibal implements the Zibal payment gateway (REST, gateway.zibal.ir).
//
// Credentials: [core.Config.MerchantKey] is the merchant id. Passing the
// literal "zibal" as merchant id puts the terminal in Zibal's own sandbox.
package zibal

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
const Name core.Name = "zibal"

// Provider endpoints.
const (
	defaultBase  = "https://gateway.zibal.ir"
	purchasePath = "/v1/request"
	verifyPath   = "/v1/verify"
	inquiryPath  = "/v1/inquiry"
	startPath    = "/start/"

	// sandboxMerchant is the merchant id that switches Zibal to test mode.
	sandboxMerchant = "zibal"
)

// Zibal result codes.
const (
	resultSuccess         = 100
	resultAlreadyVerified = 201
)

// paidAtLayout is the timestamp format Zibal answers with.
const paidAtLayout = "2006-01-02T15:04:05.000000"

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the Zibal implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers Refund, which Zibal only exposes in its panel.
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	baseURL  string
	merchant string
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a Zibal gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)

	merchant := strings.TrimSpace(cfg.MerchantKey)
	if options.Sandbox {
		merchant = sandboxMerchant
	}
	if merchant == "" {
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("MerchantKey (merchant id) is required")
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
		merchant:    merchant,
	}, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what Zibal offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Inquiry:        true,
		Callback:       true,
		RedirectMethod: http.MethodGet,
		Multiplexing:   true,
		Currencies:     []core.Currency{core.IRR},
	}
}

// Purchase creates a payment and returns the track id plus the start URL.
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

	body := purchaseRequest{
		Merchant:            g.merchant,
		Amount:              req.Amount.Rial(),
		CallbackURL:         req.CallbackURL,
		Description:         description,
		OrderID:             req.OrderID,
		Mobile:              req.Mobile,
		NationalCode:        req.NationalID,
		AllowedCards:        req.AllowedCards,
		LedgerID:            g.settings.ledgerID,
		FeeMode:             g.settings.feeMode,
		CheckMobileWithCard: g.settings.checkMobileWithCard,
		Multiplexing:        g.settings.multiplexing,
	}
	if g.settings.percentMode {
		body.PercentMode = 1
	}

	var out purchaseResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, purchasePath), body, nil, &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if out.Result != resultSuccess || out.TrackID == 0 {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Result)).WithMessage(out.Message)
	}

	token := strconv.FormatInt(out.TrackID, 10)
	return core.PurchaseResponse{
		Token:   token,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     res.Body,
		Redirect: core.Redirect{
			Method: http.MethodGet,
			URL:    transport.JoinURL(g.baseURL, startPath+token),
		},
	}, nil
}

// Verify settles the payment identified by the track id.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	trackID, err := parseTrackID(req.Token)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}

	var out verifyResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, verifyPath), verifyRequest{
		Merchant: g.merchant,
		TrackID:  trackID,
	}, nil, &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	switch out.Result {
	case resultSuccess, resultAlreadyVerified:
	default:
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Result)).WithMessage(out.Message)
	}

	amount, err := core.SettledAmount(Name, req.Amount, core.Rial(out.Amount))
	if err != nil {
		return core.VerifyResponse{}, err
	}
	orderID := out.OrderID
	if orderID == "" {
		orderID = req.OrderID
	}

	return core.VerifyResponse{
		ReferenceNumber: out.RefNumber,
		TransactionID:   req.Token,
		OrderID:         orderID,
		CardNumber:      out.CardNumber,
		Amount:          amount,
		PaidAt:          parseTime(out.PaidAt),
		Raw:             res.Body,
	}, nil
}

// Inquiry asks Zibal for the state of a track id.
func (g *Gateway) Inquiry(ctx context.Context, req core.InquiryRequest) (core.InquiryResponse, error) {
	trackID, err := parseTrackID(req.Token)
	if err != nil {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", err)
	}

	var out inquiryResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, inquiryPath), verifyRequest{
		Merchant: g.merchant,
		TrackID:  trackID,
	}, nil, &out)
	if err != nil {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", err)
	}
	if out.Result != resultSuccess {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(out.Result)).WithMessage(out.Message)
	}

	return core.InquiryResponse{
		Status:          mapStatus(out.Status),
		ReferenceNumber: out.RefNumber,
		CardNumber:      out.CardNumber,
		Amount:          core.Rial(out.Amount),
		PaidAt:          parseTime(out.PaidAt),
		Message:         out.Message,
		Raw:             res.Body,
	}, nil
}

// ParseCallback reads the trackId/success pair Zibal appends to the callback.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	success := core.FirstValue(values, "success")
	status := core.FirstValue(values, "status")
	return core.Callback{
		Gateway:   Name,
		Succeeded: success == "1",
		Token:     core.FirstValue(values, "trackId", "trackid"),
		OrderID:   core.FirstValue(values, "orderId", "orderid"),
		Code:      status,
		Values:    values,
	}, nil
}

// parseTrackID validates the token, which Zibal defines as a numeric track id.
func parseTrackID(token string) (int64, error) {
	if token == "" {
		return 0, core.ErrInvalidRequest
	}
	trackID, err := strconv.ParseInt(token, 10, 64)
	if err != nil {
		return 0, core.ErrInvalidRequest
	}
	return trackID, nil
}

// mapStatus translates a Zibal transaction status into the shared vocabulary.
//
// Zibal separates "paid" from "paid and verified", and the difference decides
// whether the merchant still owes the provider a verification: a transaction
// left at 2 is reversed to the payer. Reporting it as verified is what would
// make a merchant recovering a lost callback stop before the call that keeps
// the money.
func mapStatus(status int) core.Status {
	switch status {
	case 1:
		return core.StatusVerified
	case 2:
		return core.StatusPaid
	case -1:
		return core.StatusPending
	case 3:
		return core.StatusCanceled
	default:
		// -2 is an internal Zibal failure; 4 to 12 are the acquirer's refusals
		// (unknown card, no funds, limits), none of which the payer chose.
		return core.StatusFailed
	}
}

// parseTime converts a Zibal timestamp, tolerating the several shapes it uses.
func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{paidAtLayout, time.RFC3339, "2006-01-02T15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
