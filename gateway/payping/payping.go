// Package payping implements the PayPing gateway (REST v3, api.payping.ir).
//
// Credentials: [core.Config.MerchantKey] is the bearer token of the terminal.
// PayPing works in Toman — its specification states that every one of its
// services does — and Payvand converts the amount for you.
//
// The v3 flow differs from v2 in three ways that matter to a caller: the
// payment is identified by a payment code plus a numeric payment reference
// rather than by a single reference, the callback arrives as a POST form whose
// "data" field holds a JSON document, and a verified payment can be reversed
// when it was created with [WithReversible].
package payping

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
	"github.com/amiranmanesh/payvand/internal/transport"
)

// Name is the registry name of this gateway.
const Name core.Name = "payping"

// Provider endpoints.
const (
	defaultBase   = "https://api.payping.ir"
	payPath       = "/v3/pay"
	sharedPayPath = "/v3/pay/shared"
	verifyPath    = "/v3/pay/verify"
	reversePath   = "/v3/pay/reverse"
	startPath     = "/v3/pay/start/"
	reportPath    = "/v1/report/"
)

// codeAlreadyVerified is the metaData.code PayPing answers a repeated
// verification with, inside an HTTP 409.
const codeAlreadyVerified = 110

// Payment states reported by the report endpoint.
const (
	statusCodeCreated  = 0 // the payment code exists, nothing happened yet
	statusCodeVerified = 1 // paid and verified
	statusCodeEntered  = 2 // the payer opened the bank page
	statusCodeCanceled = 3 // the payer aborted
	statusCodeBankFail = 4 // paid on the bank page but the bank verify failed
	statusCodeAwaiting = 5 // returned from the bank, awaiting the merchant verify
)

// payedDateLayout is the shape PayPing stamps its UTC payment dates with.
const payedDateLayout = "2006-01-02 15:04:05Z"

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the PayPing implementation of [core.Gateway].
type Gateway struct {
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	baseURL  string
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a PayPing gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	if strings.TrimSpace(cfg.MerchantKey) == "" {
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("MerchantKey (bearer token) is required")
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

// Capabilities reports what PayPing offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Refund:         true,
		Inquiry:        true,
		Callback:       true,
		RedirectMethod: http.MethodGet,
		Multiplexing:   true,
		Currencies:     []core.Currency{core.IRT},
	}
}

// headers builds the authenticated headers of every call.
func (g *Gateway) headers() map[string]string {
	return map[string]string{"Authorization": "Bearer " + g.cfg.MerchantKey}
}

// Purchase creates a payment and returns the PayPing redirect URL.
func (g *Gateway) Purchase(ctx context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	amount := req.Amount.Toman()
	if amount <= 0 {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be at least 1 Toman")
	}
	if req.CallbackURL == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("callback url is required")
	}

	description := req.Description
	if description == "" {
		description = g.settings.description
	}
	identity := g.settings.payerIdentity
	if identity == "" {
		identity = firstNonEmpty(req.Mobile, req.Email)
	}

	body := payRequest{
		Amount:        amount,
		ReturnURL:     req.CallbackURL,
		PayerName:     req.PayerName,
		PayerIdentity: identity,
		Description:   description,
		ClientRefID:   req.OrderID,
		IsReversible:  g.settings.reversible,
		IsBlocked:     g.settings.blocked,
	}
	endpoint := payPath

	// A shared payment is a different endpoint whose total is the sum of the
	// shares. Checking that sum here is the only place the caller's amount and
	// the split can still be reconciled: PayPing accepts the shares alone and
	// would otherwise charge a total nobody asked for.
	if shares := g.settings.multiplexing; len(shares) > 0 {
		var total int64
		for _, share := range shares {
			total += share.Amount
		}
		if total != amount {
			return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
				WithMessage("the shares add up to " + strconv.FormatInt(total, 10) +
					" Toman, not the requested " + strconv.FormatInt(amount, 10))
		}
		endpoint, body.Amount, body.Items = sharedPayPath, 0, shares
	}

	var out payResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, endpoint), body, g.headers(), &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if !res.OK() || out.PaymentCode == "" {
		return core.PurchaseResponse{}, g.failure("purchase", res)
	}

	// PayPing hands back the address to send the payer to. Building it from the
	// payment code is the fallback, and the path a WithBaseURL sandbox takes.
	redirect := out.URL
	if redirect == "" {
		redirect = transport.JoinURL(g.baseURL, startPath+url.PathEscape(out.PaymentCode))
	}

	return core.PurchaseResponse{
		Token:   out.PaymentCode,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Extra: map[string]string{
			"paymentCode":   out.PaymentCode,
			"gatewayAmount": strconv.FormatInt(out.GatewayAmount, 10),
		},
		Raw: res.Body,
		Redirect: core.Redirect{
			Method: http.MethodGet,
			URL:    redirect,
		},
	}, nil
}

