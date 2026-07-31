// Package snapppay implements the SnappPay online instalment gateway (REST,
// api.snapppay.ir).
//
// SnappPay is a buy-now-pay-later provider rather than an acquirer: it lends
// the amount to an eligible Snapp user and settles it to the merchant, so the
// flow has two extra steps compared with a card gateway. Eligibility can be
// checked before the basket is shown with [Gateway.Eligible], and a completed
// payment has to be verified *and* settled — [Gateway.Verify] does both unless
// [WithAutoSettle] turns the second call off.
//
// Credentials: [core.Config.Username] and [core.Config.Password] are the
// merchant user of the OAuth password grant, while [core.Config.MerchantID]
// and [core.Config.MerchantKey] are the OAuth client id and client secret.
package snapppay

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
	"github.com/amiranmanesh/payvand/internal/tokenauth"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Name is the registry name of this gateway.
const Name core.Name = "snapppay"

// Provider endpoints.
const (
	defaultBaseURL = "https://api.snapppay.ir"

	oauthPath    = "/api/online/v1/oauth/token"
	eligiblePath = "/api/online/offer/v1/eligible"
	tokenPath    = "/api/online/payment/v1/token"
	verifyPath   = "/api/online/payment/v1/verify"
	settlePath   = "/api/online/payment/v1/settle"
	revertPath   = "/api/online/payment/v1/revert"
	statusPath   = "/api/online/payment/v1/status"
	cancelPath   = "/api/online/payment/v1/cancel"
	updatePath   = "/api/online/payment/v1/update"
)

// Defaults applied when the corresponding option is not set.
const (
	defaultPaymentMethod = "INSTALLMENT"
	defaultScope         = "online-merchant"
	defaultCategory      = "OTHER"
)

// Metadata keys read from [core.PurchaseRequest.Metadata]. Both are amounts in
// the currency of the request and are optional.
const (
	// DiscountKey is the discount already applied to the basket.
	DiscountKey = "discountAmount"
	// ExternalSourceKey is the part of the order paid by another means, e.g.
	// a wallet or a gift card.
	ExternalSourceKey = "externalSourceAmount"
)

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the SnappPay implementation of [core.Gateway].
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

// New builds a SnappPay gateway.
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

// Capabilities reports what SnappPay offers.
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

// Eligibility is the answer of [Gateway.Eligible]: whether this amount can be
// paid in instalments, and how SnappPay wants it advertised.
type Eligibility struct {
	// Eligible reports whether the instalment option may be offered.
	Eligible bool
	// Title is the label SnappPay wants next to the payment option.
	Title string
	// Description is the longer text SnappPay wants shown.
	Description string
	// MinAmount is the smallest amount that can be paid in instalments.
	MinAmount core.Money
	// MaxAmount is the largest amount that can be paid in instalments.
	MaxAmount core.Money
	// Plans are the repayment plans SnappPay would offer for this amount.
	Plans []Plan
	// Raw is the untouched provider response body.
	Raw string
}

// Plan is one repayment plan of an [Eligibility].
type Plan struct {
	// Count is the number of instalments.
	Count int
	// Amount is the value of a single instalment.
	Amount core.Money
	// Wage is the fee the payer pays for this plan.
	Wage core.Money
}

// Eligible asks SnappPay whether an amount may be paid in instalments. Call it
// before showing the payment option: the answer depends on the merchant's
// contract and on the amount, not on the payer.
func (g *Gateway) Eligible(ctx context.Context, amount core.Money) (Eligibility, error) {
	endpoint := transport.JoinURL(g.baseURL, eligiblePath) +
		"?amount=" + strconv.FormatInt(amount.Rial(), 10)

	var out eligibleResponse
	res, err := g.auth.JSON(ctx, http.MethodGet, endpoint, nil, nil, &out)
	if err != nil {
		return Eligibility{}, core.NewError(Name, "eligible", err)
	}
	if !out.Successful {
		return Eligibility{}, failure("eligible", out.envelope, "snapppay rejected the eligibility check")
	}

	plans := make([]Plan, 0, len(out.Response.Plans))
	for _, plan := range out.Response.Plans {
		plans = append(plans, Plan{
			Count:  plan.Count,
			Amount: core.Rial(plan.Amount),
			Wage:   core.Rial(plan.Wage),
		})
	}

	return Eligibility{
		Eligible:    out.Response.Eligible,
		Title:       out.Response.Title,
		Description: out.Response.Description,
		MinAmount:   core.Rial(out.Response.MinEligibleAmount),
		MaxAmount:   core.Rial(out.Response.MaxEligibleAmount),
		Plans:       plans,
		Raw:         res.Body,
	}, nil
}

