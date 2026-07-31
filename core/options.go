package core

import (
	"net/http"
	"time"
)

// DefaultTimeout is the per-request timeout used when the caller does not set
// one. Iranian PSP endpoints are slow, hence the generous default.
const DefaultTimeout = 30 * time.Second

// DefaultUserAgent is sent with every request unless overridden.
const DefaultUserAgent = "payvand-go/1.0"

// Doer is the subset of [http.Client] Payvand needs. Supplying your own
// implementation is how you plug in tracing, mocking or a proxy.
type Doer interface {
	// Do executes an HTTP request and returns its response.
	Do(req *http.Request) (*http.Response, error)
}

// RetryPolicy controls how transport level failures are retried. Only
// idempotent-by-design gateway calls (token creation and verification are
// keyed by order id or token) are retried, and only on network errors or 5xx
// responses.
type RetryPolicy struct {
	// MaxAttempts is the total number of attempts, including the first one.
	// Values below one disable retrying.
	MaxAttempts int
	// Backoff is the pause between attempts. It is doubled after every retry.
	Backoff time.Duration
}

// Options holds every setting shared by all gateways. Build it with [Option]
// values; the zero value is completed by [NewOptions].
type Options struct {
	// HTTPClient performs the outgoing calls.
	HTTPClient Doer
	// Timeout bounds a single gateway call.
	Timeout time.Duration
	// Logger receives request/response events.
	Logger Logger
	// Sandbox switches gateways that offer a test environment to it.
	Sandbox bool
	// BaseURL overrides the provider API host. It exists for sandboxes,
	// on-premise deployments and tests; leave it empty in production.
	BaseURL string
	// UserAgent is sent as the User-Agent header.
	UserAgent string
	// Headers are extra headers added to every request.
	Headers map[string]string
	// Retry is the transport retry policy.
	Retry RetryPolicy
	// SkipTLSVerify disables certificate verification. Some Shaparak hosts
	// still serve incomplete chains; keep it false unless you must.
	SkipTLSVerify bool

	// extra carries gateway specific option state, keyed by gateway name.
	extra map[string]any
}

// Option mutates [Options]. Gateway packages expose their own typed options
// with this same signature, so transport and gateway settings compose in one
// variadic list.
type Option func(*Options)

// NewOptions applies opts on top of the defaults and returns the result.
func NewOptions(opts ...Option) *Options {
	o := &Options{
		Timeout:   DefaultTimeout,
		Logger:    NopLogger{},
		UserAgent: DefaultUserAgent,
		Retry:     RetryPolicy{MaxAttempts: 1},
		extra:     map[string]any{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.Logger == nil {
		o.Logger = NopLogger{}
	}
	if o.extra == nil {
		o.extra = map[string]any{}
	}
	return o
}

// Extra returns the gateway specific state stored under key, or nil.
func (o *Options) Extra(key string) any {
	if o.extra == nil {
		return nil
	}
	return o.extra[key]
}

// SetExtra stores gateway specific state under key. Gateway packages call it
// from their own options; application code never needs it.
func (o *Options) SetExtra(key string, value any) {
	if o.extra == nil {
		o.extra = map[string]any{}
	}
	o.extra[key] = value
}

// WithHTTPClient sets the HTTP client used for every call.
func WithHTTPClient(client Doer) Option {
	return func(o *Options) { o.HTTPClient = client }
}

// WithTimeout bounds a single gateway call.
func WithTimeout(d time.Duration) Option {
	return func(o *Options) { o.Timeout = d }
}

// WithLogger installs a logger.
func WithLogger(l Logger) Option {
	return func(o *Options) { o.Logger = l }
}

// WithSandbox switches gateways that have a test environment to it.
func WithSandbox(enabled bool) Option {
	return func(o *Options) { o.Sandbox = enabled }
}

// WithBaseURL overrides the provider API host, mainly for tests and sandboxes.
func WithBaseURL(baseURL string) Option {
	return func(o *Options) { o.BaseURL = baseURL }
}

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(o *Options) { o.UserAgent = ua }
}

// WithHeader adds a header sent with every request.
func WithHeader(key, value string) Option {
	return func(o *Options) {
		if o.Headers == nil {
			o.Headers = map[string]string{}
		}
		o.Headers[key] = value
	}
}

// WithRetry enables transport level retrying.
func WithRetry(maxAttempts int, backoff time.Duration) Option {
	return func(o *Options) { o.Retry = RetryPolicy{MaxAttempts: maxAttempts, Backoff: backoff} }
}

// WithSkipTLSVerify disables TLS certificate verification.
func WithSkipTLSVerify(skip bool) Option {
	return func(o *Options) { o.SkipTLSVerify = skip }
}
