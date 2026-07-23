package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
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
	if _, err := checkCapabilities(c, "http://adapter", []vector{v}); err != nil {
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
	if _, err := checkCapabilities(fakeClient(h), "http://adapter", []vector{v}); err == nil {
		t.Fatal("missing capability accepted")
	}
}

func TestHTTPClientSkipsUnsupportedOptionalVector(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"protocolVersion": "1.0", "profiles": []any{}})
	})
	v := vector{ID: "optional", Operation: "decode", Profile: "missing", Required: false}
	selected, err := checkCapabilities(fakeClient(h), "http://adapter", []vector{v})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 0 {
		t.Fatalf("selected = %+v", selected)
	}
}

func TestEvaluateNonZlibWireOutputRemainsExact(t *testing.T) {
	v := vector{ID: "wire.test", Operation: "encode", Profile: "p", Required: true}
	v.Expect.Kind = "wire"
	v.Expect.Output = map[string]any{"encoding": "base64", "data": "eA=="}
	actual := map[string]any{
		"ok":     true,
		"value":  map[string]any{"parameters": []any{}, "datasets": []any{}},
		"output": map[string]any{"encoding": "base64", "data": "eA=="},
	}
	if result := evaluate(v, actual); !result.Pass {
		t.Fatalf("matching wire failed: %+v", result)
	}
	actual["output"].(map[string]any)["data"] = "eQ=="
	if result := evaluate(v, actual); result.Pass {
		t.Fatal("different wire passed")
	}
}

func TestFinishShowsWireExpectedAndDiff(t *testing.T) {
	readStderr, writeStderr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	originalStderr := os.Stderr
	os.Stderr = writeStderr
	defer func() {
		os.Stderr = originalStderr
	}()
	finish([]result{{
		ID:       "wire.test",
		Error:    wireOutputDiffError,
		Expected: map[string]any{"output": map[string]any{"encoding": "base64", "data": "eA=="}},
		Diff:     "expected:\n...\nactual:\n...",
	}}, "", "")
	if err := writeStderr.Close(); err != nil {
		t.Fatal(err)
	}
	defer readStderr.Close()

	output, err := io.ReadAll(readStderr)
	if err != nil {
		t.Fatal(err)
	}
	text := string(output)
	for _, want := range []string{
		"FAIL wire.test: wire output differs\n",
		"expected:\n{\n  \"output\": {\n",
		"diff:\nexpected:\n...\nactual:\n...",
	} {
		if !bytes.Contains(output, []byte(want)) {
			t.Errorf("stderr does not contain %q:\n%s", want, text)
		}
	}
}

func TestEvaluateZlibWireOutputComparesInflatedPayload(t *testing.T) {
	payload := []byte("the same uncompressed platform payload, repeated: the same uncompressed platform payload")
	expectedOutput := testZlibOutput(t, payload, zlib.BestCompression)
	actualOutput := testZlibOutput(t, payload, zlib.NoCompression)
	if expectedOutput["data"] == actualOutput["data"] {
		t.Fatal("test requires different compressed byte streams")
	}

	v := vector{ID: "wire.zlib", Operation: "encode", Profile: "p", Options: map[string]any{"zlib": true}}
	v.Expect.Kind = "wire"
	v.Expect.Output = expectedOutput
	actual := map[string]any{"ok": true, "output": actualOutput}

	if result := evaluate(v, actual); !result.Pass {
		t.Fatalf("equivalent zlib output failed: %+v", result)
	}
}

func TestEvaluateZlibWireOutputRejectsInvalidTransport(t *testing.T) {
	payload := []byte("expected payload")
	v := vector{ID: "wire.zlib", Operation: "encode", Profile: "p", Options: map[string]any{"zlib": true}}
	v.Expect.Kind = "wire"
	v.Expect.Output = testZlibOutput(t, payload, zlib.DefaultCompression)
	trailingJunk := testZlibOutput(t, payload, zlib.DefaultCompression)
	trailingBytes, err := base64.StdEncoding.DecodeString(trailingJunk["data"].(string))
	if err != nil {
		t.Fatal(err)
	}
	trailingJunk["data"] = base64.StdEncoding.EncodeToString(append(trailingBytes, 0xde, 0xad, 0xbe, 0xef))

	tests := map[string]map[string]any{
		"wrong encoding": {
			"encoding": "text",
			"data":     v.Expect.Output["data"],
		},
		"invalid base64": {
			"encoding": "base64",
			"data":     "%%%",
		},
		"missing magic": {
			"encoding": "base64",
			"data":     base64.StdEncoding.EncodeToString([]byte("not a zlib transport")),
		},
		"invalid zlib": {
			"encoding": "base64",
			"data":     base64.StdEncoding.EncodeToString([]byte{0xff, 0xad, 0x00, 0x01}),
		},
		"different payload": testZlibOutput(t, []byte("different payload"), zlib.DefaultCompression),
		"trailing junk":     trailingJunk,
	}
	for name, output := range tests {
		t.Run(name, func(t *testing.T) {
			actual := map[string]any{"ok": true, "output": output}
			if result := evaluate(v, actual); result.Pass {
				t.Fatalf("invalid zlib transport passed: %+v", result)
			}
		})
	}
}

func testZlibOutput(t *testing.T, payload []byte, level int) map[string]any {
	t.Helper()
	var encoded bytes.Buffer
	encoded.Write([]byte{0xff, 0xad})
	writer, err := zlib.NewWriterLevel(&encoded, level)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"encoding": "base64",
		"data":     base64.StdEncoding.EncodeToString(encoded.Bytes()),
	}
}

func TestStdioClientNegotiatesCapabilities(t *testing.T) {
	t.Setenv("XAPI_STDIO_HELPER", "1")
	required := vector{ID: "required", Operation: "decode", Profile: "p", Required: true}
	required.Expect.Kind = "canonical"
	required.Expect.Value = map[string]any{"parameters": []any{}, "datasets": []any{}}
	optional := vector{ID: "optional", Operation: "decode", Profile: "missing", Required: false}
	results := runCommand(os.Args[0]+" -test.run=TestStdioHelperProcess", []vector{required, optional}, time.Second)
	if len(results) != 1 || !results[0].Pass || results[0].ID != "required" {
		t.Fatalf("results = %+v", results)
	}
}

func TestStdioHelperProcess(t *testing.T) {
	if os.Getenv("XAPI_STDIO_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			os.Exit(2)
		}
		if request["operation"] == "capabilities" {
			_ = encoder.Encode(map[string]any{
				"protocolVersion": "1.0",
				"profiles":        []any{map[string]any{"name": "p", "operations": []string{"decode"}}},
			})
			continue
		}
		_ = encoder.Encode(map[string]any{
			"ok":    true,
			"value": map[string]any{"parameters": []any{}, "datasets": []any{}},
		})
	}
	os.Exit(0)
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
