package core_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amiranmanesh/payvand/core"
)

func TestRedirectStringAppendsParamsOnGet(t *testing.T) {
	redirect := core.Redirect{
		Method: http.MethodGet,
		URL:    "https://sadad.shaparak.ir/VPG/Purchase",
		Params: map[string]string{"Token": "abc123"},
	}
	if got, want := redirect.String(), "https://sadad.shaparak.ir/VPG/Purchase?Token=abc123"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestRedirectStringKeepsPostURLUntouched(t *testing.T) {
	redirect := core.Redirect{
		Method: http.MethodPost,
		URL:    "https://bpm.shaparak.ir/pgwchannel/startpay.mellat",
		Params: map[string]string{"RefId": "9F2"},
	}
	if got := redirect.String(); got != redirect.URL {
		t.Fatalf("String() = %q, want the bare URL", got)
	}
}

func TestRedirectSendGet(t *testing.T) {
	redirect := core.Redirect{Method: http.MethodGet, URL: "https://payment.zarinpal.com/pg/StartPay/A0001"}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/pay", nil)
	if err := redirect.Send(recorder, request); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if recorder.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusSeeOther)
	}
	if got := recorder.Header().Get("Location"); got != redirect.URL {
		t.Errorf("Location = %q, want %q", got, redirect.URL)
	}
}

func TestRedirectSendPostRendersAutoSubmittingForm(t *testing.T) {
	redirect := core.Redirect{
		Method: http.MethodPost,
		URL:    "https://sepehr.shaparak.ir/_ipgw_/payment/",
		Params: map[string]string{"TerminalID": "123", "token": "tok<script>"},
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/pay", nil)
	if err := redirect.Send(recorder, request); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	body := recorder.Body.String()
	switch {
	case !strings.Contains(body, `action="https://sepehr.shaparak.ir/_ipgw_/payment/"`):
		t.Error("the form does not post to the gateway URL")
	case !strings.Contains(body, `name="TerminalID" value="123"`):
		t.Error("the terminal id field is missing")
	case strings.Contains(body, "<script>"):
		t.Error("the token value was not escaped")
	}
}
