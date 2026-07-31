package virtual

import (
	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/gwopt"
)

// config holds the virtual gateway options.
type config struct {
	// declineAll makes every payment come back as declined, which is how a
	// failure path is exercised in tests.
	declineAll bool
	// redirectURL overrides the page the payer is sent to. Without it the
	// payer is sent straight back to the merchant callback URL.
	redirectURL string
	// failVerify makes verification fail, to exercise the recovery path.
	failVerify bool
}

// settings returns the virtual gateway option state of o.
func settings(o *core.Options) *config { return gwopt.From[config](o, string(Name)) }

// WithDecline makes every payment come back declined.
func WithDecline(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).declineAll = enabled }
}

// WithRedirectURL sends the payer to a page of your own instead of straight
// back to the callback URL, which is useful when you want to render a fake
// bank page in development.
func WithRedirectURL(redirectURL string) core.Option {
	return func(o *core.Options) { settings(o).redirectURL = redirectURL }
}

// WithFailingVerify makes [Gateway.Verify] fail, so the caller's recovery path
// can be tested.
func WithFailingVerify(enabled bool) core.Option {
	return func(o *core.Options) { settings(o).failVerify = enabled }
}
