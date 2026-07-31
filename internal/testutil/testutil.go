// Package testutil holds the fake gateway server the package's own tests are
// written against. It is internal and never reaches an application build.
package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Routes maps a request path to the handler answering it.
type Routes map[string]http.HandlerFunc

// NewServer starts a fake gateway serving routes. Unknown paths fail the test,
// which turns a wrong endpoint into an immediate, readable failure.
func NewServer(t *testing.T, routes Routes) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	for path, handler := range routes {
		mux.HandleFunc(path, handler)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected call to %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// JSON answers with a canned body and HTTP 200.
func JSON(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
}

// Status answers with a canned body and the given status code.
func Status(code int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	}
}

// XML answers with a canned XML body and HTTP 200.
func XML(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = io.WriteString(w, body)
	}
}

// Capture records the decoded JSON body of a request into target and then
// answers with the canned body.
func Capture(t *testing.T, target any, body string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if target != nil {
			if err := json.NewDecoder(r.Body).Decode(target); err != nil {
				t.Errorf("decoding request body of %s: %v", r.URL.Path, err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
}

// CaptureForm records the form values of a request and answers with the canned
// body.
func CaptureForm(t *testing.T, target *map[string]string, body string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parsing form of %s: %v", r.URL.Path, err)
		}
		values := map[string]string{}
		for key, list := range r.PostForm {
			if len(list) > 0 {
				values[key] = list[0]
			}
		}
		if target != nil {
			*target = values
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
}

// SOAPBody wraps a payload in a SOAP envelope, so a SOAP gateway can be
// answered without hand writing the envelope in every test.
func SOAPBody(payload string) http.HandlerFunc {
	return XML(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">
  <soap:Body>` + payload + `</soap:Body>
</soap:Envelope>`)
}
