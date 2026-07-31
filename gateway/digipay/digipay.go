// Package digipay implements the Digipay universal payment gateway (UPG,
// REST, api.mydigipay.com).
//
// Digipay is a wallet, a credit line and a card gateway behind one API: the
// product is chosen with the ticket type, which travels as the "type" query
// parameter of every call and is set with [WithTicketType]. The BNPL and
// credit products additionally expect a basket, supplied with [WithBasket] or
// [WithBasketBuilder], and a delivery confirmation once the goods are shipped,
// sent with [Gateway.Deliver].
//
// Credentials: [core.Config.Username] and [core.Config.Password] are the
// merchant user of the OAuth password grant, while [core.Config.MerchantID]
// and [core.Config.MerchantKey] are the OAuth client id and client secret.
package digipay

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
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
const Name core.Name = "digipay"

// Provider endpoints.
const (
	defaultBaseURL = "https://api.mydigipay.com"
	sandboxBaseURL = "https://uat.mydigipay.info"

	oauthPath   = "/digipay/api/oauth/token"
	ticketPath  = "/digipay/api/tickets/business"
	verifyPath  = "/digipay/api/purchases/verify/"
	reversePath = "/digipay/api/reverse"
	deliverPath = "/digipay/api/purchases/deliver"
	refundPath  = "/digipay/api/refunds"
)

// Defaults applied when the corresponding option is not set.
const (
	defaultAgent      = "WEB"
	defaultAPIVersion = "2022-02-02"
)

// Callback and verification keys. TicketTypeKey is read back from the callback
// so verification uses the product that actually paid, which can differ from
// the configured default when the payer switched inside the Digipay app.
const (
	// TicketTypeKey is the callback field carrying the ticket type.
	TicketTypeKey = "type"
	// TrackingCodeKey is the callback field carrying the tracking code.
	TrackingCodeKey = "trackingCode"
)

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// Gateway is the Digipay implementation of [core.Gateway].
type Gateway struct {
	// Unsupported answers Inquiry, which Digipay offers for refunds only;
	// [Gateway.RefundStatus] exposes that endpoint under its real meaning.
	core.Unsupported

	cfg      core.Config
	opts     *core.Options
	settings *config
	client   *transport.Client
	tokens   *tokenauth.Cache
	auth     *tokenauth.Client

	baseURL string
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a Digipay gateway.
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
	if options.Sandbox {
		baseURL = sandboxBaseURL
	}
	if options.BaseURL != "" {
		baseURL = options.BaseURL
	}

	gw := &Gateway{
		Unsupported: core.Unsupported{GatewayName: Name},
		cfg:         cfg,
		opts:        options,
		settings:    gwopt.From[config](options, string(Name)),
		client:      transport.New(options),
		tokens:      &tokenauth.Cache{},
		baseURL:     baseURL,
	}
	gw.auth = &tokenauth.Client{Cache: gw.tokens, Transport: gw.client, Fetch: gw.fetchToken}
	return gw, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what Digipay offers.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Refund:         true,
		Callback:       true,
		RedirectMethod: http.MethodGet,
		Currencies:     []core.Currency{core.IRR},
	}
}

// Purchase creates a ticket and returns the Digipay page the payer is sent to.
// [core.PurchaseRequest.OrderID] becomes Digipay's providerId and must be
// unique per merchant.
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
			WithMessage("order id is required; Digipay uses it as providerId")
	}

	body := ticketRequest{
		Amount:           req.Amount.Rial(),
		CellNumber:       req.Mobile,
		ProviderID:       req.OrderID,
		CallbackURL:      req.CallbackURL,
		SplitDetailsList: g.settings.splitDetails,
		PreferredGateway: g.settings.preferredGateway,
	}
	if g.settings.basketBuilder != nil {
		body.BasketDetailsDto = g.settings.basketBuilder(req)
	}

	ticketType := g.ticketType()
	endpoint := transport.JoinURL(g.baseURL, ticketPath) + "?type=" + strconv.Itoa(ticketType)
	headers := map[string]string{
		"Agent":           g.agent(),
		"Digipay-Version": g.apiVersion(),
	}

	var out ticketResponse
	res, err := g.auth.JSON(ctx, http.MethodPost, endpoint, body, headers, &out)
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}
	if !res.OK() || !out.Result.ok() || out.RedirectURL == "" {
		return core.PurchaseResponse{}, failure("purchase", out.Result, "digipay rejected the ticket request")
	}

	return core.PurchaseResponse{
		Token:    out.Ticket,
		OrderID:  req.OrderID,
		Amount:   req.Amount,
		Raw:      res.Body,
		Extra:    map[string]string{TicketTypeKey: strconv.Itoa(ticketType)},
		Redirect: core.Redirect{Method: http.MethodGet, URL: out.RedirectURL},
	}, nil
}

