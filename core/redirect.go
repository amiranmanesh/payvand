package core

import (
	"html/template"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// Redirect describes how the payer's browser must reach the bank page.
//
// Some providers hand out a plain URL (GET), others require an HTML form to be
// posted (Mellat, Sepehr, AsanPardakht). Both cases are expressed here so the
// caller writes the same three lines for every gateway:
//
//	res, _ := gw.Purchase(ctx, req)
//	res.Redirect.Send(w, r)
type Redirect struct {
	// Method is http.MethodGet or http.MethodPost.
	Method string
	// URL is the bank page to open.
	URL string
	// Params are the form fields to post when Method is POST. They are
	// already appended to URL when Method is GET.
	Params map[string]string
}

// IsPost reports whether the payer must be sent with an HTTP POST.
func (r Redirect) IsPost() bool { return strings.EqualFold(r.Method, http.MethodPost) }

// String returns the redirect URL, with the parameters appended as a query
// string when the method is GET.
func (r Redirect) String() string {
	if r.IsPost() || len(r.Params) == 0 {
		return r.URL
	}
	u, err := url.Parse(r.URL)
	if err != nil {
		return r.URL
	}
	q := u.Query()
	for k, v := range r.Params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// redirectFormTemplate is the auto-submitting form used for POST redirects.
// It is parsed once at init time; the input is escaped by html/template.
var redirectFormTemplate = template.Must(template.New("payvand-redirect").Parse(
	`<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Redirecting to the payment gateway…</title></head>
<body onload="document.forms[0].submit()">
<form method="post" action="{{ .URL }}">
{{- range .Fields }}
<input type="hidden" name="{{ .Key }}" value="{{ .Value }}">
{{- end }}
<noscript><button type="submit">Continue to the payment gateway</button></noscript>
</form>
</body>
</html>
`))

// field is one hidden input of the auto-submitting redirect form.
type field struct {
	Key   string
	Value string
}

// HTML renders an auto-submitting HTML form for a POST redirect. For a GET
// redirect it renders a form as well, so the output is always usable, but
// callers normally use [Redirect.String] in that case.
func (r Redirect) HTML() (string, error) {
	var sb strings.Builder
	if err := r.WriteHTML(&sb); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// WriteHTML writes the auto-submitting redirect form to w.
func (r Redirect) WriteHTML(w io.Writer) error {
	keys := make([]string, 0, len(r.Params))
	for k := range r.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fields := make([]field, 0, len(keys))
	for _, k := range keys {
		fields = append(fields, field{Key: k, Value: r.Params[k]})
	}
	return redirectFormTemplate.Execute(w, struct {
		URL    string
		Fields []field
	}{URL: r.URL, Fields: fields})
}

// Send hands the payer over to the bank: an HTTP 303 for GET redirects, an
// auto-submitting form for POST redirects.
func (r Redirect) Send(w http.ResponseWriter, req *http.Request) error {
	if !r.IsPost() {
		http.Redirect(w, req, r.String(), http.StatusSeeOther)
		return nil
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	return r.WriteHTML(w)
}
