package core_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amiranmanesh/payvand/core"
)

func TestCallbackValuesMergesQueryAndForm(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/callback?Authority=A1", strings.NewReader("Status=OK"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	values, err := core.CallbackValues(request)
	if err != nil {
		t.Fatalf("CallbackValues() error = %v", err)
	}
	if values["Authority"] != "A1" || values["Status"] != "OK" {
		t.Fatalf("values = %v, want the query and form merged", values)
	}
}

func TestFirstValueSkipsEmptyKeys(t *testing.T) {
	values := map[string]string{"status": "", "Status": "OK"}
	if got := core.FirstValue(values, "missing", "status", "Status"); got != "OK" {
		t.Fatalf("FirstValue() = %q, want OK", got)
	}
}

func TestCallbackVerifyRequestUsesCallerAmount(t *testing.T) {
	callback := core.Callback{
		Token:           "A1",
		OrderID:         "1001",
		ReferenceNumber: "RRN",
		TraceNumber:     "TRACE",
		CardNumber:      "6037****1234",
		Amount:          core.Rial(1),
		Values:          map[string]string{"digitalreceipt": "receipt"},
	}

	request := callback.VerifyRequest(core.Toman(1_500))
	switch {
	case request.Token != "A1":
		t.Error("the token was not carried over")
	case request.Amount.Rial() != 15_000:
		t.Errorf("amount = %v, want the caller amount, not the one reported by the browser", request.Amount)
	case request.Get("digitalreceipt") != "receipt":
		t.Error("the provider specific values were not carried over")
	}
}
