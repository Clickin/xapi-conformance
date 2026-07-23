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

func TestConfiguredResourceLimitsAreIgnored(t *testing.T) {
	const value = `{"parameters":[],"datasets":[{"id":"d","columns":[{"id":"a","type":"BLOB","index":0}],"constColumns":[],"rows":[{"type":"N","values":{"a":{"state":"value","lexical":"YQ=="}}}]}]}`
	for _, key := range []string{"datasets", "columns", "rows", "depth", "scalarBytes", "blobBytes"} {
		t.Run(key, func(t *testing.T) {
			body := `{"operation":"encode","profile":"nexacro-json-1.0","options":{"limits":{"` + key + `":0}},"value":` + value + `}`
			if status, out := call(t, "/encode", body); status != 200 || out["ok"] != true {
				t.Fatalf("configured %s limit was enforced: %d %+v", key, status, out)
			}
		})
	}

	const payload = `{"operation":"decode","profile":"nexacro-json-1.0","options":{"limits":{"payloadBytes":1}},"input":{"encoding":"base64","data":"eyJ2ZXJzaW9uIjoiMS4wIiwiUGFyYW1ldGVycyI6W10sIkRhdGFzZXRzIjpbXX0="}}`
	if status, out := call(t, "/decode", payload); status != 200 || out["ok"] != true {
		t.Fatalf("configured payload limit was enforced: %d %+v", status, out)
	}
}

func TestDriverIgnoredDecodeOptionsDoNotRelaxSourceParsing(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "base64 whitespace",
			body: `{"operation":"decode","profile":"nexacro-json-1.0","options":{"strict":true,"base64Whitespace":true},"input":{"encoding":"base64","data":"eyJ2ZXJzaW9uIjoiMS4wIiwiUGFyYW1ldGVycyI6W10sIkRhdGFzZXRzIjpbXX0 ="}}`,
		},
		{
			name: "SSV separators",
			body: `{"operation":"decode","profile":"nexacro-ssv","options":{"strict":true,"ssvUnitSeparator":"|","ssvRecordSeparator":"~"},"input":{"encoding":"base64","data":"U1NWOnV0Zi04fkRhdGFzZXQ6ZH5fUm93VHlwZV98YTpTVFJJTkd+Tnx4fn4="}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, out := call(t, "/decode", test.body)
			if status != 400 || out["error"].(map[string]any)["class"] != "malformed-input" {
				t.Fatalf("status = %d, response = %+v", status, out)
			}
		})
	}
}

func TestSaveTypeEncodeSourceRejection(t *testing.T) {
	const body = `{"operation":"encode","profile":"nexacro-json-1.0","value":{"saveType":5,"parameters":[],"datasets":[]}}`
	status, out := call(t, "/encode", body)
	if status != 400 || out["error"].(map[string]any)["class"] != "malformed-input" {
		t.Fatalf("status = %d, response = %+v", status, out)
	}
}

func TestBinaryIntrinsicLengthErrorRemainsCodecOwned(t *testing.T) {
	const body = `{"operation":"decode","profile":"nexacro-binary-5000","options":{"strict":true},"input":{"encoding":"base64","data":"/hATiP//"}}`
	status, out := call(t, "/decode", body)
	if status != 400 || out["error"].(map[string]any)["class"] != "malformed-input" || out["error"].(map[string]any)["path"] != "wire" {
		t.Fatalf("status = %d, response = %+v", status, out)
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
