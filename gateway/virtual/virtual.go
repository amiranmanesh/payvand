// Package virtual implements an in-memory gateway for development and tests.
//
// It talks to nobody: payments live in a map, the payer is sent straight back
// to the callback URL, and the whole purchase/verify/refund cycle behaves like
// a real provider. Point your staging configuration at "virtual" and the rest
// of your code does not change.
package virtual

import (
	"context"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/cryptox"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// Name is the registry name of this gateway.
const Name core.Name = "virtual"

// Callback parameters the virtual gateway returns the payer with.
const (
	// TokenParam carries the payment token.
	TokenParam = "payvand_token"
	// StatusParam carries "OK" or "NOK".
	StatusParam = "payvand_status"
)

// CardNumber is the masked PAN reported by every virtual payment.
const CardNumber = "603799******1234"

// init registers the gateway.
func init() {
	core.Register(Name, func(cfg core.Config, opts ...core.Option) (core.Gateway, error) {
		return New(cfg, opts...)
	})
}

// payment is the in-memory state of one virtual transaction.
type payment struct {
	orderID   string
	amount    core.Money
	status    core.Status
	reference string
	createdAt time.Time
}

// Gateway is the in-memory implementation of [core.Gateway].
type Gateway struct {
	cfg      core.Config
	opts     *core.Options
	settings *config

	mu       sync.Mutex
	payments map[string]*payment
}

var _ core.Gateway = (*Gateway)(nil)

// New builds a virtual gateway. No credential is required.
func New(cfg core.Config, opts ...core.Option) (*Gateway, error) {
	options := core.NewOptions(opts...)
	return &Gateway{
		cfg:      cfg,
		opts:     options,
		settings: gwopt.From[config](options, string(Name)),
		payments: map[string]*payment{},
	}, nil
}

// Name returns the gateway name.
func (g *Gateway) Name() core.Name { return Name }

// Capabilities reports what the virtual gateway offers, which is everything.
func (g *Gateway) Capabilities() core.Capabilities {
	return core.Capabilities{
		Verify:         true,
		Refund:         true,
		Inquiry:        true,
		Callback:       true,
		RedirectMethod: http.MethodGet,
		Currencies:     []core.Currency{core.IRR, core.IRT},
	}
}

// Purchase records a payment and returns a redirect back to the callback URL.
func (g *Gateway) Purchase(_ context.Context, req core.PurchaseRequest) (core.PurchaseResponse, error) {
	if req.Amount.IsZero() {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", core.ErrInvalidRequest).
			WithMessage("amount must be greater than zero")
	}

	token, err := randomToken()
	if err != nil {
		return core.PurchaseResponse{}, core.NewError(Name, "purchase", err)
	}

	g.mu.Lock()
	g.payments[token] = &payment{
		orderID:   req.OrderID,
		amount:    req.Amount,
		status:    core.StatusPending,
		createdAt: time.Now(),
	}
	g.mu.Unlock()

	status := "OK"
	if g.settings.declineAll {
		status = "NOK"
	}
	target := g.settings.redirectURL
	if target == "" {
		target = req.CallbackURL
	}

	return core.PurchaseResponse{
		Token:   token,
		OrderID: req.OrderID,
		Amount:  req.Amount,
		Raw:     `{"gateway":"virtual"}`,
		Redirect: core.Redirect{
			Method: http.MethodGet,
			URL:    target,
			Params: map[string]string{TokenParam: token, StatusParam: status},
		},
	}, nil
}

// Verify settles a recorded payment.
func (g *Gateway) Verify(_ context.Context, req core.VerifyRequest) (core.VerifyResponse, error) {
	if g.settings.failVerify {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithMessage("the virtual gateway is configured to fail verification")
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	record, ok := g.payments[req.Token]
	if !ok {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithMessage("unknown token")
	}
	if g.settings.declineAll {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrPaymentFailed).
			WithMessage("the virtual gateway declined the payment")
	}
	if record.status == core.StatusVerified {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrAlreadyVerified)
	}
	if req.Amount.Rial() > 0 && req.Amount.Rial() != record.amount.Rial() {
		return core.VerifyResponse{}, core.NewError(Name, "verify", core.ErrAmountMismatch)
	}

	reference, err := randomToken()
	if err != nil {
		return core.VerifyResponse{}, core.NewError(Name, "verify", err)
	}
	record.status = core.StatusVerified
	record.reference = reference

	return core.VerifyResponse{
		ReferenceNumber: reference,
		TransactionID:   req.Token,
		OrderID:         record.orderID,
		CardNumber:      CardNumber,
		Amount:          record.amount,
		PaidAt:          time.Now(),
		Raw:             `{"gateway":"virtual","verified":true}`,
	}, nil
}

// Refund reverses a settled payment.
func (g *Gateway) Refund(_ context.Context, req core.RefundRequest) (core.RefundResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	token := req.Token
	if token == "" {
		token = req.TransactionID
	}
	record, ok := g.payments[token]
	if !ok {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrPaymentFailed).
			WithMessage("unknown token")
	}
	if record.status != core.StatusVerified {
		return core.RefundResponse{}, core.NewError(Name, "refund", core.ErrPaymentFailed).
			WithMessage("only a verified payment can be refunded")
	}
	record.status = core.StatusRefunded

	return core.RefundResponse{RefundID: record.reference, Amount: record.amount}, nil
}

// Inquiry reports the state of a recorded payment.
func (g *Gateway) Inquiry(_ context.Context, req core.InquiryRequest) (core.InquiryResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	record, ok := g.payments[req.Token]
	if !ok {
		return core.InquiryResponse{Status: core.StatusUnknown}, nil
	}
	return core.InquiryResponse{
		Status:          record.status,
		ReferenceNumber: record.reference,
		CardNumber:      CardNumber,
		Amount:          record.amount,
		Message:         record.status.String(),
	}, nil
}

// ParseCallback reads the parameters the virtual redirect carries.
func (g *Gateway) ParseCallback(r *http.Request) (core.Callback, error) {
	values, err := core.CallbackValues(r)
	if err != nil {
		return core.Callback{}, core.NewError(Name, "callback", err)
	}
	status := core.FirstValue(values, StatusParam)
	token := core.FirstValue(values, TokenParam)

	callback := core.Callback{
		Gateway:   Name,
		Succeeded: strings.EqualFold(status, "OK"),
		Token:     token,
		Code:      status,
		Values:    values,
	}
	g.mu.Lock()
	if record, ok := g.payments[token]; ok {
		callback.OrderID = record.orderID
		callback.Amount = record.amount
	}
	g.mu.Unlock()
	return callback, nil
}

// randomToken returns a 32 character hexadecimal identifier.
func randomToken() (string, error) {
	raw, err := cryptox.RandomBytes(16)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
