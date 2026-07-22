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

var fixtures = []fixture{
	{ID: "xapi-xml.sample-basic.imported", Path: "sources/xapi/xapi-xml/src/test/resources/xapi/sample-basic.xml", Profile: "nexacro-xml-4000"},
	{ID: "xapi-xml.sample-complex.imported", Path: "sources/xapi/xapi-xml/src/test/resources/xapi/sample-complex.xml", Profile: "nexacro-xml-4000"},
	{ID: "xapi-xml.sample-constcolumn.imported", Path: "sources/xapi/xapi-xml/src/test/resources/xapi/sample-constcolumn.xml", Profile: "nexacro-xml-4000"},
	{ID: "xapi-xml.sample-empty.imported", Path: "sources/xapi/xapi-xml/src/test/resources/xapi/sample-empty.xml", Profile: "nexacro-xml-4000"},
	{ID: "xapi-xml.sample-cdata.imported", Path: "sources/xapi/xapi-xml/src/test/resources/xapi/sample-cdata.xml", Profile: "nexacro-xml-4000"},
	{ID: "xapi-xml.sample-orgrow.imported", Path: "sources/xapi/xapi-xml/src/test/resources/xapi/sample-orgrow.xml", Profile: "nexacro-xml-4000"},
	{ID: "xapi-xml.error-col-before-row.imported", Path: "sources/xapi/xapi-xml/src/test/resources/xapi/error/col-before-row.xml", Profile: "nexacro-xml-4000"},
	{ID: "xapi-xml.error-missing-column.imported", Path: "sources/xapi/xapi-xml/src/test/resources/xapi/error/missing-column.xml", Profile: "nexacro-xml-4000"},
}

func main() {
	out := flag.String("output", "vectors/valid/imported", "output directory")
	flag.Parse()
	for _, f := range fixtures {
		b, err := readFixture(f)
		if err != nil {
			fatalf("%s: %v", f.Path, err)
		}
		importWire(*out, f, b)
	}
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
	for _, f := range []struct{ path, prefix string }{
		{"sources/xapi/xapi-xml/src/test/java/io/github/clickin/xapi/XapiEdgeCasesTest.java", "xapi-xml.edge.inline"},
		{"sources/xapi/xapi-xml/src/test/java/io/github/clickin/xapi/XapiErrorHandlingTest.java", "xapi-xml.error.inline"},
		{"sources/xapi/xapi-xml/src/test/java/io/github/clickin/xapi/XapiTest.java", "xapi-xml.test.inline"},
		{"sources/xapi/xapi-xml/src/test/java/io/github/clickin/xapi/XapiWriterTest.java", "xapi-xml.writer.inline"},
		{"sources/xapi/xapi-xml/src/test/java/io/github/clickin/xapi/ByteArraySerializationTest.java", "xapi-xml.bytes.inline"},
		{"sources/xapi/xapi-xml/src/test/java/io/github/clickin/xapi/adapter/ValueObjectHandlerTest.java", "xapi-xml.value-handler.inline"},
		{"sources/xapi/xapi-xml/src/test/java/io/github/clickin/xapi/adapter/ValueObjectDataProviderTest.java", "xapi-xml.value-provider.inline"},
	} {
		importJavaXML(*out, f.path, f.prefix)
	}
	importDelimitedXML(*out, "sources/xplatform-xml/xplatform-xml-micronaut/src/test/java/io/clickin/xplatform/xml/runtime/examples/XplatformXmlSerdeTest.java", "\"\"\"", "xplatform-xml.test.inline")
}

