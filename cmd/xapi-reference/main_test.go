package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func call(t *testing.T, path, body string) (int, map[string]any) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	operation(w, r)
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, w.Body.String())
	}
	return w.Code, out
}

func TestCapabilitiesAdvertiseAllReferenceProfiles(t *testing.T) {
	w := httptest.NewRecorder()
	capabilities(w, httptest.NewRequest(http.MethodGet, "/capabilities", nil))
	var out struct {
		Profiles []struct {
			Name       string   `json:"name"`
			Operations []string `json:"operations"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	expected := []string{
		"nexacro-json-1.0",
		"nexacro-xml-4000",
		"xplatform-xml-4000",
		"nexacro-ssv",
		"xplatform-ssv",
		"nexacro-binary-5000",
		"xplatform-binary-5000",
	}
	if len(out.Profiles) != len(expected) {
		t.Fatalf("profiles = %+v", out.Profiles)
	}
	for i, name := range expected {
		if out.Profiles[i].Name != name {
			t.Fatalf("profile[%d] = %q, want %q", i, out.Profiles[i].Name, name)
		}
	}
}

func TestMalformedUnknownAndInvalidRequests(t *testing.T) {
	if status, out := call(t, "/decode", `{`); status != 400 || out["ok"] != false {
		t.Fatalf("malformed: %d %+v", status, out)
	}
	if status, out := call(t, "/decode", `{"operation":"decode","profile":"nope","input":{"encoding":"base64","data":"eA=="}}`); status != 400 || out["error"].(map[string]any)["class"] != "unsupported-profile" {
		t.Fatalf("profile: %d %+v", status, out)
	}
	if status, out := call(t, "/nope", `{"operation":"x","profile":"nexacro-json-1.0"}`); status != 400 || out["error"].(map[string]any)["class"] != "unsupported-operation" {
		t.Fatalf("operation: %d %+v", status, out)
	}
	if status, out := call(t, "/decode", `{"operation":"decode","profile":"nexacro-json-1.0","input":{"encoding":"base64","data":"%%%"}}`); status != 400 || out["error"].(map[string]any)["class"] != "malformed-input" {
		t.Fatalf("base64: %d %+v", status, out)
	}
	if status, out := call(t, "/decode", `{"operation":"decode","profile":"nexacro-json-1.0","options":{"strict":true,"future":true},"input":{"encoding":"base64","data":"eA=="}}`); status != 400 || out["error"].(map[string]any)["path"] != "options.future" {
		t.Fatalf("unknown option: %d %+v", status, out)
	}
	if status, out := call(t, "/decode", `{"operation":"encode","profile":"nexacro-json-1.0","value":{"parameters":[],"datasets":[]}}`); status != 400 || out["error"].(map[string]any)["class"] != "unsupported-operation" {
		t.Fatalf("endpoint mismatch: %d %+v", status, out)
	}
}

func TestRequestLimit(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/decode", bytes.NewReader([]byte(`{}`)))
	r.ContentLength = maxPayload + 1
	w := httptest.NewRecorder()
	limit(http.HandlerFunc(operation)).ServeHTTP(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d", w.Code)
	}
}
