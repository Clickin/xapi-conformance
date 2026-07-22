package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClientChecksCapabilitiesAndCanonicalValue(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/capabilities" {
			_ = json.NewEncoder(w).Encode(map[string]any{"protocolVersion": "1.0", "profiles": []any{map[string]any{"name": "p", "operations": []string{"decode"}}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "value": map[string]any{"parameters": []any{}, "datasets": []any{}}})
	})
	v := vector{ID: "client.test", Operation: "decode", Profile: "p", Required: true}
	v.Expect.Kind = "canonical"
	v.Expect.Value = map[string]any{"parameters": []any{}, "datasets": []any{}}
	c := fakeClient(h)
	if err := checkCapabilities(c, "http://adapter", []vector{v}); err != nil {
		t.Fatal(err)
	}
	r := run(c, "http://adapter", v)
	if !r.Pass {
		t.Fatalf("result = %+v", r)
	}
}

func TestHTTPClientRejectsMissingRequiredCapability(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"protocolVersion": "1.0", "profiles": []any{}})
	})
	v := vector{ID: "required", Operation: "decode", Profile: "missing", Required: true}
	if err := checkCapabilities(fakeClient(h), "http://adapter", []vector{v}); err == nil {
		t.Fatal("missing capability accepted")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fakeClient(h http.Handler) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return &http.Response{StatusCode: w.Code, Status: "200 OK", Header: w.Header(), Body: io.NopCloser(w.Body), Request: r}, nil
	})}
}