// Verify settles a completed payment. The tracking code comes from the
// callback, so build the request with [core.Callback.VerifyRequest] rather
// than from the purchase token.
func (g *Gateway) Verify(ctx context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	trackingCode := req.Token
	if code := req.Get(TrackingCodeKey); code != "" {
		trackingCode = code
	}
	if trackingCode == "" {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrInvalidRequest).
			WithMessage("tracking code is required")
	}

	endpoint := transport.JoinURL(g.baseURL, verifyPath+trackingCode) +
		"?type=" + g.requestTicketType(req.Extra)

	var out verifyResponse
	res, err := g.auth.JSON(ctx, http.MethodPost, endpoint, nil, nil, &out)
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	if !res.OK() || !out.Result.ok() {
		return core.VerifyResponse{}, failure("verify", out.Result, "digipay did not verify the payment")
	}

	var reported core.Money
	if parsed, convErr := strconv.ParseInt(out.Amount.String(), 10, 64); convErr == nil {
		reported = core.Rial(parsed)
	}
	amount, err := core.SettledAmount(Name, req.Amount, reported)
	if err != nil {
		return core.VerifyResponse{}, err
	}
	card := out.CardNumber
	if card == "" {
		card = req.CardNumber
	}
	reference := out.RRN
	if reference == "" {
		reference = req.ReferenceNumber
	}

	return core.VerifyResponse{
		ReferenceNumber: reference,
		TransactionID:   out.TrackingCode,
		OrderID:         req.OrderID,
		CardNumber:      card,
		Amount:          amount,
		Raw:             res.Body,
	}, nil
}

// Refund returns money to the payer. [core.RefundRequest.OrderID] is used as
// the unique providerId of the refund itself, so it must differ from the
// provider id of the purchase.
func (g *Gateway) Refund(ctx context.Context, req core.RefundRequest) (core.RefundResponse, error) {
	saleTrackingCode := firstNonEmpty(req.TransactionID, req.ReferenceNumber, req.Token)
	switch {
	case saleTrackingCode == "":
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrInvalidRequest).
			WithMessage("transaction id (the purchase tracking code) is required")
	case req.OrderID == "":
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrInvalidRequest).
			WithMessage("order id is required; Digipay uses it as the unique refund providerId")
	case req.Amount.Rial() <= 0:
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrInvalidRequest).
			WithMessage("amount is required; Digipay has no full-refund shortcut")
	}

	endpoint := transport.JoinURL(g.baseURL, refundPath) + "?type=" + g.requestTicketType(req.Extra)
	body := refundRequest{
		ProviderID:       req.OrderID,
		Amount:           req.Amount.Rial(),
		SaleTrackingCode: saleTrackingCode,
	}

	var out refundResponse
	res, err := g.auth.NoRetry().JSON(ctx, http.MethodPost, endpoint, body, nil, &out)
	if err != nil {
		return core.RefundResponse{}, core.NewError(Name, "refund", err)
	}
	if !res.OK() || !out.Result.ok() {
		return core.RefundResponse{}, failure("refund", out.Result, "digipay rejected the refund")
	}

	return core.RefundResponse{RefundID: out.TrackingCode, Amount: req.Amount, Raw: res.Body}, nil
}

