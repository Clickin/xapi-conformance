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
	if v.Parameters[0].Lexical != "12:34:56.123" || v.Datasets[0].Rows[0].Values["x"].Lexical != "a<b" {
		t.Fatalf("XML lexical value lost: %+v", v)
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

func TestEncodeXMLRoundTripsOrgRowAndConstColumn(t *testing.T) {
	v, err := DecodeProfile([]byte(`<?xml version="1.0" encoding="utf-8"?><Root xmlns="http://www.nexacroplatform.com/platform/dataset" ver="4000"><Dataset id="d"><ColumnInfo><ConstColumn id="c" type="STRING" value="x"/><Column id="a" type="STRING"/></ColumnInfo><Rows><Row type="update"><Col id="a">new</Col><OrgRow><Col id="a">old</Col></OrgRow></Row></Rows></Dataset></Root>`), "nexacro-xml-4000", DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encode(v, "nexacro-xml-4000")
	if err != nil {
		t.Fatal(err)
	}
	round, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(round.Datasets) != 1 || len(round.Datasets[0].ConstColumns) != 1 || round.Datasets[0].Rows[0].OrgRow == nil {
		t.Fatalf("XML metadata/org row lost: %s", b)
	}
}

func TestEncodeXMLPreservesVerAndParameterAttributeForm(t *testing.T) {
	v := protocol.Value{Parameters: []protocol.Parameter{{ID: "p", Type: "STRING", State: "value", Lexical: "x", Wire: map[string]any{"valueForm": "attribute"}}}, Datasets: []protocol.Dataset{}, Wire: map[string]any{"root": map[string]any{"ver": "4000"}}}
	b, err := Encode(v, "nexacro-xml-4000")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`ver="4000"`)) || !bytes.Contains(b, []byte(`value="x"`)) {
		t.Fatalf("wire metadata lost: %s", b)
	}
	round, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if round.Parameters[0].Wire["valueForm"] != "attribute" {
		t.Fatalf("parameter form lost: %+v", round.Parameters[0])
	}
}