// Verify settles the payment the callback reported. PayPing rolls a payment
// back after ten minutes without a verification, and answers a repeated one
// with [core.ErrAlreadyVerified].
//
// A provider still settling the payment is reported as
// [core.ErrVerificationPending]: the money is neither lost nor confirmed, and
// the documented recovery is to call Verify again rather than to charge the
// payer twice.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	paymentCode := firstNonEmpty(req.Token, req.Get("paymentCode"), req.Get("PaymentCode"), req.Get("code"))
	if paymentCode == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("paymentCode is required")
	}
	paymentRefID, ok := refID(req.ReferenceNumber, req.Get("paymentRefId"), req.Get("PaymentRefId"), req.Get("refId"))
	if !ok {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("a numeric paymentRefId is required")
	}
	amount := req.Amount.Toman()
	if amount <= 0 {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("amount is required: PayPing verifies against the amount of the payment order")
	}

	var out verifyResponse
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, verifyPath), verifyRequest{
		PaymentRefID: paymentRefID,
		PaymentCode:  paymentCode,
		Amount:       amount,
	}, g.headers(), &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}

	switch {
	// 202 is a 2xx, so it has to be caught before the success branch.
	case res.StatusCode == http.StatusAccepted || res.StatusCode == http.StatusBadGateway:
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrVerificationPending).
			WithCode(strconv.Itoa(res.StatusCode)).
			WithMessage("PayPing is still settling the payment; call Verify again")
	case res.StatusCode == http.StatusConflict:
		var conflict conflictDetails
		_ = json.Unmarshal([]byte(res.Body), &conflict)
		if conflict.MetaData.Code == codeAlreadyVerified {
			return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrAlreadyVerified).
				WithCode(strconv.Itoa(codeAlreadyVerified)).
				WithMessage("payment " + paymentCode + " was verified before")
		}
		return core.VerifyResponse{}, g.failure("verify", res)
	case !res.OK():
		return core.VerifyResponse{}, g.failure("verify", res)
	}

	// PayPing asks the merchant to reconcile the order reference it echoes with
	// its own record before treating the payment as settled, and it is worth
	// doing: the payment code reaching Verify may have come from the callback
	// rather than from the caller's database, and this is the one comparison
	// whose right hand side the provider states rather than the browser.
	if req.OrderID != "" && out.ClientRefID != "" && out.ClientRefID != req.OrderID {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("the provider settled order " + out.ClientRefID + ", not " + req.OrderID)
	}

	settled, err := core.SettledAmount(Name, req.Amount, core.Toman(out.Amount))
	if err != nil {
		return core.VerifyResponse{}, err
	}
	reference := strconv.FormatInt(paymentRefID, 10)
	if out.PaymentRefID > 0 {
		reference = strconv.FormatInt(out.PaymentRefID, 10)
	}

	return core.VerifyResponse{
		ReferenceNumber: reference,
		TransactionID:   firstNonEmpty(out.Code, paymentCode),
		OrderID:         firstNonEmpty(out.ClientRefID, req.OrderID),
		CardNumber:      firstNonEmpty(out.CardNumber, req.CardNumber),
		CardHash:        out.CardHashPan,
		Amount:          settled,
		// The merchant's share of the fee, converted to the Rial the field is
		// documented in.
		Fee:    core.Toman(out.BusinessWage).Rial(),
		PaidAt: parseTime(out.PayedDate),
		Raw:    res.Body,
	}, nil
}

