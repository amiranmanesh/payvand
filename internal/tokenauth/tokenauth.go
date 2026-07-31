// Package tokenauth caches the short lived bearer tokens the OAuth style
// gateways hand out, so a gateway value can be built once at start-up and then
// shared by every request without re-authenticating on each call.
//
// It is internal on purpose: applications never see it, they only observe that
// SnappPay, TorobPay, Digipay, Tara and Jibit authenticate lazily.
package tokenauth

import (
	"context"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/amiranmanesh/payvand/internal/transport"
)

// skew is subtracted from the reported lifetime so a token is never used in
// the last moments before the provider expires it.
const skew = 30 * time.Second

// Fetch obtains a fresh token. It returns the token itself and the lifetime
// the provider reported for it; a non-positive lifetime means "unknown", and
// the token is then treated as valid for [Cache.Fallback].
type Fetch func(ctx context.Context) (token string, lifetime time.Duration, err error)

// Cache holds one bearer token and hands it out until it expires. The zero
// value is ready to use and safe for concurrent use.
type Cache struct {
	// Fallback is the lifetime assumed when the provider reports none. It
	// defaults to five minutes, which is short enough to recover quickly from
	// a revoked token and long enough to keep a checkout from re-authenticating.
	Fallback time.Duration

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// Token returns the cached token, calling fetch when there is none or the
// cached one has expired. Concurrent callers share a single fetch.
func (c *Cache) Token(ctx context.Context, fetch Fetch) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.expiresAt) {
		return c.token, nil
	}

	token, lifetime, err := fetch(ctx)
	if err != nil {
		return "", err
	}
	if lifetime <= 0 {
		lifetime = c.Fallback
		if lifetime <= 0 {
			lifetime = 5 * time.Minute
		}
	}
	// A lifetime shorter than the safety margin is treated as already spent,
	// so the token is fetched again on the next call rather than used in its
	// last moments.
	if lifetime -= skew; lifetime < 0 {
		lifetime = 0
	}

	c.token, c.expiresAt = token, time.Now().Add(lifetime)
	return c.token, nil
}

// Invalidate drops the cached token, so the next [Cache.Token] authenticates
// again. Gateways call it when the provider answers "unauthorised", which is
// how a token revoked on the provider side is recovered from.
func (c *Cache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token, c.expiresAt = "", time.Time{}
}

// Client performs authenticated JSON calls on behalf of a gateway: it attaches
// the cached bearer token, and when the provider rejects it, renews the token
// and replays the call exactly once.
type Client struct {
	// Cache holds the token between calls.
	Cache *Cache
	// Transport performs the HTTP round trips.
	Transport *transport.Client
	// Fetch obtains a new token when the cache is empty or stale.
	Fetch Fetch
	// Expired reports whether a response means the token must be renewed. A
	// nil value falls back to plain HTTP 401 detection.
	Expired func(transport.Response) bool
}

// JSON sends an authenticated JSON request and decodes the answer into out.
func (c *Client) JSON(ctx context.Context, method, endpoint string, body any, headers map[string]string, out any) (transport.Response, error) {
	var (
		res transport.Response
		err error
	)
	for attempt := range 2 {
		var token string
		if token, err = c.Cache.Token(ctx, c.Fetch); err != nil {
			return transport.Response{}, err
		}

		merged := map[string]string{"Authorization": "Bearer " + token}
		for k, v := range headers {
			merged[k] = v
		}

		if res, err = c.Transport.JSON(ctx, method, endpoint, body, merged, out); err != nil {
			return res, err
		}
		if attempt == 0 && c.expired(res) {
			c.Cache.Invalidate()
			// The rejected answer already decoded into out; clear it so its
			// error fields cannot leak into the replayed response.
			reset(out)
			continue
		}
		return res, nil
	}
	return res, err
}

// expired applies the provider specific check, defaulting to HTTP 401.
func (c *Client) expired(res transport.Response) bool {
	if c.Expired != nil {
		return c.Expired(res)
	}
	return res.StatusCode == http.StatusUnauthorized
}

// reset zeroes the value behind a decoding target, so a replayed call starts
// from an empty struct instead of the fields the first answer left behind.
func reset(out any) {
	if out == nil {
		return
	}
	value := reflect.ValueOf(out)
	if value.Kind() == reflect.Pointer && !value.IsNil() {
		value.Elem().Set(reflect.Zero(value.Elem().Type()))
	}
}
