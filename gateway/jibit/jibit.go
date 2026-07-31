// Package jibit implements the Jibit Proxy Payment Gateway (PPG v3, REST,
// napi.jibit.ir).
//
// Jibit is not an acquirer: it sits in front of several PSPs and switches a
// purchase to one of them, which is why the payer is redirected to a
// pspSwitchingUrl rather than to a bank page Payvand knows about. Verification,
// reversal and refunds are all done against Jibit itself.
//
// Credentials: [core.Config.MerchantKey] is the API key and
// [core.Config.Password] is the secret key of the PPG client. Payvand exchanges
// them for a bearer token on the first call and refreshes it automatically.
package jibit

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
	"github.com/amiranmanesh/payvand/internal/tokenauth"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Name is the registry name of this gateway.
const Name core.Name = "jibit"

// Provider endpoints.
const (
	defaultBaseURL = "https://napi.jibit.ir/ppg"

	tokensPath    = "/v3/tokens"
	purchasesPath = "/v3/purchases"
	refundPath    = "/v3/purchases/refund"
	reversePath   = "/v3/purchases/reverse"
)

// tokenLifetime is how long a Jibit access token is valid. The provider does
// not report it in the response, so it is applied by the client.
const tokenLifetime = 24 * time.Hour

// minAmount is the smallest purchase Jibit accepts, in Rial.
const minAmount = 5_000

// Verification results returned in the status field.
const (
	statusSuccess         = "SUCCESS"
	statusSuccessful      = "SUCCESSFUL"
	statusAlreadyVerified = "ALREADY_VERIFIED"
	statusAlreadyReversed = "ALREADY_REVERSED"
)

// Authentication error codes that mean the token must be renewed.
const (
	errAuthRequired       = "security.auth_required"
	errTokenVerification  = "token.verification_failed"
	errPurchaseNotFound   = "purchase.not_found"
	errDuplicateReference = "clientReferenceNumber.duplicated"
)

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the Jibit implementation of [core.Gateway].
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

// New builds a Jibit gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	if strings.TrimSpace(cfg.MerchantKey) == "" {
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("MerchantKey (api key) is required")
	}
	if strings.TrimSpace(cfg.Password) == "" {
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("Password (secret key) is required")
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
		tokens:   &tokenauth.Cache{Fallback: tokenLifetime},
		baseURL:  baseURL,
	}
	gw.auth = &tokenauth.Client{
		Cache:     gw.tokens,
		Transport: gw.client,
		Fetch:     gw.fetchToken,
		Expired:   expiredToken,
	}
	return gw, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what Jibit offers.
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

// Purchase creates a purchase and returns the PSP switching URL the payer is
// sent to. Jibit expires a purchase that is not verified within fifteen
// minutes, so persist the token and verify as soon as the callback arrives.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	if req.Amount.Rial() < minAmount {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be at least 5000 Rial")
	}
	if req.CallbackURL == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("callback url is required")
	}
	if req.OrderID == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("order id is required; Jibit uses it as clientReferenceNumber")
	}

	description := req.Description
	if description == "" {
		description = g.settings.description
	}
	payerCard := ""
	if g.settings.matchPayerCard && len(req.AllowedCards) > 0 {
		payerCard = req.AllowedCards[0]
	}
	userIdentifier := g.settings.userIdentifier
	if userIdentifier == "" {
		userIdentifier = req.Mobile
	}

	body := purchaseRequest{
		Amount:                req.Amount.Rial(),
		Wage:                  g.settings.wage,
		Currency:              core.IRR.String(),
		CallbackURL:           req.CallbackURL,
		ClientReferenceNumber: req.OrderID,
		PayerCardNumber:       payerCard,
		PayerMobileNumber:     req.Mobile,
		PayerNationalCode:     req.NationalID,
		UserIdentifier:        userIdentifier,
		Description:           description,
		AdditionalData:        g.settings.additionalData,
	}

	var out purchaseResponse
	res, err := g.authorized(ctx, http.MethodPost, transport.JoinURL(g.baseURL, purchasesPath), body, &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if len(out.Errors) > 0 || out.PSPSwitchingURL == "" {
		return core.PurchaseResponse{}, failure(Name, "purchase", out.Errors, res)
	}

	return core.PurchaseResponse{
		Token:    purchaseToken(out),
		OrderID:  req.OrderID,
		Amount:   req.Amount,
		Raw:      res.Body,
		Redirect: core.Redirect{Method: http.MethodGet, URL: out.PSPSwitchingURL},
	}, nil
}