// Purchase creates an instalment payment and returns the SnappPay page the
// payer is sent to.
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
			WithMessage("order id is required; SnappPay uses it as transactionId")
	}

	body := tokenRequest{
		Amount:               req.Amount.Rial(),
		Mobile:               internationalMobile(req.Mobile),
		PaymentMethodTypeDto: g.paymentMethod(),
		TransactionID:        req.OrderID,
		ReturnURL:            req.CallbackURL,
		DiscountAmount:       metadataRial(req, DiscountKey),
		ExternalSourceAmount: metadataRial(req, ExternalSourceKey),
		CartList:             g.carts(req),
	}

	var out tokenResponse
	res, err := g.auth.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, tokenPath), body, nil, &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if !out.Successful || out.Response.PaymentToken == "" {
		return core.PurchaseResponse{}, failure("purchase", out.envelope, "snapppay rejected the payment request")
	}

	return core.PurchaseResponse{
		Token:    out.Response.PaymentToken,
		OrderID:  req.OrderID,
		Amount:   req.Amount,
		Raw:      res.Body,
		Redirect: core.Redirect{Method: http.MethodGet, URL: out.Response.PaymentPageURL},
	}, nil
}

// Verify confirms a completed payment and, unless [WithAutoSettle] disabled
// it, settles it in the same call. Both steps are required: SnappPay reverts a
// payment that is verified but never settled.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	if req.Token == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("token is required")
	}

	verified, res, err := g.call(ctx, "verify", verifyPath, req.Token)
	if err != nil {
		return core.VerifyResponse{}, err
	}
	// Settling is what makes the payment final and releases the goods, so the
	// amount is reconciled before that step rather than after it.
	amount, err := core.SettledAmount(Name, req.Amount, core.Rial(verified.Amount))
	if err != nil {
		return core.VerifyResponse{}, err
	}
	if !g.settings.manualSettle {
		settled, settleRes, settleErr := g.call(ctx, "verify", settlePath, req.Token)
		if settleErr != nil {
			return core.VerifyResponse{}, settleErr
		}
		// The settle answer is thinner than the verify one, so it adds to what
		// verification reported instead of replacing it.
		res = settleRes
		if settled.ReferenceNumber != "" {
			verified.ReferenceNumber = settled.ReferenceNumber
		}
		if settled.TransactionID != "" {
			verified.TransactionID = settled.TransactionID
		}
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

// Settle releases a verified payment to the merchant. Call it only when
// [WithAutoSettle] turned the automatic settlement off.
func (g *Gateway) Settle(ctx context.Context, paymentToken string) error {
	_, _, err := g.call(ctx, "settle", settlePath, paymentToken)
	return err
}

// Cancel drops a payment the payer never completed, freeing the credit
// SnappPay reserved for it.
func (g *Gateway) Cancel(ctx context.Context, paymentToken string) error {
	_, _, err := g.call(ctx, "cancel", cancelPath, paymentToken)
	return err
}

// Refund reverts a settled payment. SnappPay reverts the whole amount; a
// partial amount in the request is ignored by the provider.
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
	if !out.Successful {
		return core.InquiryResponse{}, failure("inquiry", out.envelope, "snapppay rejected the status request")
	}

	return core.InquiryResponse{
		Status:  mapStatus(out.Response.Status),
		Amount:  core.Rial(out.Response.Amount),
		Message: out.Response.Status,
		Raw:     res.Body,
	}, nil
}

// Update replaces the basket and the amount of a payment the payer has not
// paid yet, which is what a "change your order" flow needs.
func (g *Gateway) Update(ctx context.Context, paymentToken string, req core.PurchaseRequest) error {
	body := updateRequest{
		Amount:               req.Amount.Rial(),
		PaymentMethodTypeDto: g.paymentMethod(),
		PaymentToken:         paymentToken,
		DiscountAmount:       metadataRial(req, DiscountKey),
		ExternalSourceAmount: metadataRial(req, ExternalSourceKey),
		CartList:             g.carts(req),
	}

	var out verifyResponse
	if _, err := g.auth.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, updatePath), body, nil, &out); err != nil {
		return core.NewError(Name, "update", err)
	}
	if !out.Successful {
		return failure("update", out.envelope, "snapppay rejected the update")
	}
	return nil
}

