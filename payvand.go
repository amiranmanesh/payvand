// Package payvand is a dependency-free Go client for the Iranian internet
// payment gateways (IPG).
//
// Every provider — bank acquirers and PSPs alike — is reached through one
// interface, [Gateway], so the call sites of an application never change when
// the provider does. Choosing a provider is choosing a value:
//
//	pv := payvand.Init(payvand.WithTimeout(20 * time.Second))
//
//	gw, err := pv.Gateway(payvand.Zarinpal, payvand.Config{MerchantKey: merchantID})
//	if err != nil {
//	    return err
//	}
//
//	purchase, err := gw.Purchase(ctx, payvand.PurchaseRequest{
//	    Amount:      payvand.Toman(15_000),
//	    OrderID:     "10245",
//	    CallbackURL: "https://shop.example/payments/callback",
//	})
//	if err != nil {
//	    return err
//	}
//	purchase.Redirect.Send(w, r) // GET redirect or auto-posting form
//
// After the payer returns, the callback is parsed and the payment verified:
//
//	callback, _ := gw.ParseCallback(r)
//	verified, err := gw.Verify(ctx, callback.VerifyRequest(payvand.Toman(15_000)))
//
// Swapping Zarinpal for Mellat, Parsian or the in-memory [payvand.Virtual]
// gateway changes the first line and nothing else.
//
// Provider specific behaviour is opt-in through options declared by the
// gateway packages, and composes with the shared ones:
//
//	gw, err := pv.Gateway(payvand.Zibal, cfg,
//	    payvand.WithSandbox(true),
//	    zibal.WithFeeMode(1),
//	    zibal.WithMultiplexing(zibal.Share{BankAccount: iban, Amount: 50_000}),
//	)
//
// The package imports nothing outside the Go standard library.
package payvand

import (
	"time"

	"github.com/amiranmanesh/payvand/core"

	// Linking every gateway keeps payvand.New usable with a name that comes
	// from configuration or from a database row.
	"github.com/amiranmanesh/payvand/gateway/asanpardakht"
	"github.com/amiranmanesh/payvand/gateway/bitpay"
	"github.com/amiranmanesh/payvand/gateway/digipay"
	"github.com/amiranmanesh/payvand/gateway/idpay"
	"github.com/amiranmanesh/payvand/gateway/irankish"
	"github.com/amiranmanesh/payvand/gateway/jibit"
	"github.com/amiranmanesh/payvand/gateway/mellat"
	"github.com/amiranmanesh/payvand/gateway/nextpay"
	"github.com/amiranmanesh/payvand/gateway/parsian"
	"github.com/amiranmanesh/payvand/gateway/pasargad"
	"github.com/amiranmanesh/payvand/gateway/payir"
	"github.com/amiranmanesh/payvand/gateway/payping"
	"github.com/amiranmanesh/payvand/gateway/payweb"
	"github.com/amiranmanesh/payvand/gateway/sadad"
	"github.com/amiranmanesh/payvand/gateway/saman"
	"github.com/amiranmanesh/payvand/gateway/sepehr"
	"github.com/amiranmanesh/payvand/gateway/snapppay"
	"github.com/amiranmanesh/payvand/gateway/tara"
	"github.com/amiranmanesh/payvand/gateway/top"
	"github.com/amiranmanesh/payvand/gateway/torobpay"
	"github.com/amiranmanesh/payvand/gateway/vandar"
	"github.com/amiranmanesh/payvand/gateway/virtual"
	"github.com/amiranmanesh/payvand/gateway/yekpay"
	"github.com/amiranmanesh/payvand/gateway/zarinpal"
	"github.com/amiranmanesh/payvand/gateway/zibal"
)

