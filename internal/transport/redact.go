package transport

import (
	"regexp"
	"strings"
)

// redactedKeys are the request fields that carry a credential rather than
// payment data. Iranian gateways authenticate on the body rather than on a
// header — Mellat sends userPassword in its envelope, Parsian a LoginAccount,
// Pay.ir an api key, TOP a Pin, the OAuth providers a password grant — so the
// body of a perfectly ordinary call is a terminal's credentials.
//
// The keys are matched whole and case-insensitively, which is why "merchant"
// (Zibal's secret) is listed while "merchantId" (an identifier the payer's
// browser sees anyway) is not.
var redactedKeys = []string{
	"api",
	"api_key",
	"apikey",
	"authorization",
	"client_secret",
	"loginaccount",
	"merchant",
	"merchant_id",
	"merchantkey",
	"passwd",
	"password",
	"pin",
	"pwd",
	"secret",
	"signdata",
	"userpassword",
}

// placeholder replaces a redacted value. It is deliberately visible, so a log
// reader can tell a hidden credential from a missing field.
const placeholder = "[redacted]"

// Redaction patterns, one per encoding Payvand puts on the wire. Each captures
// the key so the field keeps its name and only the value disappears.
var (
	keyAlternation = strings.Join(redactedKeys, "|")

	jsonField = regexp.MustCompile(`(?i)"(` + keyAlternation + `)"\s*:\s*"[^"]*"`)
	xmlField  = regexp.MustCompile(`(?i)<([\w.-]*:?(?:` + keyAlternation + `))>[^<]*</[\w.:-]+>`)
	formField = regexp.MustCompile(`(?i)(^|&)(` + keyAlternation + `)=[^&]*`)
)

// redact hides credential values inside a request or response body, so the
// logger receives the shape of the call without the secret that authorised it.
// Everything it does not recognise is passed through untouched: a body is
// evidence, and over-redacting it would cost the support case it exists for.
func redact(body string) string {
	if body == "" {
		return body
	}
	body = jsonField.ReplaceAllString(body, `"$1":"`+placeholder+`"`)
	body = xmlField.ReplaceAllString(body, `<$1>`+placeholder+`</$1>`)
	return formField.ReplaceAllString(body, `${1}${2}=`+placeholder)
}
