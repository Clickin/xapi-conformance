package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/Clickin/xapi-conformance/internal/codec"
	"github.com/Clickin/xapi-conformance/internal/protocol"
)

const maxPayload = 10 << 20

func main() {
	if len(os.Args) > 1 && os.Args[1] == "stdio" {
		stdio()
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/capabilities", capabilities)
	mux.HandleFunc("/decode", operation)
	mux.HandleFunc("/encode", operation)
	mux.HandleFunc("/roundtrip", operation)
	s := &http.Server{Addr: env("XAPI_ADDR", ":8787"), Handler: limit(mux), ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	log.Printf("xapi reference server listening on %s", s.Addr)
	log.Fatal(s.ListenAndServe())
}

// stdio is the low-overhead CI transport: one complete JSON request and one
// complete JSON response per line. Logs never go to stdout.
func stdio() {
	s := bufio.NewScanner(os.Stdin)
	s.Buffer(make([]byte, 64<<10), maxPayload+1)
	for s.Scan() {
		line := s.Bytes()
		var req protocol.Envelope
		if err := protocol.DecodeJSON(bytes.NewReader(line), &req, maxPayload); err != nil {
			b, _ := json.Marshal(protocol.Response{OK: false, Error: &protocol.ErrorBody{Class: "invalid-request", Message: err.Error()}})
			fmt.Fprintln(os.Stdout, string(b))
			continue
		}
		if req.Operation == "capabilities" {
			r := httptest.NewRequest(http.MethodGet, "/capabilities", nil)
			w := httptest.NewRecorder()
			capabilities(w, r)
			fmt.Fprintln(os.Stdout, strings.TrimSpace(w.Body.String()))
			continue
		}
		path := "/" + req.Operation
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(line))
		w := httptest.NewRecorder()
		operation(w, r)
		fmt.Fprintln(os.Stdout, strings.TrimSpace(w.Body.String()))
	}
}
func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
func limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > maxPayload {
			writeError(w, http.StatusRequestEntityTooLarge, "limit-exceeded", "", "request body too large")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func capabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "invalid-request", "", "method must be GET")
		return
	}
	profiles := []any{}
	for _, name := range []string{"nexacro-json-1.0", "nexacro-xml-4000", "xplatform-xml-4000", "nexacro-ssv", "xplatform-ssv", "nexacro-binary-5000", "xplatform-binary-5000"} {
		options := []string{"strict", "base64Whitespace", "limits", "zlib"}
		if name == "nexacro-ssv" {
			options = append(options, "ssvUnitSeparator", "ssvRecordSeparator")
		}
		operations := []string{"decode", "encode", "roundtrip"}
		profiles = append(profiles, map[string]any{"name": name, "operations": operations, "options": options, "limits": map[string]int{"payloadBytes": maxPayload, "datasets": 100, "rows": 100000, "columns": 1000, "scalarBytes": 1048576, "blobBytes": 10485760}})
	}
	writeJSON(w, map[string]any{"protocolVersion": protocol.Version, "implementation": "reference-go", "profiles": profiles})
}
func operation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "invalid-request", "", "method must be POST")
		return
	}
	endpoint := strings.TrimPrefix(r.URL.Path, "/")
	if endpoint != "decode" && endpoint != "encode" && endpoint != "roundtrip" {
		writeError(w, 400, "unsupported-operation", "operation", "operation is not supported")
		return
	}
	var req protocol.Envelope
	if err := protocol.DecodeJSON(io.LimitReader(r.Body, maxPayload+1), &req, maxPayload); err != nil {
		writeError(w, 400, "invalid-request", "", err.Error())
		return
	}
	if req.Operation != endpoint {
		writeError(w, 400, "unsupported-operation", "operation", "operation does not match endpoint")
		return
	}
	if !supportedProfile(req.Profile) {
		writeError(w, 400, "unsupported-profile", "profile", "profile is not supported by reference adapter")
		return
	}
	if key := invalidOption(req.Options, boolOpt(req.Options, "strict"), req.Profile); key != "" {
		writeError(w, 400, "invalid-request", "options."+key, "unknown option")
		return
	}
	var out protocol.Response
	switch req.Operation {
	case "decode":
		if req.Input == nil {
			writeError(w, 400, "invalid-request", "input", "input is required")
			return
		}
		if msg := invalidDecodeSource(req.Options, req.Profile); msg != "" {
			writeError(w, 400, "malformed-input", "", msg)
			return
		}
		b, err := protocol.DecodeInput(*req.Input, false)
		if err != nil {
			writeError(w, 400, "malformed-input", "input.data", err.Error())
			return
		}
		if codec.IsZlib(b) {
			b, err = codec.InflateZlibLimit(b, maxPayload)
			if err != nil {
				writeError(w, 400, "malformed-input", "wire", err.Error())
				return
			}
		}
		v, err := codec.DecodeProfile(b, req.Profile, decodeOptions(req.Options))
		if err != nil {
			writeError(w, 400, "malformed-input", "wire", err.Error())
			return
		}
		out = protocol.Response{OK: true, Value: &v}
	case "encode":
		if req.Value == nil {
			writeError(w, 400, "invalid-request", "value", "value is required")
			return
		}
		if msg := invalidEncodeSource(*req.Value); msg != "" {
			writeError(w, 400, "malformed-input", "", msg)
			return
		}
		b, err := codec.Encode(*req.Value, req.Profile)
		if err != nil {
			writeError(w, 400, "invalid-value", "value", err.Error())
			return
		}
		if boolOpt(req.Options, "zlib") {
			b, err = codec.DeflateZlib(b)
			if err != nil {
				writeError(w, 400, "invalid-value", "value", err.Error())
				return
			}
		}
		out = protocol.Response{OK: true, Value: req.Value, Output: protocol.EncodeOutput(b)}
	case "roundtrip":
		if req.Input == nil {
			writeError(w, 400, "invalid-request", "input", "input is required")
			return
		}
		if msg := invalidDecodeSource(req.Options, req.Profile); msg != "" {
			writeError(w, 400, "malformed-input", "", msg)
			return
		}
		b, err := protocol.DecodeInput(*req.Input, false)
		if err != nil {
			writeError(w, 400, "malformed-input", "input.data", err.Error())
			return
		}
		if codec.IsZlib(b) {
			b, err = codec.InflateZlibLimit(b, maxPayload)
			if err != nil {
				writeError(w, 400, "malformed-input", "wire", err.Error())
				return
			}
		}
		v, err := codec.DecodeProfile(b, req.Profile, decodeOptions(req.Options))
		if err != nil {
			writeError(w, 400, "malformed-input", "wire", err.Error())
			return
		}
		encoded, e := codec.Encode(v, req.Profile)
		if e != nil {
			writeError(w, 400, "invalid-value", "value", e.Error())
			return
		}
		if boolOpt(req.Options, "zlib") {
			encoded, e = codec.DeflateZlib(encoded)
			if e != nil {
				writeError(w, 400, "invalid-value", "value", e.Error())
				return
			}
		}
		out = protocol.Response{OK: true, Value: &v, Output: protocol.EncodeOutput(encoded)}
	default:
		writeError(w, 400, "unsupported-operation", "operation", "operation is not supported")
		return
	}
	writeJSON(w, out)
}
func boolOpt(m map[string]any, k string) bool { v, ok := m[k].(bool); return ok && v }
func strictOpt(m map[string]any) bool {
	if v, ok := m["strict"].(bool); ok {
		return v
	}
	return true
}
func decodeOptions(options map[string]any) codec.DecodeOptions {
	return codec.DecodeOptions{Strict: strictOpt(options)}
}
func supportedProfile(profile string) bool {
	switch profile {
	case "nexacro-json-1.0", "nexacro-xml-4000", "xplatform-xml-4000", "nexacro-ssv", "xplatform-ssv", "nexacro-binary-5000", "xplatform-binary-5000":
		return true
	default:
		return false
	}
}
func invalidOption(options map[string]any, strict bool, profile string) string {
	if !strict {
		return ""
	}
	for key := range options {
		if key == "strict" || key == "base64Whitespace" || key == "limits" || key == "zlib" {
			continue
		}
		if (profile == "nexacro-ssv" || profile == "xplatform-ssv") && (key == "ssvUnitSeparator" || key == "ssvRecordSeparator") {
			continue
		}
		return key
	}
	return ""
}

func invalidDecodeSource(options map[string]any, profile string) string {
	if profile != "nexacro-ssv" && profile != "xplatform-ssv" {
		return ""
	}
	for _, key := range []string{"ssvUnitSeparator", "ssvRecordSeparator"} {
		if value, ok := options[key].(string); ok && value != "" {
			return "custom SSV separators are not accepted for decode"
		}
	}
	return ""
}

func invalidEncodeSource(v protocol.Value) string {
	if v.SaveType != 0 {
		return "saveType is not accepted for encode"
	}
	return ""
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, class, path, msg string) {
	writeJSONStatus(w, status, protocol.Response{OK: false, Error: &protocol.ErrorBody{Class: class, Path: path, Message: msg}})
}
func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