// Names of the supported gateways. Pass one to [Client.Gateway] or [New].
const (
	// AsanPardakht is the AsanPardakht PSP (REST v1).
	AsanPardakht = asanpardakht.Name
	// BitPay is the BitPay.ir aggregator.
	BitPay = bitpay.Name
	// DigiPay is the Digipay wallet, credit and BNPL gateway.
	DigiPay = digipay.Name
	// IDPay is the IDPay PSP.
	IDPay = idpay.Name
	// IranKish is the Iran Kish acquirer (Bank Kar Afarin group).
	IranKish = irankish.Name
	// Jibit is the Jibit proxy payment gateway.
	Jibit = jibit.Name
	// Mellat is the Behpardakht Mellat acquirer.
	Mellat = mellat.Name
	// NextPay is the NextPay PSP.
	NextPay = nextpay.Name
	// Parsian is the Parsian Bank acquirer.
	Parsian = parsian.Name
	// Pasargad is the Bank Pasargad acquirer.
	Pasargad = pasargad.Name
	// PayIr is the Pay.ir PSP.
	PayIr = payir.Name
	// PayPing is the PayPing PSP.
	PayPing = payping.Name
	// PayWeb is the PayWeb PSP.
	PayWeb = payweb.Name
	// Sadad is the Sadad / Bank Melli acquirer.
	Sadad = sadad.Name
	// Saman is the Saman Bank (SEP) acquirer.
	Saman = saman.Name
	// Sepehr is the Sepehr / Bank Saderat (Mabna) acquirer.
	Sepehr = sepehr.Name
	// SnappPay is the SnappPay online instalment (BNPL) gateway.
	SnappPay = snapppay.Name
	// Tara is the Tara club credit gateway.
	Tara = tara.Name
	// Top is the TOP (Taban Ati Pardaz) in-app gateway.
	Top = top.Name
	// TorobPay is the TorobPay online credit (BNPL) gateway.
	TorobPay = torobpay.Name
	// Vandar is the Vandar PSP.
	Vandar = vandar.Name
	// Virtual is the in-memory gateway used for development and tests.
	Virtual = virtual.Name
	// YekPay is the YekPay multi-currency PSP.
	YekPay = yekpay.Name
	// Zarinpal is the Zarinpal PSP.
	Zarinpal = zarinpal.Name
	// Zibal is the Zibal PSP.
	Zibal = zibal.Name
)

// Core types, re-exported so a normal application imports this package only.
type (
	// Gateway is the interface every provider implements.
	Gateway = core.Gateway
	// Name identifies a gateway in the registry.
	Name = core.Name
	// Config carries the credentials of one terminal.
	Config = core.Config
	// Capabilities describes what a gateway supports.
	Capabilities = core.Capabilities
	// Client is the initialised entry point holding shared options.
	Client = core.Client
	// Option configures a gateway.
	Option = core.Option
	// Options is the resolved option set handed to a gateway.
	Options = core.Options
	// Doer is the HTTP client contract.
	Doer = core.Doer
	// Logger receives request and response events.
	Logger = core.Logger
	// SlogLogger adapts a standard library slog logger.
	SlogLogger = core.SlogLogger
	// NopLogger is the logger that drops everything, used by default.
	NopLogger = core.NopLogger
	// Factory builds a gateway; supply one to [Register].
	Factory = core.Factory
	// Money is an amount plus the unit it is expressed in.
	Money = core.Money
	// Currency is the unit of a [Money].
	Currency = core.Currency
	// Status is the normalised state of a transaction.
	Status = core.Status
	// Redirect describes how to send the payer to the bank.
	Redirect = core.Redirect
	// Callback is the parsed return request of a gateway.
	Callback = core.Callback
	// PurchaseRequest is the input of [Gateway.Purchase].
	PurchaseRequest = core.PurchaseRequest
	// PurchaseResponse is the output of [Gateway.Purchase].
	PurchaseResponse = core.PurchaseResponse
	// VerifyRequest is the input of [Gateway.Verify].
	VerifyRequest = core.VerifyRequest
	// VerifyResponse is the output of [Gateway.Verify].
	VerifyResponse = core.VerifyResponse
	// RefundRequest is the input of [Gateway.Refund].
	RefundRequest = core.RefundRequest
	// RefundResponse is the output of [Gateway.Refund].
	RefundResponse = core.RefundResponse
	// InquiryRequest is the input of [Gateway.Inquiry].
	InquiryRequest = core.InquiryRequest
	// InquiryResponse is the output of [Gateway.Inquiry].
	InquiryResponse = core.InquiryResponse
	// Error is the rich error returned by every gateway.
	Error = core.Error
)

// Currency values.
const (
	// IRR is the Iranian Rial.
	IRR = core.IRR
	// IRT is the Iranian Toman.
	IRT = core.IRT
)

// Transaction statuses.
const (
	// StatusUnknown means the provider reported no mappable state.
	StatusUnknown = core.StatusUnknown
	// StatusPending means the payer has not finished yet.
	StatusPending = core.StatusPending
	// StatusPaid means the money was taken but not settled.
	StatusPaid = core.StatusPaid
	// StatusVerified means the payment is settled.
	StatusVerified = core.StatusVerified
	// StatusFailed means the payment failed.
	StatusFailed = core.StatusFailed
	// StatusCanceled means the payer aborted.
	StatusCanceled = core.StatusCanceled
	// StatusRefunded means the payment was returned.
	StatusRefunded = core.StatusRefunded
)

