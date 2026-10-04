// xapi-import converts pinned upstream wire fixtures into language-neutral
// vectors. It is deterministic and records malformed upstream fixtures as
// negative vectors instead of dropping them.
package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Clickin/xapi-conformance/internal/codec"
	"github.com/Clickin/xapi-conformance/internal/protocol"
)

type fixture struct {
	ID, Path, Profile  string
	StartLine, EndLine int
	Prefix, Suffix     string
}
type vector struct {
	ID        string         `json:"id"`
	Operation string         `json:"operation"`
	Profile   string         `json:"profile"`
	Input     protocol.Input `json:"input"`
	Options   map[string]any `json:"options"`
	Expect    struct {
		Kind  string              `json:"kind"`
		Value *protocol.Value     `json:"value,omitempty"`
		Error *protocol.ErrorBody `json:"error,omitempty"`
	} `json:"expect"`
	Source   map[string]string `json:"source"`
	Required bool              `json:"required"`
}

func main() {
	out := flag.String("output", "vectors/valid/imported", "output directory")
	flag.Parse()
	for _, f := range []struct{ path, prefix string }{
		{"sources/xapi-js/packages/core/test/handler.test.ts", "xapi-js.handler.inline"},
		{"sources/xapi-js/packages/core/test/index.test.ts", "xapi-js.core-index.inline"},
		{"sources/xapi-js/packages/core/test/schema.test.ts", "xapi-js.core-schema.inline"},
		{"sources/xapi-js/packages/adaptor-express/test/middleware.test.ts", "xapi-js.express.inline"},
		{"sources/xapi-js/packages/adaptor-fetch/test/index.test.ts", "xapi-js.fetch.inline"},
		{"sources/xapi-js/packages/adaptor-nestjs/test/xapi-request-interceptor.test.ts", "xapi-js.nest-request.inline"},
	} {
		importDelimitedXML(*out, f.path, "`", f.prefix)
	}
	importDelimitedXML(*out, "sources/xplatform-xml/xplatform-xml-micronaut/src/test/java/io/clickin/xplatform/xml/runtime/examples/XplatformXmlSerdeTest.java", "\"\"\"", "xplatform-xml.test.inline")
}

func importDelimitedXML(validOut, path, delimiter, idPrefix string) {
	b, err := os.ReadFile(path)
	if err != nil {
		fatalf("%s: %v", path, err)
	}
	text := string(b)
	cursor, ordinal := 0, 0
	seen := map[[32]byte]bool{}
	for cursor < len(text) {
		start := strings.Index(text[cursor:], "<?xml")
		root := strings.Index(text[cursor:], "<Root")
		if start < 0 || (root >= 0 && root < start) {
			start = root
		}
		if start < 0 {
			break
		}
		start += cursor
		relEnd := strings.Index(text[start:], delimiter)
		if relEnd < 0 {
			break
		}
		end := start + relEnd
		wire := strings.TrimSpace(text[start:end])
		cursor = end + 1
		if strings.Contains(wire, "${") || !strings.Contains(wire, "<Root") {
			continue
		}
		hash := sha256.Sum256([]byte(wire))
		if seen[hash] {
			continue
		}
		seen[hash] = true
		ordinal++
		f := fixture{ID: fmt.Sprintf("%s-%02d-%x.imported", idPrefix, ordinal, hash[:4]), Path: path, Profile: "xplatform-xml-5000"}
		if strings.HasPrefix(path, "sources/xapi-js/") {
			f.Profile = "nexacro-xml-4000"
		}
		v := vector{ID: f.ID, Operation: "decode", Profile: f.Profile, Input: protocol.Input{Encoding: "base64", Data: base64.StdEncoding.EncodeToString([]byte(wire))}, Options: map[string]any{"strict": false}, Required: true}
		repo, commit, sourcePath, license, attr := sourceMeta(path)
		v.Source = map[string]string{"repository": repo, "commit": commit, "path": sourcePath, "license": license, "attribution": attr}
		value, decodeErr := codec.DecodeWithStrict([]byte(wire), false)
		if decodeErr != nil {
			v.Expect.Kind = "error"
			v.Expect.Error = &protocol.ErrorBody{Class: "malformed-input", Path: "wire"}
			out := filepath.Join(filepath.Dir(filepath.Dir(validOut)), "invalid", filepath.Base(validOut))
			writeVector(out, f, v)
		} else {
			v.Expect.Kind = "canonical"
			v.Expect.Value = &value
			writeVector(validOut, f, v)
		}
	}
}

func sourceMeta(path string) (string, string, string, string, string) {
	if strings.HasPrefix(path, "sources/xapi-js/") {
		return "Clickin/xapi-js", "244339098838d098d7087588d039a24d26448b5e", strings.TrimPrefix(path, "sources/xapi-js/"), "MIT", "Clickin/xapi-js"
	}
	if strings.HasPrefix(path, "sources/xplatform-xml/") {
		return "Clickin/xplatform-xml", "cda8b6b31f64511ff9d22f64539c4b78e862455d", strings.TrimPrefix(path, "sources/xplatform-xml/"), "upstream-no-license-file", "Clickin/xplatform-xml"
	}
	return "xapi-conformance", "working-tree", path, "repository", "xapi-conformance"
}
func writeVector(out string, f fixture, v vector) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fatalf("%s: %v", f.Path, err)
	}
	name := filepath.Join(out, strings.ReplaceAll(strings.TrimSuffix(f.ID, ".imported"), ".", "-")+".json")
	if err = os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		fatalf("%s: %v", name, err)
	}
	if err = os.WriteFile(name, append(b, '\n'), 0644); err != nil {
		fatalf("%s: %v", name, err)
	}
	fmt.Println(name)
}
func fatalf(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...); os.Exit(1) }
