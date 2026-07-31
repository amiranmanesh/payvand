package transport_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/amiranmanesh/payvand/core"
	"github.com/amiranmanesh/payvand/internal/transport"
)

func TestJSONSendsHeadersAndDecodes(t *testing.T) {
	var gotUserAgent, gotCustom, gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgent = r.Header.Get("User-Agent")
		gotCustom = r.Header.Get("X-Terminal")
		gotContentType = r.Header.Get("Content-Type")
		_, _ = w.Write([]byte(`{"token":"tok-1"}`))
	}))
	defer server.Close()

	client := transport.New(core.NewOptions(
		core.WithUserAgent("payvand-test"),
		core.WithHeader("X-Terminal", "1234"),
	))

	var out struct {
		Token string `json:"token"`
	}
	res, err := client.JSON(context.Background(), http.MethodPost, server.URL, map[string]string{"a": "b"}, nil, &out)
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	switch {
	case !res.OK():
		t.Errorf("status = %d", res.StatusCode)
	case out.Token != "tok-1":
		t.Errorf("token = %q", out.Token)
	case gotUserAgent != "payvand-test" || gotCustom != "1234" || gotContentType != "application/json":
		t.Errorf("headers = %q / %q / %q", gotUserAgent, gotCustom, gotContentType)
	}
}

func TestJSONReportsUnreadableBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html>maintenance</html>`))
	}))
	defer server.Close()

	client := transport.New(core.NewOptions())
	var out struct{ Token string }
	if _, err := client.JSON(context.Background(), http.MethodGet, server.URL, nil, nil, &out); err == nil {
		t.Fatal("an unparsable body must be reported")
	}
}

func TestRetryOnServerError(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := transport.New(core.NewOptions(core.WithRetry(3, time.Millisecond)))
	res, err := client.Do(context.Background(), http.MethodGet, server.URL, nil, nil)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if calls != 3 || !res.OK() {
		t.Fatalf("calls = %d, status = %d", calls, res.StatusCode)
	}
}

func TestNoRetryOnClientError(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := transport.New(core.NewOptions(core.WithRetry(3, time.Millisecond)))
	if _, err := client.Do(context.Background(), http.MethodGet, server.URL, nil, nil); err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want a 4xx not to be retried", calls)
	}
}

func TestFormEncodesValues(t *testing.T) {
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got = r.PostForm
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	client := transport.New(core.NewOptions())
	var out struct {
		Status int `json:"status"`
	}
	if _, err := client.Form(context.Background(), server.URL, url.Values{"api": {"key"}}, nil, &out); err != nil {
		t.Fatalf("Form() error = %v", err)
	}
	if got.Get("api") != "key" || out.Status != 1 {
		t.Fatalf("form = %v, out = %+v", got, out)
	}
}

func TestTimeoutIsHonoured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := transport.New(core.NewOptions(core.WithTimeout(5 * time.Millisecond)))
	if _, err := client.Do(context.Background(), http.MethodGet, server.URL, nil, nil); err == nil {
		t.Fatal("a call slower than the timeout must fail")
	}
}

func TestJoinURL(t *testing.T) {
	cases := []struct{ base, path, want string }{
		{"https://api.example", "/v1/pay", "https://api.example/v1/pay"},
		{"https://api.example/", "v1/pay", "https://api.example/v1/pay"},
		{"", "https://api.example/v1/pay", "https://api.example/v1/pay"},
	}
	for _, tc := range cases {
		if got := transport.JoinURL(tc.base, tc.path); got != tc.want {
			t.Errorf("JoinURL(%q, %q) = %q, want %q", tc.base, tc.path, got, tc.want)
		}
	}
}

// recordingLogger keeps the fields of every event, so a test can assert on
// what a real logging pipeline would have received.
type recordingLogger struct {
	events []map[string]string
}

func (l *recordingLogger) Debug(_ context.Context, _ string, fields map[string]string) {
	l.events = append(l.events, fields)
}

func (l *recordingLogger) Error(_ context.Context, _ string, _ error, fields map[string]string) {
	l.events = append(l.events, fields)
}

// contains reports whether any recorded field holds the given text.
func (l *recordingLogger) contains(text string) bool {
	for _, fields := range l.events {
		for _, value := range fields {
			if strings.Contains(value, text) {
				return true
			}
		}
	}
	return false
}

func TestLoggingHidesTheTerminalCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"token":"tok-1"}`))
	}))
	defer server.Close()

	logger := &recordingLogger{}
	client := transport.New(core.NewOptions(core.WithLogger(logger)))

	body := map[string]string{"username": "shop", "password": "s3cret", "orderId": "1001"}
	if _, err := client.JSON(context.Background(), http.MethodPost, server.URL, body, nil, nil); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}

	if logger.contains("s3cret") {
		t.Error("the terminal password reached the logger")
	}
	if !logger.contains("1001") {
		t.Error("the order id must survive, or the log is useless for support")
	}
}

func TestLoggingHidesCredentialsOnAFailedCall(t *testing.T) {
	logger := &recordingLogger{}
	client := transport.New(core.NewOptions(core.WithLogger(logger)))

	body := map[string]string{"password": "s3cret"}
	// An unroutable host fails the round trip, which is the path that logs at
	// error level — the one a production incident actually goes through.
	_, err := client.JSON(context.Background(), http.MethodPost, "http://127.0.0.1:1/pay", body, nil, nil)
	if err == nil {
		t.Fatal("the call was expected to fail")
	}
	if logger.contains("s3cret") {
		t.Error("the terminal password reached the logger on the error path")
	}
}

func TestNoRetryCallsExactlyOnce(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	options := core.NewOptions(core.WithRetry(3, time.Millisecond))
	client := transport.New(options)

	if _, err := client.Do(context.Background(), http.MethodPost, server.URL, nil, nil); err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want the retry policy to apply", calls)
	}

	calls = 0
	if _, err := client.NoRetry().Do(context.Background(), http.MethodPost, server.URL, nil, nil); err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want a single attempt", calls)
	}
}

func TestNoRetryLeavesTheOriginalAlone(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := transport.New(core.NewOptions(core.WithRetry(2, time.Millisecond)))
	_ = client.NoRetry()

	if _, err := client.Do(context.Background(), http.MethodPost, server.URL, nil, nil); err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want the original client to keep retrying", calls)
	}
}
