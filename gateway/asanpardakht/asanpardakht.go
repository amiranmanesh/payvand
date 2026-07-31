// Package asanpardakht implements the AsanPardakht IPG
// (REST v1, ipgrest.asanpardakht.ir).
//
// Credentials: [core.Config.MerchantID] is the merchant configuration id,
// [core.Config.Username] and [core.Config.Password] are the "usr" and "pwd"
// headers of the REST API.
//
// The provider settles in three steps — read the transaction, verify it, then
// settle it — and [Gateway.Verify] performs all three.
package asanpardakht

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
const Name core.Name = "asanpardakht"

// PayGateTranIDKey is the key of the provider transaction id inside
// [core.VerifyRequest.Extra] and [core.RefundRequest.Extra].
//
// Fill it from your own records only. [core.Gateway.Verify] looks the
// transaction up from the invoice id and uses the supplied value solely to
// cross-check that answer, because the callback map every [core.Callback]
// carries reaches Extra unfiltered. [core.Gateway.Refund] does skip the lookup
// when no order id is given, which makes the value the transaction to reverse:
// never hand it one that came back from the payer's browser.
const PayGateTranIDKey = "pay_gate_tran_id"

// Provider endpoints. The API and the payment page live on different hosts.
const (
	defaultAPIBase = "https://ipgrest.asanpardakht.ir"
	defaultPayBase = "https://asan.shaparak.ir"

	tokenPath      = "/v1/Token"
	tranResultPath = "/v1/TranResult"
	verifyPath     = "/v1/Verify"
	settlementPath = "/v1/Settlement"
	cancelPath     = "/v1/Cancel"
	reversePath    = "/v1/Reverse"
)

// defaultServiceType is the service type of a normal purchase.
const defaultServiceType = 1

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the AsanPardakht implementation of [core.Gateway].
type Gateway struct {
	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	apiBase  string
	payBase  string
}

var _ core.Gateway = (*Gateway)(nil)

// New builds an AsanPardakht gateway.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	switch {
	case strings.TrimSpace(cfg.MerchantID) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).
			WithMessage("MerchantID (merchant configuration id) is required")
	case strings.TrimSpace(cfg.Username) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).WithMessage("Username is required")
	case strings.TrimSpace(cfg.Password) == "":
		return nil, core.NewError(Name, "new", core.ErrInvalidConfig).WithMessage("Password is required")
	}

	apiBase, payBase := defaultAPIBase, defaultPayBase
	if options.BaseURL != "" {
		apiBase, payBase = options.BaseURL, options.BaseURL
	}

	return &Gateway{
		cfg:      cfg,
		opts:     options,
		settings: gwopt.From[config](options, string(Name)),
		client:   transport.New(options),
		apiBase:  apiBase,
		payBase:  payBase,
	}, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what AsanPardakht offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Refund:         true,
		Inquiry:        true,
		Callback:       true,
		RedirectMethod: http.MethodPost,
		Multiplexing:   true,
		Currencies:     []core.Currency{core.IRR},
	}
}

// headers builds the authenticated headers of every call.
func (g *Gateway) headers() map[string]string {
	return map[string]string{"usr": g.cfg.Username, "pwd": g.cfg.Password}
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

	serviceType := g.settings.serviceTypeID
	if serviceType == 0 {
		serviceType = defaultServiceType
	}

	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.apiBase, tokenPath), tokenRequest{
		ServiceTypeID:           serviceType,
		MerchantConfigurationID: g.cfg.MerchantID,
		LocalInvoiceID:          req.OrderID,
		AmountInRials:           req.Amount.Rial(),
		AdditionalData:          g.settings.additionalData,
		CallbackURL:             req.CallbackURL,
		PaymentID:               g.settings.paymentID,
		Settlements:             g.settings.settlements,
	}, g.headers(), nil)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}

	// The endpoint answers with the token as a bare JSON string.
	token := strings.Trim(strings.TrimSpace(res.Body), `"`)
	if !res.OK() || token == "" {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrPaymentFailed).
			WithCode(strconv.Itoa(res.StatusCode)).WithMessage(errorMessage(res.Body))
	}

	params := map[string]string{"RefId": token}
	if req.Mobile != "" {
		params["mobileap"] = req.Mobile
	}

	return core.PurchaseResponse{
		Token:    token,
		OrderID:  req.OrderID,
		Amount:   req.Amount,
		Raw:      res.Body,
		Redirect: core.Redirect{Method: http.MethodPost, URL: g.payBase, Params: params},
	}, nil
}

