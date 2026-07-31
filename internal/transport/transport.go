// Package transport is the HTTP plumbing shared by every gateway: JSON, form
// and raw calls with timeout, retry, logging and header handling.
//
// It is internal on purpose. Applications configure it through
// [github.com/amiranmanesh/payvand/core.Options] and never import it.
package transport

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/amiranmanesh/payvand/core"
)

// maxLoggedBody bounds how much of a body is handed to the logger, so a broken
// gateway cannot flood the application logs.
const maxLoggedBody = 4 << 10

// Client performs the outgoing HTTP calls of a gateway.
type Client struct {
	// opts is the resolved option set of the owning gateway.
	opts *core.Options
	// doer is the underlying HTTP client.
	doer core.Doer
}

// New builds a client from the resolved options. When the caller did not
// supply an HTTP client, a dedicated one is created with the configured
// timeout and TLS settings.
func New(opts *core.Options) *Client {
	c := &Client{opts: opts, doer: opts.HTTPClient}
	if c.doer == nil {
		httpClient := &http.Client{Timeout: opts.Timeout}
		if opts.SkipTLSVerify {
			httpClient.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // opt-in only
			}
		}
		c.doer = httpClient
	}
	return c
}

// Response is the outcome of one call, kept as raw text so gateways can put it
// in [core.PurchaseResponse.Raw] for support and auditing.
type Response struct {
	// StatusCode is the HTTP status code.
	StatusCode int
	// Body is the raw response body.
	Body string
	// Header is the response header.
	Header http.Header
}

// OK reports whether the HTTP status is in the 2xx range.
func (r Response) OK() bool { return r.StatusCode >= 200 && r.StatusCode < 300 }

// JSON sends body as JSON and decodes the response into out when out is not
// nil. A non-2xx status is returned to the caller instead of being turned into
// an error, because several Iranian gateways carry their business errors in
// the body of a 4xx response.
func (c *Client) JSON(ctx context.Context, method, endpoint string, body any, headers map[string]string, out any) (Response, error) {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return Response{}, fmt.Errorf("payvand: encoding request body: %w", err)
		}
	}

	merged := map[string]string{"Content-Type": "application/json", "Accept": "application/json"}
	for k, v := range headers {
		merged[k] = v
	}

	res, err := c.Do(ctx, method, endpoint, payload, merged)
	if err != nil {
		return res, err
	}
	if out != nil && strings.TrimSpace(res.Body) != "" {
		if err := json.Unmarshal([]byte(res.Body), out); err != nil {
			return res, fmt.Errorf("%w: %s", core.ErrUnexpectedResponse, truncate(res.Body))
		}
	}
	return res, nil
}

// Form sends values as application/x-www-form-urlencoded and decodes a JSON
// response into out when out is not nil.
func (c *Client) Form(ctx context.Context, endpoint string, values url.Values, headers map[string]string, out any) (Response, error) {
	merged := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "application/json",
	}
	for k, v := range headers {
		merged[k] = v
	}

	res, err := c.Do(ctx, http.MethodPost, endpoint, []byte(values.Encode()), merged)
	if err != nil {
		return res, err
	}
	if out != nil && strings.TrimSpace(res.Body) != "" {
		if err := json.Unmarshal([]byte(res.Body), out); err != nil {
			return res, fmt.Errorf("%w: %s", core.ErrUnexpectedResponse, truncate(res.Body))
		}
	}
	return res, nil
}

// Do performs a single call with the configured retry policy and returns the
// raw response.
func (c *Client) Do(ctx context.Context, method, endpoint string, body []byte, headers map[string]string) (Response, error) {
	attempts := c.opts.Retry.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	backoff := c.opts.Retry.Backoff

	var lastRes Response
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		lastRes, lastErr = c.do(ctx, method, endpoint, body, headers)
		if lastErr == nil && lastRes.StatusCode < http.StatusInternalServerError {
			return lastRes, nil
		}
		if attempt == attempts {
			break
		}
		select {
		case <-ctx.Done():
			return lastRes, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	return lastRes, lastErr
}

// do performs exactly one HTTP round trip.
func (c *Client) do(ctx context.Context, method, endpoint string, body []byte, headers map[string]string) (Response, error) {
	ctx, cancel := context.WithTimeout(ctx, c.opts.Timeout)
	defer cancel()

	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return Response{}, fmt.Errorf("payvand: building request: %w", err)
	}
	for k, v := range c.opts.Headers {
		req.Header.Set(k, v)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if c.opts.UserAgent != "" {
		req.Header.Set("User-Agent", c.opts.UserAgent)
	}

	fields := map[string]string{"method": method, "url": endpoint, "request": truncate(string(body))}
	started := time.Now()

	res, err := c.doer.Do(req)
	if err != nil {
		c.opts.Logger.Error(ctx, "payvand: gateway call failed", err, fields)
		return Response{}, fmt.Errorf("payvand: calling %s: %w", endpoint, err)
	}
	defer func() { _ = res.Body.Close() }()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		c.opts.Logger.Error(ctx, "payvand: reading gateway response failed", err, fields)
		return Response{StatusCode: res.StatusCode, Header: res.Header}, fmt.Errorf("payvand: reading response of %s: %w", endpoint, err)
	}

	fields["status"] = strconv.Itoa(res.StatusCode)
	fields["duration_ms"] = strconv.FormatInt(time.Since(started).Milliseconds(), 10)
	fields["response"] = truncate(string(raw))
	c.opts.Logger.Debug(ctx, "payvand: gateway call", fields)

	return Response{StatusCode: res.StatusCode, Body: string(raw), Header: res.Header}, nil
}

// truncate shortens a body for logging.
func truncate(s string) string {
	if len(s) <= maxLoggedBody {
		return s
	}
	return s[:maxLoggedBody] + "…"
}

// JoinURL concatenates a base URL and a path without doubling the slash. It is
// how gateways honour [core.Options.BaseURL] overrides.
func JoinURL(base, path string) string {
	if base == "" {
		return path
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}