// ParseCallback reads the parameters SnappPay returns the payer with. The
// provider reports the outcome of the credit decision here, but the payment is
// only final once [Gateway.Verify] succeeded.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}

	status := core.FirstValue(values, "status", "state", "result")
	token := core.FirstValue(values, "paymentToken", "payment_token", "token")

	return core.Callback{
		Gateway: Name,
		// SnappPay returns without a status field on the happy path of some
		// contracts, so a token with no explicit failure counts as success.
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
	// Reverting money is not replayed: a lost answer must not send it twice.
	auth := g.auth
	if op == "refund" {
		auth = auth.NoRetry()
	}
	if paymentToken == "" {
		return verifyData{}, transport.Response{}, core.NewError(Name, op, core.ErrInvalidRequest).
			WithMessage("payment token is required")
	}

	var out verifyResponse
	res, err := auth.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, path),
		paymentTokenRequest{PaymentToken: paymentToken}, nil, &out)
	if err != nil {
		return verifyData{}, res, core.NewError(Name, op, err)
	}
	if !out.Successful {
		return verifyData{}, res, failure(op, out.envelope, "snapppay rejected the "+op)
	}
	return out.Response, res, nil
}

// fetchToken runs the OAuth password grant and returns the bearer token.
func (g *Gateway) fetchToken(ctx context.Context) (string, time.Duration, error) {
	scope := g.settings.scope
	if scope == "" {
		scope = defaultScope
	}
	form := url.Values{
		"grant_type": {"password"},
		"scope":      {scope},
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
			message = "snapppay refused the merchant credentials"
		}
		return "", 0, core.NewError(Name, "token", core.ErrInvalidConfig).
			WithCode(out.Error).WithMessage(message)
	}
	return out.AccessToken, time.Duration(out.ExpiresIn) * time.Second, nil
}

// paymentMethod returns the configured SnappPay payment method type.
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
		CartID:      1,
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
	return core.NewError(Name, op, core.ErrPaymentFailed).
		WithCode(env.ErrorData.code()).
		WithMessage(env.ErrorData.message(fallback))
}

// succeeded interprets the status of a callback.
func succeeded(status, token string) bool {
	switch strings.ToUpper(status) {
	case "OK", "SUCCESS", "SUCCESSFUL", "TRUE", "1", "COMPLETED", "VERIFY", "SETTLE":
		return true
	case "":
		return token != ""
	default:
		return false
	}
}

// mapStatus converts a SnappPay payment state into the normalised status.
func mapStatus(status string) core.Status {
	switch strings.ToUpper(status) {
	case "INIT", "PENDING", "IN_PROGRESS":
		return core.StatusPending
	case "VERIFY", "VERIFIED":
		return core.StatusPaid
	case "SETTLE", "SETTLED", "SUCCESS", "SUCCESSFUL":
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

// internationalMobile converts an Iranian mobile number to the +98 form
// SnappPay expects, leaving anything else untouched.
func internationalMobile(mobile string) string {
	mobile = strings.TrimSpace(mobile)
	switch {
	case mobile == "":
		return ""
	case strings.HasPrefix(mobile, "+98"):
		return mobile
	case strings.HasPrefix(mobile, "0098"):
		return "+98" + mobile[4:]
	case strings.HasPrefix(mobile, "98") && len(mobile) == 12:
		return "+" + mobile
	case strings.HasPrefix(mobile, "0"):
		return "+98" + mobile[1:]
	default:
		return mobile
	}
}

// metadataRial reads an optional amount from the request metadata and converts
// it to Rial using the currency of the request.
func metadataRial(req core.PurchaseRequest, key string) int64 {
	raw, ok := req.Metadata[key]
	if !ok || raw == "" {
		return 0
	}
	amount, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return core.Money{Amount: amount, Currency: req.Amount.Currency}.Rial()
}

// basicAuth builds the HTTP basic credential of the OAuth client.
func basicAuth(clientID, clientSecret string) string {
	return base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
}
