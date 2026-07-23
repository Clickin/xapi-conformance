package codec

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Clickin/xapi-conformance/internal/protocol"
)

func TestSaveTypeFiltersCanonicalRows(t *testing.T) {
	baseRows := []protocol.Row{
		{Type: "N", Values: map[string]protocol.Cell{"a": {State: "value", Lexical: "normal"}}},
		{Type: "I", Values: map[string]protocol.Cell{"a": {State: "value", Lexical: "inserted"}}},
		{Type: "U", OrgRow: &protocol.Row{Type: "O", Values: map[string]protocol.Cell{"a": {State: "value", Lexical: "old"}}}, Values: map[string]protocol.Cell{"a": {State: "value", Lexical: "updated"}}},
		{Type: "D", Values: map[string]protocol.Cell{"a": {State: "value", Lexical: "deleted"}}},
	}
	cases := []struct {
		saveType int
		want     []string
	}{
		{0, []string{"normal", "inserted", "updated", "deleted"}},
		{1, []string{"normal", "inserted", "updated", "deleted"}},
		{2, []string{"normal", "inserted", "updated"}},
		{3, []string{"inserted", "updated"}},
		{4, []string{"deleted"}},
		{5, []string{"inserted", "updated", "deleted"}},
	}
	for _, tc := range cases {
		value := protocol.Value{
			SaveType: tc.saveType,
			Datasets: []protocol.Dataset{{
				ID: "d", Columns: []protocol.Column{{ID: "a", Type: "STRING", Index: 0}},
				ConstColumns: []protocol.ConstColumn{}, Rows: append([]protocol.Row(nil), baseRows...),
			}},
		}
		wire, err := Encode(value, "nexacro-json-1.0")
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodeProfile(wire, "nexacro-json-1.0", DecodeOptions{Strict: true})
		if err != nil {
			t.Fatal(err)
		}
		rows := decoded.Datasets[0].Rows
		if len(rows) != len(tc.want) {
			t.Fatalf("saveType %d row count = %d, want %d; wire=%s", tc.saveType, len(rows), len(tc.want), wire)
		}
		for i, row := range rows {
			if row.Type != "N" {
				t.Fatalf("saveType %d row %d type = %q, want N; wire=%s", tc.saveType, i, row.Type, wire)
			}
			if got := row.Values["a"].Lexical; got != tc.want[i] {
				t.Fatalf("saveType %d row %d value = %q, want %q; wire=%s", tc.saveType, i, got, tc.want[i], wire)
			}
		}
	}
}

func TestDatasetSaveTypeOverridesRoot(t *testing.T) {
	value := protocol.Value{
		SaveType: 4,
		Datasets: []protocol.Dataset{{
			ID: "d", SaveType: 2, Columns: []protocol.Column{{ID: "a", Type: "STRING", Index: 0}},
			ConstColumns: []protocol.ConstColumn{},
			Rows: []protocol.Row{
				{Type: "N", Values: map[string]protocol.Cell{"a": {State: "value", Lexical: "normal"}}},
				{Type: "D", Values: map[string]protocol.Cell{"a": {State: "value", Lexical: "deleted"}}},
			},
		}},
	}
	wire, err := Encode(value, "nexacro-json-1.0")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeProfile(wire, "nexacro-json-1.0", DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Datasets[0].Rows) != 1 || decoded.Datasets[0].Rows[0].Type != "N" {
		t.Fatalf("dataset saveType override = %+v", decoded.Datasets[0].Rows)
	}
}

func TestSourceScalarLexicalsNormalizeOnEncode(t *testing.T) {
	value := protocol.Value{Parameters: []protocol.Parameter{
		{ID: "hex", Type: "INT", State: "value", Lexical: "0x10"},
		{ID: "grouped", Type: "INT", State: "value", Lexical: "1,234.9", Index: 1},
		{ID: "truthy", Type: "BOOLEAN", State: "value", Lexical: "YES", Index: 2},
		{ID: "falsey", Type: "BOOLEAN", State: "value", Lexical: "not-a-boolean", Index: 3},
		{ID: "float", Type: "DOUBLE", State: "value", Lexical: "1,234.5", Index: 4},
		{ID: "invalid", Type: "DOUBLE", State: "value", Lexical: "not-a-number", Index: 5},
		{ID: "decimal", Type: "BIGDECIMAL", State: "value", Lexical: "001.2300", Index: 6},
	}}
	wire, err := Encode(value, "nexacro-json-1.0")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Parameters []struct {
			Value any `json:"value"`
		} `json:"Parameters"`
	}
	if err := json.Unmarshal(wire, &document); err != nil {
		t.Fatal(err)
	}
	want := []any{"16", "1234", "1", "0", "1234.5", "0.0", "1.2300"}
	for i, expected := range want {
		if document.Parameters[i].Value != expected {
			t.Fatalf("parameter %d value = %#v, want %#v; wire=%s", i, document.Parameters[i].Value, expected, wire)
		}
	}
	xmlWire, err := Encode(protocol.Value{Parameters: []protocol.Parameter{{ID: "b", Type: "BOOLEAN", State: "value", Lexical: "on"}}}, "nexacro-xml-4000")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(xmlWire, []byte(`type="int">1</Parameter>`)) {
		t.Fatalf("boolean XML = %s", xmlWire)
	}
}

func TestFileAndJSONBlobContracts(t *testing.T) {
	value := protocol.Value{Parameters: []protocol.Parameter{{ID: "file", Type: "FILE", State: "value", Lexical: "eA=="}}}
	jsonWire, err := Encode(value, "nexacro-json-1.0")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(jsonWire, []byte(`"id":"file","type":"blob","value":"eA=="`)) {
		t.Fatalf("FILE JSON = %s", jsonWire)
	}
	invalidJSON := []byte(`{"version":"1.0","Parameters":[{"id":"blob","type":"blob","value":"%%%"}],"Datasets":[]}`)
	decoded, err := DecodeProfile(invalidJSON, "nexacro-json-1.0", DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	blob := decoded.Parameters[0]
	if blob.Type != "BLOB" || blob.State != "value" || blob.Lexical != "JSUl" {
		t.Fatalf("decoded JSON BLOB = %+v", blob)
	}
	for _, profile := range []string{"nexacro-json-1.0", "nexacro-xml-4000", nexacroSSVProfile, nexacroBinaryProfile} {
		_, err := Encode(protocol.Value{Parameters: []protocol.Parameter{{ID: "blob", Type: "BLOB", State: "value", Lexical: "%%%"}}}, profile)
		if err == nil {
			t.Fatalf("%s accepted invalid canonical BLOB", profile)
		}
	}
}
