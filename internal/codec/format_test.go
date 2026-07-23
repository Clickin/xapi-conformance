package codec

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/Clickin/xapi-conformance/internal/protocol"
)

func TestDecodeXMLEntitiesForBothProfiles(t *testing.T) {
	profiles := []struct {
		name      string
		namespace string
	}{
		{name: "xplatform-xml-4000", namespace: "http://www.tobesoft.com/platform/Dataset"},
		{name: "nexacro-xml-4000", namespace: "http://www.nexacroplatform.com/platform/dataset"},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			wire := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?><Root xmlns="%s" ver="4000"><Dataset id="d"><ColumnInfo><Column id="a" type="STRING"/></ColumnInfo><Rows><Row><Col id="a">first&#10;second &amp; third</Col></Row></Rows></Dataset></Root>`, profile.namespace)
			value, err := DecodeProfile([]byte(wire), profile.name, DecodeOptions{Strict: true})
			if err != nil {
				t.Fatal(err)
			}
			if got := value.Datasets[0].Rows[0].Values["a"].Lexical; got != "first\nsecond & third" {
				t.Fatalf("entity decoded %q", got)
			}
		})
	}
}

func TestEncodeXMLUsesDocumentedLayoutAndEntities(t *testing.T) {
	value := protocol.Value{
		Parameters: []protocol.Parameter{{ID: "p", Type: "STRING", State: "value", Lexical: "first\nsecond"}},
		Datasets: []protocol.Dataset{{
			ID:           "d",
			Columns:      []protocol.Column{{ID: "a", Type: "STRING", Index: 0}},
			ConstColumns: []protocol.ConstColumn{{ID: "c", Type: "STRING", Value: protocol.Cell{State: "value", Lexical: "fixed"}}},
			Rows:         []protocol.Row{{Type: "N", Values: map[string]protocol.Cell{"a": {State: "value", Lexical: "a\nb"}}}},
		}},
	}
	for _, profile := range []string{"xplatform-xml-4000", "nexacro-xml-4000"} {
		wire, err := Encode(value, profile)
		if err != nil {
			t.Fatal(err)
		}
		if profile == "nexacro-xml-4000" {
			if !bytes.Contains(wire, []byte("first\nsecond")) || bytes.Contains(wire, []byte("&#10;")) {
				t.Fatalf("Nexacro line feed encoding: %s", wire)
			}
		} else if !bytes.Contains(wire, []byte("&#10;")) || bytes.Contains(wire, []byte("first\nsecond")) {
			t.Fatalf("XPlatform line feed encoding: %s", wire)
		}
		if bytes.Contains(wire, []byte("<Datasets>")) {
			t.Fatalf("non-format Datasets wrapper emitted: %s", wire)
		}
		if bytes.Contains(wire, []byte("<Row type=")) || bytes.Contains(wire, []byte("<OrgRow>")) {
			t.Fatalf("default row metadata was not omitted: %s", wire)
		}
		constantIndex := bytes.Index(wire, []byte("<ConstColumn"))
		columnIndex := bytes.Index(wire, []byte("<Column "))
		constantFirst := constantIndex >= 0 && columnIndex >= 0 && constantIndex < columnIndex
		if constantFirst != (profile == "nexacro-xml-4000") {
			t.Fatalf("profile column order: %s", wire)
		}
	}
}

