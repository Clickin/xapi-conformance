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
		if !bytes.Contains(wire, []byte("&#10;")) || bytes.Contains(wire, []byte("first\nsecond")) {
			t.Fatalf("line feed was not entity encoded: %s", wire)
		}
		if bytes.Contains(wire, []byte("<Datasets>")) {
			t.Fatalf("non-format Datasets wrapper emitted: %s", wire)
		}
		if bytes.Contains(wire, []byte("<Row type=")) || bytes.Contains(wire, []byte("<OrgRow>")) {
			t.Fatalf("default row metadata was not omitted: %s", wire)
		}
		constantIndex := bytes.Index(wire, []byte("<ConstColumn"))
		columnIndex := bytes.Index(wire, []byte("<Column "))
		if constantIndex < 0 || columnIndex < 0 || constantIndex > columnIndex {
			t.Fatalf("ConstColumn did not precede Column: %s", wire)
		}
		if _, err := DecodeProfile(wire, profile, DecodeOptions{Strict: true}); err != nil {
			t.Fatalf("encoded XML rejected: %v\n%s", err, wire)
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
	if bytes.Contains(encoded, []byte(`"_RowType_"`)) {
		t.Fatalf("default JSON _RowType_ was not omitted: %s", encoded)
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

func TestDecodeXMLDistinguishesNullAndEmptyParameter(t *testing.T) {
	wire := []byte(`<?xml version="1.0" encoding="utf-8"?><Root xmlns="http://www.nexacroplatform.com/platform/dataset" ver="4000"><Parameters><Parameter id="null"/><Parameter id="empty"></Parameter></Parameters></Root>`)
	value, err := DecodeProfile(wire, "nexacro-xml-4000", DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if value.Parameters[0].State != "null" || value.Parameters[1].State != "empty" {
		t.Fatalf("parameter states: %+v", value.Parameters)
	}
}

func TestDecodeJSONAppliesDocumentedDefaultsAndOriginalRowRules(t *testing.T) {
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
	value, err := DecodeProfile(wire, "nexacro-json-1.0", DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if value.Parameters[0].Type != "INT" || value.Parameters[1].Type != "FLOAT" || value.Parameters[2].Type != "STRING" {
		t.Fatalf("parameter defaults: %+v", value.Parameters)
	}
	dataset := value.Datasets[0]
	if dataset.ConstColumns[0].Type != "INT" || dataset.Columns[0].Type != "STRING" || dataset.Columns[0].Prop != "sum" || dataset.Columns[0].SumText != "total" {
		t.Fatalf("column defaults: %+v %+v", dataset.ConstColumns, dataset.Columns)
	}
	if len(dataset.Rows) != 2 ||
		dataset.Rows[0].Type != "N" || dataset.Rows[0].OrgRow == nil || dataset.Rows[0].OrgRow.Values["a"].Lexical != "ignored" ||
		dataset.Rows[1].Type != "U" || dataset.Rows[1].OrgRow == nil || dataset.Rows[1].OrgRow.Values["a"].Lexical != "old" {
		t.Fatalf("original row rules: %+v", dataset.Rows)
	}
}

func TestDecodeJSONRequiresVersion(t *testing.T) {
	wire := []byte(`{"Parameters":[],"Datasets":[]}`)
	if _, err := DecodeProfile(wire, "nexacro-json-1.0", DecodeOptions{Strict: true}); err == nil {
		t.Fatal("missing JSON version accepted")
	}
}

func TestXMLTypeCaseBlobEncodingAndOptionalConstantValue(t *testing.T) {
	for _, profile := range []struct {
		name      string
		namespace string
	}{
		{name: "xplatform-xml-4000", namespace: "http://www.tobesoft.com/platform/Dataset"},
		{name: "nexacro-xml-4000", namespace: "http://www.nexacroplatform.com/platform/dataset"},
	} {
		t.Run(profile.name, func(t *testing.T) {
			wire := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?><Root xmlns="%s" ver="4000"><Dataset id="d"><ColumnInfo><ConstColumn id="missing" type="string"/><ConstColumn id="empty" type="STRING" value=""/><ConstColumn id="blobConst" type="bLoB" enc="BASE64" value="eA=="/><Column id="lower" type="string"/><Column id="mixed" type="StRiNg"/><Column id="blob" type="blob" enc="base64"/></ColumnInfo><Rows/></Dataset></Root>`, profile.namespace)
			value, err := DecodeProfile([]byte(wire), profile.name, DecodeOptions{Strict: true})
			if err != nil {
				t.Fatal(err)
			}
			dataset := value.Datasets[0]
			if dataset.Columns[0].Type != "STRING" || dataset.Columns[1].Type != "STRING" || dataset.Columns[2].Type != "BLOB" {
				t.Fatalf("case-insensitive types were not normalized: %+v", dataset.Columns)
			}
			if dataset.Columns[2].Encoding != "base64" || dataset.ConstColumns[2].Encoding != "base64" {
				t.Fatalf("BLOB encoding was not normalized: %+v %+v", dataset.Columns, dataset.ConstColumns)
			}
			if dataset.ConstColumns[0].Value.State != "missing" || dataset.ConstColumns[1].Value.State != "empty" {
				t.Fatalf("optional ConstColumn values collapsed: %+v", dataset.ConstColumns)
			}

			encoded, err := Encode(protocol.Value{Parameters: []protocol.Parameter{}, Datasets: []protocol.Dataset{{
				ID: "blob", Columns: []protocol.Column{{ID: "b", Type: "blob"}},
				ConstColumns: []protocol.ConstColumn{{ID: "c", Type: "BlOb", Value: protocol.Cell{State: "value", Lexical: "eA=="}}},
				Rows:         []protocol.Row{},
			}}}, profile.name)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Count(encoded, []byte(`enc="base64"`)) != 2 {
				t.Fatalf("BLOB encoding contract not emitted: %s", encoded)
			}
		})
	}
}

func TestXMLStrictRejectsBlobWithoutBase64Encoding(t *testing.T) {
	for _, element := range []string{
		`<Column id="b" type="BLOB"/>`,
		`<ConstColumn id="b" type="BLOB" value="eA=="/>`,
	} {
		wire := []byte(`<?xml version="1.0" encoding="utf-8"?><Root xmlns="http://www.nexacroplatform.com/platform/dataset" ver="4000"><Dataset id="d"><ColumnInfo>` + element + `</ColumnInfo></Dataset></Root>`)
		if _, err := DecodeProfile(wire, "nexacro-xml-4000", DecodeOptions{Strict: true}); err == nil {
			t.Fatalf("BLOB without base64 encoding accepted: %s", wire)
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
