package core

// Client is the initialised entry point of the package: it remembers the
// options shared by the whole application (HTTP client, timeout, logger,
// retries) so that every gateway created from it inherits them.
//
//	pv := payvand.Init(payvand.WithTimeout(20 * time.Second))
//	gw, err := pv.Gateway(payvand.Zarinpal, payvand.Config{MerchantKey: key})
type Client struct {
	// defaults are the options applied before the per-gateway ones.
	defaults []Option
}

// Init creates a [Client] carrying the given default options.
func Init(opts ...Option) *Client {
	return &Client{defaults: opts}
}

// Gateway builds the named gateway with the client defaults, overridden by the
// options given here.
func (c *Client) Gateway(name Name, cfg Config, opts ...Option) (Gateway, error) {
	return New(name, cfg, c.merge(opts)...)
}

// MustGateway is [Client.Gateway] for the wiring phase of a program: it panics
// instead of returning an error, which is what you want when a misconfigured
// terminal must stop the process from starting.
func (c *Client) MustGateway(name Name, cfg Config, opts ...Option) Gateway {
	gw, err := c.Gateway(name, cfg, opts...)
	if err != nil {
		panic(err)
	}
	return gw
}

// With returns a copy of the client with additional default options.
func (c *Client) With(opts ...Option) *Client {
	return &Client{defaults: c.merge(opts)}
}

// merge concatenates the client defaults with call level options. Later
// options win because they are applied last.
func (c *Client) merge(opts []Option) []Option {
	merged := make([]Option, 0, len(c.defaults)+len(opts))
	merged = append(merged, c.defaults...)
	merged = append(merged, opts...)
	return merged
}