// Refund reverses a verified payment, returning the whole amount to the card it
// came from. PayPing only accepts it for a payment created with
// [WithReversible] and only within thirty minutes of the verification; partial
// reversals do not exist, so [core.RefundRequest.Amount] is used to check what
// came back rather than to ask for a part of it.
func (g *Gateway) Refund(ctx context.Context, req core.RefundRequest) (core.RefundResponse, error) {
	paymentCode := firstNonEmpty(req.Token, req.TransactionID, req.Get("paymentCode"), req.Get("code"))
	if paymentCode == "" {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrInvalidRequest).
			WithMessage("paymentCode is required")
	}
	paymentRefID, ok := refID(req.ReferenceNumber, req.Get("paymentRefId"), req.Get("PaymentRefId"), req.Get("refId"))
	if !ok {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrInvalidRequest).
			WithMessage("a numeric paymentRefId is required")
	}

	var out reverseResponse
	res, err := g.client.NoRetry().JSON(ctx, http.MethodPost, transport.JoinURL(g.baseURL, reversePath), reverseRequest{
		PaymentRefID: paymentRefID,
		PaymentCode:  paymentCode,
	}, g.headers(), &out)
	if err != nil {
		return core.RefundResponse{}, core.NewError(Name, "refund", err)
	}
	if !res.OK() {
		return core.RefundResponse{}, g.failure("refund", res)
	}

	// The money is already back with the payer here, so a surprising amount is
	// reported rather than turned into an error the caller might retry on.
	reversed := req.Amount
	if out.Amount > 0 {
		reversed = core.Toman(out.Amount)
	}

	return core.RefundResponse{
		RefundID: firstNonEmpty(out.Code, paymentCode),
		Amount:   reversed,
		Raw:      res.Body,
	}, nil
}

// Inquiry reads the state of a payment from its payment code. It is the path
// PayPing documents for a callback that never arrived: a payment reported as
// awaiting is one the payer completed and the merchant still owes a Verify.
func (g *Gateway) Inquiry(ctx context.Context, req core.InquiryRequest) (core.InquiryResponse, error) {
	paymentCode := firstNonEmpty(req.Token, req.Get("paymentCode"), req.Get("code"))
	if paymentCode == "" {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", core.ErrInvalidRequest).
			WithMessage("paymentCode is required")
	}

	var out reportResponse
	endpoint := transport.JoinURL(g.baseURL, reportPath+url.PathEscape(paymentCode))
	res, err := g.client.JSON(ctx, http.MethodGet, endpoint, nil, g.headers(), &out)
	if err != nil {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", err)
	}
	if !res.OK() {
		return core.InquiryResponse{}, g.failure("inquiry", res)
	}

	return core.InquiryResponse{
		Status:          inquiryStatus(out.PaymentStatus, out.IsPaid),
		ReferenceNumber: out.RefID,
		Amount:          core.Toman(out.Amount),
		PaidAt:          parseTime(out.PayDate),
		Message:         statusMessage(out.PaymentStatus),
		Raw:             res.Body,
	}, nil
}