// Verify settles a purchase the payer has completed. A purchase that Jibit
// already verified is reported with [core.ErrAlreadyVerified], which callers
// normally treat as success.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	if req.Token == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("token (purchase id) is required")
	}

	endpoint := transport.JoinURL(g.baseURL, purchasesPath+"/"+req.Token+"/verify")

	var out verifyResponse
	res, err := g.authorized(ctx, http.MethodPost, endpoint, nil, &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	switch {
	case strings.EqualFold(out.Status, statusAlreadyVerified):
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrAlreadyVerified).
			WithCode(out.Status)
	case len(out.Errors) > 0:
		return core.VerifyResponse{}, failure(Name, "verify", out.Errors, res)
	case !strings.EqualFold(out.Status, statusSuccess) && !strings.EqualFold(out.Status, statusSuccessful):
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithCode(out.Status).WithMessage("jibit did not verify the purchase")
	}

	// The verify endpoint answers with a status only, so the reference number,
	// the card and above all the amount come from the purchase itself. The
	// inquiry is what makes the settled amount checkable at all here, so it is
	// worth its round trip whenever the caller stated what it expects to be
	// paid, and not only when the callback fields are missing.
	verified := core.VerifyResponse{
		ReferenceNumber: req.ReferenceNumber,
		TransactionID:   req.Token,
		OrderID:         req.OrderID,
		CardNumber:      req.CardNumber,
		Amount:          req.Amount,
		Raw:             res.Body,
	}
	if verified.ReferenceNumber == "" || !req.Amount.IsZero() {
		inquiry, inquiryErr := g.Inquiry(ctx, core.InquiryRequest{Token: req.Token})
		if inquiryErr == nil {
			amount, err := core.SettledAmount(Name, req.Amount, inquiry.Amount)
			if err != nil {
				return core.VerifyResponse{}, err
			}
			verified.Amount = amount
			if verified.ReferenceNumber == "" {
				verified.ReferenceNumber = inquiry.ReferenceNumber
			}
			if verified.CardNumber == "" {
				verified.CardNumber = inquiry.CardNumber
			}
		}
	}
	return verified, nil
}

// Refund returns money to the payer's card. Leave
// [core.RefundRequest.Amount] at zero for a full refund.
func (g *Gateway) Refund(ctx context.Context, req core.RefundRequest) (core.RefundResponse, error) {
	body := refundRequest{Amount: req.Amount.Rial(), Cancellable: g.settings.refundCancellable}
	switch {
	case req.Token != "":
		id, err := strconv.ParseInt(req.Token, 10, 64)
		if err != nil {
			return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrInvalidRequest).
				WithMessage("token must be the numeric purchase id")
		}
		body.PurchaseID = id
	case req.OrderID != "":
		body.ClientReferenceNumber = req.OrderID
	default:
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrInvalidRequest).
			WithMessage("token (purchase id) or order id is required")
	}

	var out refundResponse
	res, err := g.authorized(ctx, http.MethodPost, transport.JoinURL(g.baseURL, refundPath), body, &out)
	if err != nil {
		return core.RefundResponse{}, core.NewError(Name, "refund", err)
	}
	if len(out.Errors) > 0 || out.RefundID == "" {
		return core.RefundResponse{}, failure(Name, "refund", out.Errors, res)
	}

	return core.RefundResponse{RefundID: out.RefundID, Amount: req.Amount, Raw: res.Body}, nil
}

// Reverse cancels a purchase that was paid but not verified yet, returning the
// money to the payer through the PSP. Jibit exposes it next to Refund because
// the two settle differently; use Reverse while the payment is still fresh and
// Refund afterwards.
func (g *Gateway) Reverse(ctx context.Context, req core.RefundRequest) (core.RefundResponse, error) {
	body := reverseRequest{}
	switch {
	case req.Token != "":
		id, err := strconv.ParseInt(req.Token, 10, 64)
		if err != nil {
			return core.RefundResponse{}, core.NewError(Name, "reverse", core.ErrInvalidRequest).
				WithMessage("token must be the numeric purchase id")
		}
		body.PurchaseID = id
	case req.OrderID != "":
		body.ClientReferenceNumber = req.OrderID
	default:
		return core.RefundResponse{}, core.NewError(Name, "reverse", core.ErrInvalidRequest).
			WithMessage("token (purchase id) or order id is required")
	}

	var out reverseResponse
	res, err := g.authorized(ctx, http.MethodPost, transport.JoinURL(g.baseURL, reversePath), body, &out)
	if err != nil {
		return core.RefundResponse{}, core.NewError(Name, "reverse", err)
	}
	switch {
	case len(out.Errors) > 0:
		return core.RefundResponse{}, failure(Name, "reverse", out.Errors, res)
	case strings.EqualFold(out.Status, statusAlreadyReversed):
		return core.RefundResponse{Amount: req.Amount, Raw: res.Body}, nil
	case !strings.EqualFold(out.Status, statusSuccessful) && !strings.EqualFold(out.Status, statusSuccess):
		return core.RefundResponse{}, core.NewError(Name, "reverse", core.ErrPaymentFailed).
			WithCode(out.Status).WithMessage("jibit did not reverse the purchase")
	}

	return core.RefundResponse{Amount: req.Amount, Raw: res.Body}, nil
}