func TestJSONDoesNotApplyXMLEntities(t *testing.T) {
	const literal = "first&#10;second &amp; third"
	wire := []byte(`{"version":"1.0","Parameters":[{"id":"p","type":"STRING","value":"first&#10;second &amp; third"}],"Datasets":[{"id":"d","ColumnInfo":{"Column":[{"id":"a","type":"STRING"}]},"Rows":[{"a":"x&#10;y &lt;z&gt;"}]}]}`)
	value, err := DecodeProfile(wire, "nexacro-json-1.0", DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if value.Parameters[0].Lexical != literal {
		t.Fatalf("JSON entity-like text decoded as %q", value.Parameters[0].Lexical)
	}
	row := value.Datasets[0].Rows[0]
	if row.Type != "N" || row.OrgRow != nil {
		t.Fatalf("omitted JSON row metadata decoded as %+v", row)
	}

	value.Parameters[0].Lexical = "first\nsecond & third"
	encoded, err := Encode(value, "nexacro-json-1.0")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("&#10;")) || bytes.Contains(encoded, []byte("&amp;")) {
		t.Fatalf("JSON used XML entity encoding: %s", encoded)
	}
	if !bytes.Contains(encoded, []byte(`"_RowType_":"N"`)) {
		t.Fatalf("default JSON _RowType_ was not emitted: %s", encoded)
	}
	roundTrip, err := DecodeProfile(encoded, "nexacro-json-1.0", DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := roundTrip.Parameters[0].Lexical; got != "first\nsecond & third" {
		t.Fatalf("JSON roundtrip changed lexical value to %q", got)
	}
	if got := roundTrip.Datasets[0].Rows[0].Values["a"].Lexical; got != "x&#10;y &lt;z&gt;" {
		t.Fatalf("JSON row entity-like text changed to %q", got)
	}
}

func TestDecodeXMLTreatsEmptyParameterFormsEqually(t *testing.T) {
	wire := []byte(`<?xml version="1.0" encoding="utf-8"?><Root xmlns="http://www.nexacroplatform.com/platform/dataset" ver="4000"><Parameters><Parameter id="self-closing"/><Parameter id="paired"></Parameter></Parameters></Root>`)
	value, err := DecodeProfile(wire, "nexacro-xml-4000", DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if value.Parameters[0].State != "empty" || value.Parameters[1].State != "empty" {
		t.Fatalf("parameter states: %+v", value.Parameters)
	}
}

func TestDecodeJSONRejectsOrphanOriginalRow(t *testing.T) {
	wire := []byte(`{
		"version":"1.0",
		"Parameters":[{"id":"i","value":1},{"id":"f","value":1.5},{"id":"s","value":"1"}],
		"Datasets":[{"id":"d","ColumnInfo":{
			"ConstColumn":[{"id":"c","value":10}],
			"Column":[{"id":"a","prop":"sum","sumtext":"total"}]
		},"Rows":[
			{"_RowType_":"O","a":"ignored"},
			{"_RowType_":"N","a":"normal"},
			{"_RowType_":"O","a":"ignored"},
			{"_RowType_":"U","a":"new"},
			{"_RowType_":"O","a":"old"}
		]}]
	}`)
	if _, err := DecodeProfile(wire, "nexacro-json-1.0", DecodeOptions{Strict: true}); err == nil {
		t.Fatal("orphan original row was accepted")
	}
}

func TestDecodeJSONDefaultsVersion(t *testing.T) {
	wire := []byte(`{"Parameters":[],"Datasets":[]}`)
	value, err := DecodeProfile(wire, "nexacro-json-1.0", DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if value.Wire["version"] != "1.0" {
		t.Fatalf("wire version = %#v", value.Wire)
	}
}

func TestXMLEncodeTypeCaseBlobEncodingAndOptionalConstantValue(t *testing.T) {
	for _, profile := range []string{"xplatform-xml-4000", "nexacro-xml-4000"} {
		t.Run(profile, func(t *testing.T) {
			encoded, err := Encode(protocol.Value{Datasets: []protocol.Dataset{{
				ID: "blob",
				Columns: []protocol.Column{
					{ID: "lower", Type: "string"},
					{ID: "mixed", Type: "StRiNg"},
					{ID: "b", Type: "blob"},
				},
				ConstColumns: []protocol.ConstColumn{
					{ID: "missing", Type: "string", Value: protocol.Cell{State: "missing"}},
					{ID: "empty", Type: "STRING", Value: protocol.Cell{State: "value", Lexical: ""}},
					{ID: "c", Type: "BlOb", Value: protocol.Cell{State: "value", Lexical: "eA=="}},
				},
			}}}, profile)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Count(encoded, []byte(`type="string"`)) != 4 ||
				bytes.Count(encoded, []byte(`encrypt="base64"`)) != 2 ||
				!bytes.Contains(encoded, []byte(`<ConstColumn id="missing" type="string" size="32"/>`)) ||
				!bytes.Contains(encoded, []byte(`<ConstColumn id="empty" type="string" size="32" value=""/>`)) {
				t.Fatalf("XML type/value contract not emitted: %s", encoded)
			}
		})
	}
}

func TestXMLAcceptsBlobWithoutEncodingMetadata(t *testing.T) {
	for _, element := range []string{
		`<Column id="b" type="BLOB"/>`,
		`<ConstColumn id="b" type="BLOB" value="eA=="/>`,
	} {
		wire := []byte(`<?xml version="1.0" encoding="utf-8"?><Root xmlns="http://www.nexacroplatform.com/platform/dataset" ver="4000"><Dataset id="d"><ColumnInfo>` + element + `</ColumnInfo></Dataset></Root>`)
		value, err := DecodeProfile(wire, "nexacro-xml-4000", DecodeOptions{Strict: true})
		if err != nil {
			t.Fatalf("BLOB without encoding metadata rejected: %v", err)
		}
		if len(value.Datasets) != 1 {
			t.Fatalf("dataset missing: %+v", value)
		}
	}
}

func TestTypesAreCaseInsensitiveAcrossJSONAndSSV(t *testing.T) {
	jsonWire := []byte(`{"version":"1.0","Parameters":[{"id":"p","type":"sTrInG","value":"x"}],"Datasets":[{"id":"d","ColumnInfo":{"ConstColumn":[{"id":"c","type":"iNt","value":1}],"Column":[{"id":"a","type":"bIgDeCiMaL"}]},"Rows":[]}]}`)
	jsonValue, err := DecodeProfile(jsonWire, "nexacro-json-1.0", DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if jsonValue.Parameters[0].Type != "STRING" || jsonValue.Datasets[0].ConstColumns[0].Type != "INT" || jsonValue.Datasets[0].Columns[0].Type != "BIGDECIMAL" {
		t.Fatalf("JSON types were not normalized: %+v", jsonValue)
	}

	ssvWire := []byte("SSV\x1ep:sTrInG=x\x1eDataset:d\x1e_Const_\x1fc:iNt=1\x1e_RowType_\x1fa:bIgDeCiMaL\x1e\x1e")
	ssvValue, err := DecodeProfile(ssvWire, nexacroSSVProfile, DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if ssvValue.Parameters[0].Type != "STRING" || ssvValue.Datasets[0].ConstColumns[0].Type != "INT" || ssvValue.Datasets[0].Columns[0].Type != "BIGDECIMAL" {
		t.Fatalf("SSV types were not normalized: %+v", ssvValue)
	}
}

func TestEncodeUsesPublishedTypeAliases(t *testing.T) {
	value := protocol.Value{
		Parameters: []protocol.Parameter{
			{ID: "bool", Type: "BOOLEAN", State: "value", Lexical: "true"},
			{ID: "long", Type: "LONG", State: "value", Lexical: "7"},
			{ID: "double", Type: "DOUBLE", State: "value", Lexical: "1.5"},
			{ID: "file", Type: "FILE", State: "value", Lexical: "AA=="},
		},
		Datasets: []protocol.Dataset{},
	}
	wire, err := Encode(value, "nexacro-json-1.0")
	if err != nil {
		t.Fatal(err)
	}
	text := string(wire)
	for _, alias := range []string{`"type":"int"`, `"type":"bigdecimal"`, `"type":"float"`, `"type":"blob"`} {
		if !strings.Contains(text, alias) {
			t.Fatalf("missing published alias %s in %s", alias, text)
		}
	}
}