// Verify runs the provider's three step settlement: read the transaction,
// verify it and settle it.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	if req.OrderID == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("order id (local invoice id) is required")
	}

	result, res, err := g.tranResult(ctx, "verify", req.OrderID)
	if err != nil {
		return core.VerifyResponse{}, err
	}
	if err := checkTranID(req.Get(PayGateTranIDKey), result.PayGateTranID, "verify"); err != nil {
		return core.VerifyResponse{}, err
	}
	if result.PayGateTranID == 0 {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithMessage("the transaction was not found on the provider side")
	}
	// An invoice the provider knows always carries the amount it was paid for,
	// so a zero here is an answer that cannot be settled against, not a reason
	// to skip the comparison.
	if req.Amount.Rial() > 0 && result.Amount != req.Amount.Rial() {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrAmountMismatch).
			WithMessage("the provider reported " + strconv.FormatInt(result.Amount, 10) + " Rial")
	}

	body := tranRequest{MerchantConfigurationID: g.cfg.MerchantID, PayGateTranID: result.PayGateTranID}
	if err := g.command(ctx, "verify", verifyPath, body); err != nil {
		return core.VerifyResponse{}, err
	}
	if !g.settings.skipSettlement {
		if err := g.command(ctx, "verify", settlementPath, body); err != nil {
			return core.VerifyResponse{}, err
		}
	}

	amount := req.Amount
	if result.Amount > 0 {
		amount = core.Rial(result.Amount)
	}

	return core.VerifyResponse{
		ReferenceNumber: firstNonEmpty(result.RRN, result.RefID),
		TransactionID:   strconv.FormatInt(result.PayGateTranID, 10),
		OrderID:         req.OrderID,
		CardNumber:      firstNonEmpty(result.CardNumber, req.CardNumber),
		Amount:          amount,
		Raw:             res.Body,
	}, nil
}

// Refund reverses a settled transaction, or cancels an unsettled one when the
// gateway was built with [WithCancelInsteadOfReverse].
func (g *Gateway) Refund(ctx context.Context, req core.RefundRequest) (core.RefundResponse, error) {
	tranID, err := g.resolveTranID(ctx, "refund", req.OrderID, firstNonEmpty(req.Get(PayGateTranIDKey), req.TransactionID))
	if err != nil {
		return core.RefundResponse{}, err
	}

	path := reversePath
	if g.settings.cancelInsteadOfReverse {
		path = cancelPath
	}
	body := tranRequest{MerchantConfigurationID: g.cfg.MerchantID, PayGateTranID: tranID}
	if err := g.command(ctx, "refund", path, body); err != nil {
		return core.RefundResponse{}, err
	}

	return core.RefundResponse{RefundID: strconv.FormatInt(tranID, 10), Amount: req.Amount}, nil
}

// Inquiry reads the state of a transaction from its local invoice id.
func (g *Gateway) Inquiry(ctx context.Context, req core.InquiryRequest) (core.InquiryResponse, error) {
	if req.OrderID == "" {
		return core.InquiryResponse{}, core.NewError(Name, "inquiry", core.ErrInvalidRequest).
			WithMessage("order id (local invoice id) is required")
	}

	result, res, err := g.tranResult(ctx, "inquiry", req.OrderID)
	if err != nil {
		return core.InquiryResponse{}, err
	}

	return core.InquiryResponse{
		Status:          mapStatus(result.ServiceStatusCode, result.PayGateTranID),
		ReferenceNumber: firstNonEmpty(result.RRN, result.RefID),
		CardNumber:      result.CardNumber,
		Amount:          core.Rial(result.Amount),
		Message:         result.Message,
		Raw:             res.Body,
	}, nil
}

// ParseCallback reads the fields AsanPardakht posts to the callback URL.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	invoice := core.FirstValue(values, "localInvoiceId", "LocalInvoiceId", "InvoiceId")
	return core.Callback{
		Gateway:   Name,
		Succeeded: invoice != "",
		Token:     core.FirstValue(values, "RefId", "refId", "ReturnId"),
		OrderID:   invoice,
		Values:    values,
	}, nil
}