// ParseCallback reads the payment PayPing returns the payer with.
//
// v3 posts an x-www-form-urlencoded body carrying status, errorCode and a
// "data" JSON document; the document is flattened into
// [core.Callback.Values] so a caller reading raw fields sees the same map
// whatever shape the callback arrived in.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}

	var data callbackData
	if raw := core.FirstValue(values, "data", "Data"); raw != "" {
		if json.Unmarshal([]byte(raw), &data) == nil {
			flatten(values, data)
		}
	}

	status := core.FirstValue(values, "status", "Status")
	reference := core.FirstValue(values, "paymentRefId", "PaymentRefId", "refid", "refId", "RefID")
	paymentCode := core.FirstValue(values, "paymentCode", "PaymentCode", "code")

	// status is authoritative when PayPing sends it. Without it — an older
	// callback, or a hand rolled test — a reference number is the only signal
	// that the payer got as far as paying.
	succeeded := reference != ""
	if status != "" {
		succeeded = status == "1" && reference != ""
	}

	return core.Callback{
		Gateway:         Name,
		Succeeded:       succeeded,
		Token:           paymentCode,
		ReferenceNumber: reference,
		OrderID:         core.FirstValue(values, "clientRefId", "ClientRefId", "clientrefid"),
		CardNumber:      core.FirstValue(values, "cardNumber", "CardNumber", "cardnumber"),
		Amount:          core.Toman(data.Amount),
		Code:            firstNonEmpty(core.FirstValue(values, "errorCode", "ErrorCode"), status),
		Values:          values,
	}, nil
}

// failure turns an RFC 7807 error body into the gateway error, keeping the
// business code PayPing puts in metaData so the caller can map it.
func (g *Gateway) failure(op string, res transport.Response) error {
	err := core.NewError(Name, op, core.ErrPaymentFailed).WithMessage(res.Body)

	var problem problemDetails
	if json.Unmarshal([]byte(res.Body), &problem) != nil {
		return err
	}
	if message := problem.message(); message != "" {
		err = err.WithMessage(message)
	}
	if problem.MetaData.Code != 0 {
		err = err.WithCode(strconv.Itoa(problem.MetaData.Code))
	}
	return err
}

// flatten copies the fields of the callback document into the raw value map,
// without overwriting anything the form already carried.
func flatten(values map[string]string, data callbackData) {
	set := func(key, value string) {
		if value == "" || value == "0" {
			return
		}
		if _, exists := values[key]; !exists {
			values[key] = value
		}
	}
	set("clientRefId", data.ClientRefID)
	set("paymentCode", data.PaymentCode)
	set("paymentRefId", data.PaymentRefID.String())
	set("amount", strconv.FormatInt(data.Amount, 10))
	set("gatewayAmount", strconv.FormatInt(data.GatewayAmount, 10))
	set("cardNumber", data.CardNumber)
	set("cardHashPan", data.CardHashPan)
}

// refID reads the numeric payment reference out of the first candidate that
// carries one. PayPing keys v3 on an int64, so anything else is a caller
// mistake rather than a zero worth sending.
func refID(candidates ...string) (int64, bool) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(firstNonEmpty(candidates...)), 10, 64)
	return parsed, err == nil
}

// inquiryStatus maps a PayPing payment state onto the normalised status.
func inquiryStatus(status int, isPaid bool) core.Status {
	switch status {
	case statusCodeVerified:
		return core.StatusVerified
	case statusCodeAwaiting:
		return core.StatusPaid
	case statusCodeCanceled:
		return core.StatusCanceled
	case statusCodeBankFail:
		return core.StatusFailed
	case statusCodeCreated, statusCodeEntered:
		return core.StatusPending
	default:
		if isPaid {
			return core.StatusVerified
		}
		return core.StatusUnknown
	}
}

// statusMessage describes a PayPing payment state in words.
func statusMessage(status int) string {
	switch status {
	case statusCodeCreated:
		return "payment code created"
	case statusCodeVerified:
		return "paid and verified"
	case statusCodeEntered:
		return "payer opened the bank page"
	case statusCodeCanceled:
		return "canceled by the payer"
	case statusCodeBankFail:
		return "paid but the bank verification failed"
	case statusCodeAwaiting:
		return "paid, awaiting the merchant verification"
	default:
		return ""
	}
}

// parseTime reads a PayPing UTC timestamp.
func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{payedDateLayout, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
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