func importWire(validOut string, f fixture, b []byte) {
	repository, commit, sourcePath, license, attribution := sourceMeta(f.Path)
	v := vector{ID: f.ID, Operation: "decode", Profile: f.Profile, Input: protocol.Input{Encoding: "base64", Data: base64.StdEncoding.EncodeToString(b)}, Options: map[string]any{}, Required: true, Source: map[string]string{"repository": repository, "commit": commit, "path": sourcePath, "license": license, "attribution": attribution}}
	value, err := codec.Decode(b)
	if err != nil {
		v.Expect.Kind = "error"
		v.Expect.Error = &protocol.ErrorBody{Class: "malformed-input", Path: "wire"}
		writeVector(filepath.Join(filepath.Dir(filepath.Dir(validOut)), "invalid", filepath.Base(validOut)), f, v)
		return
	}
	v.Expect.Kind = "canonical"
	v.Expect.Value = &value
	writeVector(validOut, f, v)
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
		f := fixture{ID: fmt.Sprintf("%s-%02d-%x.imported", idPrefix, ordinal, hash[:4]), Path: path, Profile: "xplatform-xml-4000"}
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

func importJavaXML(validOut, path, idPrefix string) {
	b, err := os.ReadFile(path)
	if err != nil {
		fatalf("%s: %v", path, err)
	}
	stringsInFile := javaStringLiterals(string(b))
	seen := map[[32]byte]bool{}
	ordinal := 0
	for i, literal := range stringsInFile {
		if !strings.Contains(literal, "<Root") {
			continue
		}
		wire := literal
		for j := i + 1; j < len(stringsInFile) && !strings.Contains(wire, "</Root>"); j++ {
			wire += stringsInFile[j]
		}
		wire = strings.TrimSpace(wire)
		if !strings.Contains(wire, "</Root>") {
			continue
		}
		hash := sha256.Sum256([]byte(wire))
		if seen[hash] {
			continue
		}
		seen[hash] = true
		ordinal++
		f := fixture{ID: fmt.Sprintf("%s-%02d-%x.imported", idPrefix, ordinal, hash[:4]), Path: path, Profile: "nexacro-xml-4000"}
		v := vector{ID: f.ID, Operation: "decode", Profile: f.Profile, Input: protocol.Input{Encoding: "base64", Data: base64.StdEncoding.EncodeToString([]byte(wire))}, Options: map[string]any{"strict": false}, Required: true}
		repo, commit, sourcePath, license, attr := sourceMeta(path)
		v.Source = map[string]string{"repository": repo, "commit": commit, "path": sourcePath, "license": license, "attribution": attr}
		value, decodeErr := codec.DecodeWithStrict([]byte(wire), false)
		if decodeErr != nil {
			v.Expect.Kind = "error"
			v.Expect.Error = &protocol.ErrorBody{Class: "malformed-input", Path: "wire"}
			writeVector(filepath.Join(filepath.Dir(filepath.Dir(validOut)), "invalid", filepath.Base(validOut)), f, v)
		} else {
			v.Expect.Kind = "canonical"
			v.Expect.Value = &value
			writeVector(validOut, f, v)
		}
	}
}

func javaStringLiterals(text string) []string {
	var out []string
	for i := 0; i < len(text); i++ {
		if text[i] != '"' || (i > 0 && text[i-1] == '\\') {
			continue
		}
		var b strings.Builder
		for i++; i < len(text); i++ {
			if text[i] == '"' {
				break
			}
			if text[i] != '\\' || i+1 >= len(text) {
				b.WriteByte(text[i])
				continue
			}
			i++
			switch text[i] {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case '\\', '"', '\'':
				b.WriteByte(text[i])
			case 'u':
				if i+4 < len(text) {
					var code uint64
					if _, err := fmt.Sscanf(text[i+1:i+5], "%04x", &code); err == nil {
						b.WriteRune(rune(code))
						i += 4
					} else {
						b.WriteByte('u')
					}
				} else {
					b.WriteByte('u')
				}
			default:
				b.WriteByte(text[i])
			}
		}
		out = append(out, b.String())
	}
	return out
}
func sourceMeta(path string) (string, string, string, string, string) {
	if strings.HasPrefix(path, "sources/xapi-js/") {
		return "Clickin/xapi-js", "244339098838d098d7087588d039a24d26448b5e", strings.TrimPrefix(path, "sources/xapi-js/"), "MIT", "Clickin/xapi-js"
	}
	if strings.HasPrefix(path, "sources/xplatform-xml/") {
		return "Clickin/xplatform-xml", "cda8b6b31f64511ff9d22f64539c4b78e862455d", strings.TrimPrefix(path, "sources/xplatform-xml/"), "upstream-no-license-file", "Clickin/xplatform-xml"
	}
	return "Clickin/xapi", "e0d54759162bbb06aef8a1bc4c557f9fe7336991", strings.TrimPrefix(path, "sources/xapi/"), "Apache-2.0", "Clickin/xapi"
}
func readFixture(f fixture) ([]byte, error) {
	b, err := os.ReadFile(f.Path)
	if err != nil || f.StartLine == 0 {
		return b, err
	}
	lines := strings.Split(string(b), "\n")
	if f.StartLine < 1 || f.EndLine > len(lines) {
		return nil, fmt.Errorf("inline range is outside source")
	}
	selected := strings.Join(lines[f.StartLine-1:f.EndLine], "\n")
	selected = strings.TrimPrefix(selected, f.Prefix)
	selected = strings.TrimSuffix(selected, f.Suffix)
	return []byte(selected + "\n"), nil
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