// tranResult reads the provider side state of a local invoice.
func (g *Gateway) tranResult(ctx context.Context, op, orderID string) (tranResult, transport.Response, error) {
	endpoint := transport.JoinURL(g.apiBase, tranResultPath) + "?" + url.Values{
		"merchantConfigurationId": {g.cfg.MerchantID},
		"localInvoiceId":          {orderID},
	}.Encode()

	var out tranResult
	res, err := g.client.JSON(ctx, http.MethodGet, endpoint, nil, g.headers(), &out)
	if err != nil {
		return out, res, core.NewError(Name, op, err)
	}
	if !res.OK() {
		return out, res, core.NewError(Name, op, core.ErrPaymentFailed).
			WithCode(strconv.Itoa(res.StatusCode)).WithMessage(errorMessage(res.Body))
	}
	return out, res, nil
}

// resolveTranID returns the provider transaction id. An invoice id is resolved
// through the provider, which then also settles what the caller supplied; a
// bare transaction id is taken as given, since there is nothing to check it
// against.
func (g *Gateway) resolveTranID(ctx context.Context, op, orderID, known string) (int64, error) {
	if orderID == "" {
		if known == "" {
			return 0, core.NewError(Name, op, core.ErrInvalidRequest).
				WithMessage("order id or payGateTranId is required")
		}
		parsed, err := strconv.ParseInt(known, 10, 64)
		if err != nil {
			return 0, core.NewError(Name, op, core.ErrInvalidRequest).
				WithMessage("payGateTranId must be numeric")
		}
		return parsed, nil
	}

	result, _, err := g.tranResult(ctx, op, orderID)
	if err != nil {
		return 0, err
	}
	if result.PayGateTranID == 0 {
		return 0, core.NewError(Name, op, core.ErrPaymentFailed).
			WithMessage("the transaction was not found on the provider side")
	}
	if err := checkTranID(known, result.PayGateTranID, op); err != nil {
		return 0, err
	}
	return result.PayGateTranID, nil
}

// checkTranID cross-checks a caller supplied payGateTranId against the one the
// provider reports for the invoice.
//
// The supplied value is never used as the settlement key.
// [core.Callback.VerifyRequest] copies the whole callback map into Extra, so
// anything read from there is ultimately chosen by the payer's browser; letting
// it name the transaction to settle would settle an order against a payment
// that was never made for it. A value that disagrees with the provider is
// reported instead of ignored, so a caller passing the wrong one finds out.
func checkTranID(supplied string, found int64, op string) error {
	if supplied == "" {
		return nil
	}
	parsed, err := strconv.ParseInt(supplied, 10, 64)
	if err != nil || parsed != found {
		return core.NewError(Name, op, core.ErrInvalidRequest).
			WithMessage("the supplied payGateTranId is not the transaction the provider reports for this invoice")
	}
	return nil
}

// command posts one of the verify/settle/cancel/reverse bodies.
func (g *Gateway) command(ctx context.Context, op, path string, body tranRequest) error {
	res, err := g.client.JSON(ctx, http.MethodPost, transport.JoinURL(g.apiBase, path), body, g.headers(), nil)
	if err != nil {
		return core.NewError(Name, op, err)
	}
	if !res.OK() {
		return core.NewError(Name, op, core.ErrPaymentFailed).
			WithCode(strconv.Itoa(res.StatusCode)).WithMessage(errorMessage(res.Body))
	}
	return nil
}

// mapStatus translates an AsanPardakht service status into the shared
// vocabulary.
func mapStatus(serviceStatusCode string, tranID int64) core.Status {
	switch serviceStatusCode {
	case "OK", "ok", "":
		if tranID > 0 {
			return core.StatusPaid
		}
		return core.StatusPending
	default:
		return core.StatusFailed
	}
}

// errorMessage extracts the provider message out of an error body.
func errorMessage(body string) string {
	var detail errorResponse
	if err := json.Unmarshal([]byte(body), &detail); err == nil && detail.Message != "" {
		return detail.Message
	}
	return strings.TrimSpace(body)
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