// Sentinel errors, comparable with errors.Is.
var (
	// ErrNotSupported is returned by an operation the provider lacks.
	ErrNotSupported = core.ErrNotSupported
	// ErrGatewayNotRegistered is returned for an unknown gateway name.
	ErrGatewayNotRegistered = core.ErrGatewayNotRegistered
	// ErrInvalidConfig is returned when credentials are missing.
	ErrInvalidConfig = core.ErrInvalidConfig
	// ErrInvalidRequest is returned for an unusable request.
	ErrInvalidRequest = core.ErrInvalidRequest
	// ErrPaymentFailed is returned when the provider rejected the payment.
	ErrPaymentFailed = core.ErrPaymentFailed
	// ErrPaymentCanceled is returned when the payer aborted.
	ErrPaymentCanceled = core.ErrPaymentCanceled
	// ErrAlreadyVerified is returned for a repeated verification.
	ErrAlreadyVerified = core.ErrAlreadyVerified
	// ErrVerificationPending is returned when the provider is still settling
	// the payment and Verify must be called again.
	ErrVerificationPending = core.ErrVerificationPending
	// ErrAmountMismatch is returned when the settled amount differs.
	ErrAmountMismatch = core.ErrAmountMismatch
	// ErrUnexpectedResponse is returned for an unreadable provider answer.
	ErrUnexpectedResponse = core.ErrUnexpectedResponse
)

// Init creates the [Client] every gateway is built from, carrying the options
// shared by the whole application.
func Init(opts ...Option) *Client { return core.Init(opts...) }

// New builds a gateway directly, without a [Client]. Use it for one-off jobs;
// long lived applications are better served by [Init].
func New(name Name, cfg Config, opts ...Option) (Gateway, error) {
	return core.New(name, cfg, opts...)
}

// MustNew is [New] for the wiring phase of a program: it panics instead of
// returning an error.
func MustNew(name Name, cfg Config, opts ...Option) Gateway {
	gw, err := core.New(name, cfg, opts...)
	if err != nil {
		panic(err)
	}
	return gw
}

// Register adds a gateway of your own to the registry, so an in-house provider
// is reachable by name exactly like the built-in ones.
func Register(name Name, factory core.Factory) { core.Register(name, factory) }

// Registered returns the sorted names of the linked gateways.
func Registered() []Name { return core.Registered() }

// IsRegistered reports whether a gateway is available.
func IsRegistered(name Name) bool { return core.IsRegistered(name) }

// Rial builds an amount expressed in Iranian Rial.
func Rial(amount int64) Money { return core.Rial(amount) }

// Toman builds an amount expressed in Iranian Toman.
func Toman(amount int64) Money { return core.Toman(amount) }

// SettledAmount reconciles the amount a provider reports for a payment with the
// amount that was ordered, returning an error wrapping [ErrAmountMismatch] when
// they disagree. Gateways apply it inside [Gateway.Verify]; it is exported for
// the same check against an [InquiryResponse].
func SettledAmount(gateway Name, requested, reported Money) (Money, error) {
	return core.SettledAmount(gateway, requested, reported)
}

// WithHTTPClient sets the HTTP client used for every call.
func WithHTTPClient(client Doer) Option { return core.WithHTTPClient(client) }

// WithTimeout bounds a single gateway call.
func WithTimeout(d time.Duration) Option { return core.WithTimeout(d) }

// WithLogger installs a logger.
func WithLogger(l Logger) Option { return core.WithLogger(l) }

// WithSandbox switches gateways that have a test environment to it.
func WithSandbox(enabled bool) Option { return core.WithSandbox(enabled) }

// WithBaseURL overrides the provider host, for sandboxes and tests.
func WithBaseURL(baseURL string) Option { return core.WithBaseURL(baseURL) }

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(ua string) Option { return core.WithUserAgent(ua) }

// WithHeader adds a header sent with every request.
func WithHeader(key, value string) Option { return core.WithHeader(key, value) }

// WithRetry enables transport level retrying.
func WithRetry(maxAttempts int, backoff time.Duration) Option {
	return core.WithRetry(maxAttempts, backoff)
}

// WithSkipTLSVerify disables TLS certificate verification. Only reach for it
// when a Shaparak host serves a chain your trust store cannot complete.
func WithSkipTLSVerify(skip bool) Option { return core.WithSkipTLSVerify(skip) }