// Reverse cancels a payment within the window Digipay allows (about
// twenty-five minutes). Afterwards use [Gateway.Refund].
func (g *Gateway) Reverse(ctx context.Context, trackingCode, providerID string, ticketType int) error {
	if trackingCode == "" || providerID == "" {
		return core.NewError(Name, "reverse", core.ErrInvalidRequest).
			WithMessage("tracking code and provider id are required")
	}

	endpoint := transport.JoinURL(g.baseURL, reversePath) + "?type=" + strconv.Itoa(ticketType)
	var out reverseResponse
	res, err := g.auth.JSON(ctx, http.MethodPost, endpoint,
		reverseRequest{TrackingCode: trackingCode, ProviderID: providerID}, nil, &out)
	if err != nil {
		return core.NewError(Name, "reverse", err)
	}
	if !res.OK() || !out.Result.ok() {
		return failure("reverse", out.Result, "digipay rejected the reversal")
	}
	return nil
}

// DeliverRequest describes the shipment of a credit or BNPL order. Digipay
// only starts collecting the instalments once the goods are reported as
// delivered.
type DeliverRequest struct {
	// TrackingCode is the tracking code of the verified purchase.
	TrackingCode string
	// InvoiceNumber is the merchant's invoice number.
	InvoiceNumber string
	// DeliveryDate is the date the goods were handed over.
	DeliveryDate time.Time
	// Products are the product identifiers that were delivered.
	Products []string
	// TicketType is the product the purchase was paid with; it must be
	// [TypeCredit] or [TypeBNPL].
	TicketType int
}

// Deliver reports a credit or BNPL order as shipped.
func (g *Gateway) Deliver(ctx context.Context, req DeliverRequest) error {
	switch {
	case req.TrackingCode == "" || req.InvoiceNumber == "":
		return core.NewError(Name, "deliver", core.ErrInvalidRequest).
			WithMessage("tracking code and invoice number are required")
	case len(req.Products) == 0:
		return core.NewError(Name, "deliver", core.ErrInvalidRequest).
			WithMessage("at least one product is required")
	case req.TicketType != TypeCredit && req.TicketType != TypeBNPL:
		return core.NewError(Name, "deliver", core.ErrInvalidRequest).
			WithMessage("delivery is only defined for the credit and BNPL products")
	}

	deliveryDate := req.DeliveryDate
	if deliveryDate.IsZero() {
		deliveryDate = time.Now()
	}
	endpoint := transport.JoinURL(g.baseURL, deliverPath) + "?type=" + strconv.Itoa(req.TicketType)

	var out deliverResponse
	res, err := g.auth.JSON(ctx, http.MethodPost, endpoint, deliverRequest{
		DeliveryDate:  deliveryDate.Format(time.DateOnly),
		InvoiceNumber: req.InvoiceNumber,
		TrackingCode:  req.TrackingCode,
		Products:      req.Products,
	}, nil, &out)
	if err != nil {
		return core.NewError(Name, "deliver", err)
	}
	if !res.OK() || !out.Result.ok() {
		return failure("deliver", out.Result, "digipay rejected the delivery report")
	}
	return nil
}

// RefundStatus reads the state of a refund. Digipay has no purchase inquiry,
// which is why [core.Gateway.Inquiry] reports [core.ErrNotSupported].
func (g *Gateway) RefundStatus(ctx context.Context, refundID string, ticketType int) (core.InquiryResponse, error) {
	if refundID == "" {
		return core.InquiryResponse{}, core.NewError(Name, "refund-status", core.ErrInvalidRequest).
			WithMessage("refund id is required")
	}

	endpoint := transport.JoinURL(g.baseURL, refundPath+"/"+refundID) + "?type=" + strconv.Itoa(ticketType)
	var out refundStatusResponse
	res, err := g.auth.JSON(ctx, http.MethodPost, endpoint, nil, nil, &out)
	if err != nil {
		return core.InquiryResponse{}, core.NewError(Name, "refund-status", err)
	}
	if !res.OK() || !out.Result.ok() {
		return core.InquiryResponse{}, failure("refund-status", out.Result, "digipay rejected the refund inquiry")
	}

	status := core.StatusUnknown
	switch out.Status.String() {
	case "0":
		status = core.StatusRefunded
	case "1":
		status = core.StatusFailed
	case "2":
		status = core.StatusPending
	}

	return core.InquiryResponse{
		Status:  status,
		Message: out.Result.message(""),
		Raw:     res.Body,
	}, nil
}

