package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A body is one JSON value: anything after it (a second object, garbage) is
// invalid_json, like an unknown field.
func TestDecodeJSONRejectsTrailingData(t *testing.T) {
	for _, body := range []string{`{"host":"example.org"} {"host":"other.org"}`, `{"host":"example.org"}garbage`, `{"host":"example.org"}]`} {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/scans", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		var req createScanRequest
		if decodeJSON(rec, r, maxBodyBytes, &req) {
			t.Errorf("%q accepted as %+v", body, req)
		}
	}
}

// The demo form and POST /api/e decode with the same rule (decodeStrict);
// whitespace after the value is fine.
func TestDecodeStrict(t *testing.T) {
	var v usageEvent
	if err := decodeStrict(strings.NewReader("{\"e\":\"page_view\",\"p\":\"/\"}\n "), &v); err != nil {
		t.Errorf("value with trailing whitespace: %v", err)
	}
	for _, body := range []string{`{"e":"page_view"}{"e":"lang_switch"}`, `{"e":"page_view"} x`, `{"e":"page_view","zz":1}`} {
		if err := decodeStrict(strings.NewReader(body), &v); err == nil {
			t.Errorf("%q accepted", body)
		}
	}
}
