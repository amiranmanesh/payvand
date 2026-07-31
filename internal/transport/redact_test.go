package transport

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		secret string
		keep   string
	}{
		{
			name:   "json password",
			body:   `{"username":"shop","password":"s3cret","amount":150000}`,
			secret: "s3cret",
			keep:   "150000",
		},
		{
			name:   "json client secret of an oauth grant",
			body:   `{"grant_type":"password","client_secret":"cs-1","scope":"online"}`,
			secret: "cs-1",
			keep:   "online",
		},
		{
			name:   "xml envelope of a mellat call",
			body:   `<ns:userName>user</ns:userName><ns:userPassword>p4ss</ns:userPassword><ns:orderId>1001</ns:orderId>`,
			secret: "p4ss",
			keep:   "1001",
		},
		{
			name:   "xml login account of a parsian call",
			body:   `<ns:LoginAccount>tok-secret</ns:LoginAccount><ns:Amount>150000</ns:Amount>`,
			secret: "tok-secret",
			keep:   "150000",
		},
		{
			name:   "form api key of a pay.ir call",
			body:   `api=key-1&amount=150000&redirect=https%3A%2F%2Fshop.example`,
			secret: "key-1",
			keep:   "150000",
		},
		{
			name:   "json merchant of a zibal call",
			body:   `{"merchant":"zibal-secret","trackId":3355}`,
			secret: "zibal-secret",
			keep:   "3355",
		},
		{
			name:   "json pin of a top call",
			body:   `{"Pin":"top-pin","Amount":150000}`,
			secret: "top-pin",
			keep:   "150000",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redact(tc.body)
			if strings.Contains(got, tc.secret) {
				t.Errorf("the credential survived redaction: %s", got)
			}
			if !strings.Contains(got, tc.keep) {
				t.Errorf("redaction ate the payment data: %s", got)
			}
			if !strings.Contains(got, placeholder) {
				t.Errorf("no placeholder was left behind: %s", got)
			}
		})
	}
}

func TestRedactLeavesOrdinaryFieldsAlone(t *testing.T) {
	body := `{"merchantId":"1234","terminalId":"5678","orderId":"1001"}`
	if got := redact(body); got != body {
		t.Fatalf("redact() = %s, want the body unchanged", got)
	}
}

func TestRedactHandlesAnEmptyBody(t *testing.T) {
	if got := redact(""); got != "" {
		t.Fatalf("redact() = %q, want empty", got)
	}
}
