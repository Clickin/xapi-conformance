package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
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
	for _, name := range []string{"nexacro-json-1.0", "nexacro-xml-4000", "xplatform-xml-4000"} {
		profiles = append(profiles, map[string]any{"name": name, "operations": []string{"decode", "encode", "roundtrip"}, "options": []string{"strict", "base64Whitespace", "limits"}, "limits": map[string]int{"payloadBytes": maxPayload, "datasets": 100, "rows": 100000, "columns": 1000, "scalarBytes": 1048576, "blobBytes": 10485760}})
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
	if req.Profile != "nexacro-json-1.0" && req.Profile != "nexacro-xml-4000" && req.Profile != "xplatform-xml-4000" {
		writeError(w, 400, "unsupported-profile", "profile", "profile is not supported by reference adapter")
		return
	}
	if key := invalidOption(req.Options, boolOpt(req.Options, "strict")); key != "" {
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
		b, err := protocol.DecodeInput(*req.Input, boolOpt(req.Options, "base64Whitespace"))
		if err != nil {
			writeError(w, 400, "malformed-input", "input.data", err.Error())
			return
		}
		v, err := codec.DecodeWithStrict(b, strictOpt(req.Options))
		if err != nil {
			writeError(w, 400, "malformed-input", "wire", err.Error())
			return
		}
		if class, path, msg := checkLimits(v, req.Options, int64(len(b))); class != "" {
			writeError(w, 400, class, path, msg)
			return
		}
		out = protocol.Response{OK: true, Value: &v}
	case "encode":
		if req.Value == nil {
			writeError(w, 400, "invalid-request", "value", "value is required")
			return
		}
		if class, path, msg := checkLimits(*req.Value, req.Options, -1); class != "" {
			writeError(w, 400, class, path, msg)
			return
		}
		b, err := codec.Encode(*req.Value, req.Profile)
		if err != nil {
			writeError(w, 500, "internal", "", err.Error())
			return
		}
		out = protocol.Response{OK: true, Value: req.Value, Output: protocol.EncodeOutput(b)}
	case "roundtrip":
		if req.Input == nil {
			writeError(w, 400, "invalid-request", "input", "input is required")
			return
		}
		b, err := protocol.DecodeInput(*req.Input, boolOpt(req.Options, "base64Whitespace"))
		if err != nil {
			writeError(w, 400, "malformed-input", "input.data", err.Error())
			return
		}
		v, err := codec.DecodeWithStrict(b, strictOpt(req.Options))
		if err != nil {
			writeError(w, 400, "malformed-input", "wire", err.Error())
			return
		}
		if class, path, msg := checkLimits(v, req.Options, int64(len(b))); class != "" {
			writeError(w, 400, class, path, msg)
			return
		}
		encoded, e := codec.Encode(v, req.Profile)
		if e != nil {
			writeError(w, 500, "internal", "", e.Error())
			return
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
func invalidOption(options map[string]any, strict bool) string {
	if !strict {
		return ""
	}
	for k := range options {
		if k != "strict" && k != "base64Whitespace" && k != "limits" {
			return k
		}
	}
	return ""
}

func checkLimits(v protocol.Value, options map[string]any, payloadBytes int64) (string, string, string) {
	l, _ := options["limits"].(map[string]any)
	if l == nil {
		return "", "", ""
	}
	if n, ok := limitNumber(l, "payloadBytes"); ok && payloadBytes >= 0 && payloadBytes > n {
		return "limit-exceeded", "input.data", "payload exceeds configured limit"
	}
	if n, ok := limitNumber(l, "datasets"); ok && int64(len(v.Datasets)) > n {
		return "limit-exceeded", "value.datasets", "dataset limit exceeded"
	}
	for di, d := range v.Datasets {
		if n, ok := limitNumber(l, "columns"); ok && int64(len(d.Columns)+len(d.ConstColumns)) > n {
			return "limit-exceeded", fmt.Sprintf("value.datasets[%d].columns", di), "column limit exceeded"
		}
		if n, ok := limitNumber(l, "rows"); ok && int64(len(d.Rows)) > n {
			return "limit-exceeded", fmt.Sprintf("value.datasets[%d].rows", di), "row limit exceeded"
		}
		for ri, row := range d.Rows {
			if n, ok := limitNumber(l, "depth"); ok && int64(rowDepth(row)) > n {
				return "limit-exceeded", fmt.Sprintf("value.datasets[%d].rows[%d].orgRow", di, ri), "row depth limit exceeded"
			}
			if len(row.Values) > 0 {
				for id, c := range row.Values {
					if n, ok := limitNumber(l, "scalarBytes"); ok && int64(len(c.Lexical)) > n {
						return "limit-exceeded", fmt.Sprintf("value.datasets[%d].rows[%d].values.%s", di, ri, id), "scalar limit exceeded"
					}
					if c.State == "value" && strings.EqualFold(columnType(d, id), "BLOB") {
						if n, ok := limitNumber(l, "blobBytes"); ok {
							if decoded, err := base64.StdEncoding.DecodeString(c.Lexical); err == nil && int64(len(decoded)) > n {
								return "limit-exceeded", fmt.Sprintf("value.datasets[%d].rows[%d].values.%s", di, ri, id), "blob limit exceeded"
							}
						}
					}
				}
			}
		}
	}
	return "", "", ""
}
func limitNumber(m map[string]any, key string) (int64, bool) {
	v, ok := m[key].(float64)
	return int64(v), ok && v >= 0
}
func columnType(d protocol.Dataset, id string) string {
	for _, c := range d.Columns {
		if c.ID == id {
			return c.Type
		}
	}
	return ""
}
func rowDepth(r protocol.Row) int {
	n := 1
	for r.OrgRow != nil {
		n++
		r = *r.OrgRow
	}
	return n
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