// Inquiry reads the current state of a purchase, which is how a lost callback
// is recovered from.
func (g *Gateway) Inquiry(ctx context.Context, req core.InquiryRequest) (core.InquiryResponse, error) {
	query := ""
	switch {
	case req.Token != "":
		query = "?purchaseId=" + req.Token
	case req.OrderID != "":
		query = "?clientReferenceNumber=" + req.OrderID
	default:
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", core.ErrInvalidRequest).
			WithMessage("token (purchase id) or order id is required")
	}

	var out listResponse
	res, err := g.authorized(ctx, http.MethodGet, transport.JoinURL(g.baseURL, purchasesPath)+query, nil, &out)
	if err != nil {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", err)
	}
	if len(out.Errors) > 0 {
		return core.InquiryResponse{}, failure(Name, "inquiry", out.Errors, res)
	}
	if len(out.Elements) == 0 {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", core.ErrPaymentFailed).
			WithCode(errPurchaseNotFound).WithMessage("jibit knows no such purchase")
	}

	element := out.Elements[0]
	card := element.PayerMaskedCardNumber
	if card == "" {
		card = element.PayerCardNumber
	}
	reference := element.PSPRRN
	if reference == "" {
		reference = element.PSPReferenceNumber
	}

	return core.InquiryResponse{
		Status:          mapState(element.State),
		ReferenceNumber: reference,
		CardNumber:      card,
		Amount:          core.Rial(element.Amount),
		PaidAt:          parseTime(element.UpdatedAt),
		Message:         element.State,
		Raw:             res.Body,
	}, nil
}

// ParseCallback reads the form Jibit posts back to the merchant.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}

	status := core.FirstValue(values, "status")
	amount := core.Money{}
	if parsed, err := strconv.ParseInt(core.FirstValue(values, "amount"), 10, 64); err == nil {
		amount = core.Rial(parsed)
	}

	return core.Callback{
		Gateway:         Name,
		Succeeded:       strings.EqualFold(status, statusSuccessful),
		Token:           core.FirstValue(values, "purchaseId", "purchaseIdStr"),
		OrderID:         core.FirstValue(values, "clientReferenceNumber"),
		ReferenceNumber: core.FirstValue(values, "pspRRN", "pspRrn", "pspReferenceNumber"),
		TraceNumber:     core.FirstValue(values, "pspTraceNumber"),
		CardNumber:      core.FirstValue(values, "payerMaskedCardNumber", "payerCardNumber"),
		Amount:          amount,
		Code:            status,
		Message:         core.FirstValue(values, "failReason"),
		Values:          values,
	}, nil
}

// authorized performs an authenticated call, renewing the bearer token once
// when Jibit reports that it expired.
func (g *Gateway) authorized(ctx context.Context, method, endpoint string, body any, out any) (transport.Response, error) {
	return g.auth.JSON(ctx, method, endpoint, body, nil, out)
}

// fetchToken exchanges the API key and the secret key for an access token.
func (g *Gateway) fetchToken(ctx context.Context) (string, time.Duration, error) {
	var out tokenResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, tokensPath),
		tokenRequest{APIKey: g.cfg.MerchantKey, SecretKey: g.cfg.Password}, nil, &out)
	if err != nil {
		return "", 0, core.NewError(Name, "token", err)
	}
	if out.AccessToken == "" {
		return "", 0, failure(Name, "token", out.Errors, res)
	}
	return out.AccessToken, tokenLifetime, nil
}

// expiredToken reports whether a response says the bearer token is no longer
// accepted.
func expiredToken(res transport.Response) bool {
	if res.StatusCode == http.StatusUnauthorized {
		return true
	}
	var body struct {
		Errors []serviceError `json:"errors"`
	}
	if json.Unmarshal([]byte(res.Body), &body) != nil {
		return false
	}
	for _, e := range body.Errors {
		if e.Code == errAuthRequired || e.Code == errTokenVerification {
			return true
		}
	}
	return false
}

// purchaseToken picks the identifier to persist next to the order.
func purchaseToken(out purchaseResponse) string {
	if out.PurchaseIDStr != "" {
		return out.PurchaseIDStr
	}
	return strconv.FormatInt(out.PurchaseID, 10)
}

// failure turns a Jibit error list into the package error type.
func failure(name core.Name, op string, errs []serviceError, res transport.Response) error {
	e := first(errs)
	message := e.Message
	if message == "" {
		message = e.Code
	}
	if message == "" {
		message = "jibit rejected the request with HTTP " + strconv.Itoa(res.StatusCode)
	}

	wrapped := core.ErrPaymentFailed
	switch e.Code {
	case errDuplicateReference:
		wrapped = core.ErrInvalidRequest
	case errAuthRequired, errTokenVerification:
		wrapped = core.ErrInvalidConfig
	}
	return core.NewError(name, op, wrapped).WithCode(e.Code).WithMessage(message)
}

// mapState converts a Jibit purchase state into the normalised status.
func mapState(state string) core.Status {
	switch strings.ToUpper(state) {
	case "IN_PROGRESS", "PENDING", "CREATED":
		return core.StatusPending
	case "READY_TO_VERIFY", "SUCCESSFUL", "SUCCESS":
		return core.StatusPaid
	case "VERIFIED":
		return core.StatusVerified
	case "FAILED", "EXPIRED":
		return core.StatusFailed
	case "REVERSED", "REFUNDED":
		return core.StatusRefunded
	default:
		return core.StatusUnknown
	}
}

// parseTime converts a Jibit timestamp ("2006-01-02T15:04:05").
func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