// ParseCallback reads the form Digipay posts back to the merchant. The token
// of the returned callback is the tracking code, which is what verification
// keys on.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}

	outcome := core.FirstValue(values, "result", "status")
	amount := core.Money{}
	if parsed, err := strconv.ParseInt(core.FirstValue(values, "amount"), 10, 64); err == nil {
		amount = core.Rial(parsed)
	}

	return core.Callback{
		Gateway:         Name,
		Succeeded:       strings.EqualFold(outcome, "SUCCESS"),
		Token:           core.FirstValue(values, TrackingCodeKey, "ticket"),
		OrderID:         core.FirstValue(values, "providerId"),
		ReferenceNumber: core.FirstValue(values, "rrn"),
		Amount:          amount,
		Code:            outcome,
		Message:         core.FirstValue(values, "message"),
		Values:          values,
	}, nil
}

// fetchToken runs the OAuth password grant. Digipay expects the credentials as
// multipart form data rather than as a URL encoded body.
func (g *Gateway) fetchToken(ctx context.Context) (string, time.Duration, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for field, value := range map[string]string{
		"username":   g.cfg.Username,
		"password":   g.cfg.Password,
		"grant_type": "password",
	} {
		if err := writer.WriteField(field, value); err != nil {
			return "", 0, core.NewError(Name, "token", err)
		}
	}
	if err := writer.Close(); err != nil {
		return "", 0, core.NewError(Name, "token", err)
	}

	headers := map[string]string{
		"Content-Type":  writer.FormDataContentType(),
		"Authorization": "Basic " + basicAuth(g.cfg.MerchantID, g.cfg.MerchantKey),
		"Accept":        "application/json",
	}

	res, err := g.client.Do(ctx, http.MethodPost, transport.JoinURL(g.baseURL, oauthPath), body.Bytes(), headers)
	if err != nil {
		return "", 0, core.NewError(Name, "token", err)
	}

	var out oauthResponse
	if err := json.Unmarshal([]byte(res.Body), &out); err != nil || out.AccessToken == "" {
		message := out.Description
		if message == "" {
			message = "digipay refused the merchant credentials"
		}
		return "", 0, core.NewError(Name, "token", core.ErrInvalidConfig).
			WithCode(out.Error).WithMessage(message)
	}
	return out.AccessToken, time.Duration(out.ExpiresIn) * time.Second, nil
}

// ticketType returns the configured Digipay product.
func (g *Gateway) ticketType() int {
	if g.settings.ticketTypeSet {
		return g.settings.ticketType
	}
	return TypeWallet
}

// requestTicketType prefers the ticket type reported by the callback, so a
// payer who switched product inside the Digipay app is still verified against
// the product that actually paid.
func (g *Gateway) requestTicketType(extra map[string]string) string {
	if raw, ok := extra[TicketTypeKey]; ok && raw != "" {
		if _, err := strconv.Atoi(raw); err == nil {
			return raw
		}
	}
	return strconv.Itoa(g.ticketType())
}

// agent returns the Agent header value.
func (g *Gateway) agent() string {
	if g.settings.agent != "" {
		return g.settings.agent
	}
	return defaultAgent
}

// apiVersion returns the Digipay-Version header value.
func (g *Gateway) apiVersion() string {
	if g.settings.apiVersion != "" {
		return g.settings.apiVersion
	}
	return defaultAPIVersion
}

// failure turns a rejected result block into the package error type.
func failure(op string, res result, fallback string) error {
	return core.NewError(Name, op, core.ErrPaymentFailed).
		WithCode(res.Status.String()).
		WithMessage(res.message(fallback))
}

// firstNonEmpty returns the first non-empty string of its arguments.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// basicAuth builds the HTTP basic credential of the OAuth client.
func basicAuth(clientID, clientSecret string) string {
	return base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
}
