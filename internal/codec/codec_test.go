package codec

import (
	"bytes"
	"testing"

	"github.com/Clickin/xapi-conformance/internal/protocol"
)

func TestDecodeJSONPreservesStatesAndRows(t *testing.T) {
	v, err := Decode([]byte(`{"version":"1.0","Parameters":[{"id":"p","type":"BIGDECIMAL","value":"001.20"}],"Datasets":[{"id":"d","ColumnInfo":{"Column":[{"id":"a","type":"STRING"},{"id":"b","type":"DATE"}]},"Rows":[{"_RowType_":"U","a":"","b":null}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if v.Parameters[0].Lexical != "001.20" || v.Parameters[0].Type != "BIGDECIMAL" {
		t.Fatalf("parameter lexical/type lost: %+v", v.Parameters[0])
	}
	row := v.Datasets[0].Rows[0]
	if row.Type != "U" || row.Values["a"].State != "empty" || row.Values["b"].State != "null" {
		t.Fatalf("row states lost: %+v", row)
	}
}

func TestDecodeXMLBasic(t *testing.T) {
	v, err := DecodeProfile([]byte(`<?xml version="1.0" encoding="utf-8"?><Root xmlns="http://www.nexacroplatform.com/platform/dataset" ver="4000"><Parameters><Parameter id="p" type="TIME">12:34:56.123</Parameter></Parameters><Dataset id="d"><ColumnInfo><Column id="x" type="STRING"/></ColumnInfo><Rows><Row><Col id="x"><![CDATA[a<b]]></Col></Row></Rows></Dataset></Root>`), "nexacro-xml-4000", DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if v.Parameters[0].State != "null" || v.Datasets[0].Rows[0].Values["x"].Lexical != "a<b" {
		t.Fatalf("XML scalar normalization failed: %+v", v)
	}
}

func TestDecodeXMLNormalizesLenientDateTimeLexical(t *testing.T) {
	wire := []byte(`<Root><Dataset id="d"><ColumnInfo><Column id="dt" type="DATETIME"/></ColumnInfo><Rows><Row><Col id="dt">not-a-datetime</Col></Row></Rows></Dataset></Root>`)
	value, err := DecodeProfile(wire, "nexacro-xml-4000", DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	cell := value.Datasets[0].Rows[0].Values["dt"]
	if cell.State != "value" || cell.Lexical != "690190220012803000" {
		t.Fatalf("DATETIME cell = %+v", cell)
	}
}

func TestDecodeRejectsMalformedXML(t *testing.T) {
	if _, err := Decode([]byte(`<Root><Rows></Root>`)); err == nil {
		t.Fatal("malformed XML accepted")
	}
}

func TestDecodeRejectsDuplicateXMLAttributes(t *testing.T) {
	if _, err := Decode([]byte(`<Root><Datasets><Dataset id="d" a="1" a="2"/></Datasets></Root>`)); err == nil {
		t.Fatal("duplicate XML attribute accepted")
	}
}

func TestEncodeXMLWritesOrgRowAndConstColumn(t *testing.T) {
	value := protocol.Value{Datasets: []protocol.Dataset{{
		ID:           "d",
		Columns:      []protocol.Column{{ID: "a", Type: "STRING"}},
		ConstColumns: []protocol.ConstColumn{{ID: "c", Type: "STRING", Value: protocol.Cell{State: "value", Lexical: "x"}}},
		Rows: []protocol.Row{{
			Type:   "U",
			Values: map[string]protocol.Cell{"a": {State: "value", Lexical: "new"}},
			OrgRow: &protocol.Row{
				Type:   "O",
				Values: map[string]protocol.Cell{"a": {State: "value", Lexical: "old"}},
			},
		}},
	}}}
	wire, err := Encode(value, "nexacro-xml-4000")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(wire, []byte(`<ConstColumn id="c" type="string" size="32" value="x"/>`)) ||
		!bytes.Contains(wire, []byte(`<Row type="update">`)) ||
		!bytes.Contains(wire, []byte("<OrgRow>")) ||
		!bytes.Contains(wire, []byte(`>old</Col>`)) {
		t.Fatalf("XML metadata/org row missing: %s", wire)
	}
}

func TestEncodeXMLUsesProfileFramingAndTextParameters(t *testing.T) {
	v := protocol.Value{Parameters: []protocol.Parameter{{ID: "p", Type: "STRING", State: "value", Lexical: "x", Wire: map[string]any{"valueForm": "attribute"}}}, Datasets: []protocol.Dataset{}, Wire: map[string]any{"root": map[string]any{"ver": "4000"}}}
	b, err := Encode(v, "nexacro-xml-4000")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte(`ver="4000"`)) || bytes.Contains(b, []byte(`value="x"`)) || !bytes.Contains(b, []byte(`>x</Parameter>`)) {
		t.Fatalf("unexpected Nexacro XML framing: %s", b)
	}
}
