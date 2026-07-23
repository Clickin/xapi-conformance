package codec

import (
	"bytes"
	"testing"

	"github.com/Clickin/xapi-conformance/internal/protocol"
)

func TestDecodeNexacroSSVDocumentedLayout(t *testing.T) {
	wire := "SSV:utf-8\x1e" +
		"p:INT=1\x1e" +
		"Dataset:d\x1e" +
		"_Const_\x1fc:STRING(5)=fixed\x1e" +
		"_RowType_\x1fa:STRING(10):SUM:total\x1fb:INT\x1e" +
		"U\x1fnew\x1f1\x1e" +
		"O\x1fold\x1f1\x1e" +
		"N\x1f\x1f\x03\x1e" +
		"\x1e"

	value, err := DecodeProfile([]byte(wire), nexacroSSVProfile, DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if value.Parameters[0].ID != "p" || value.Parameters[0].Type != "INT" || value.Parameters[0].Lexical != "1" {
		t.Fatalf("parameter lost: %+v", value.Parameters)
	}
	dataset := value.Datasets[0]
	if dataset.Columns[0].Prop != "" || dataset.Columns[0].SumText != "" {
		t.Fatalf("summary metadata leaked: %+v", dataset.Columns[0])
	}
	if dataset.ConstColumns[0].Index != 2 || dataset.ConstColumns[0].Value.State != "" {
		t.Fatalf("constant metadata mismatch: %+v", dataset.ConstColumns[0])
	}
	if dataset.Rows[0].OrgRow != nil {
		t.Fatalf("SSV original row leaked: %+v", dataset.Rows)
	}
	if dataset.Rows[0].Values["c"].Lexical != "fixed" {
		t.Fatalf("constant value missing from row: %+v", dataset.Rows[0].Values)
	}
	if dataset.Rows[1].Values["a"].State != "empty" || dataset.Rows[1].Values["b"].State != "null" {
		t.Fatalf("Nexacro cell states lost: %+v", dataset.Rows[1].Values)
	}
}

func TestDecodeNexacroSSVPackedVariables(t *testing.T) {
	wire := "SSV:utf-8\x1ep:INT=1\x1fq:STRING=two\x1e\x1e"
	value, err := DecodeProfile([]byte(wire), nexacroSSVProfile, DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Parameters) != 2 || value.Parameters[0].ID != "p" || value.Parameters[1].ID != "q" {
		t.Fatalf("packed variables lost: %+v", value.Parameters)
	}
}

func TestDecodeSSVLatin1CodePage(t *testing.T) {
	wire := []byte("SSV:iso-8859-1\x1ep:STRING=")
	wire = append(wire, 0xe9)
	wire = append(wire, []byte("\x1e\x1e")...)
	value, err := DecodeProfile(wire, nexacroSSVProfile, DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := value.Parameters[0].Lexical; got != "é" {
		t.Fatalf("latin-1 value = %q", got)
	}
}
func TestDecodeXPlatformSSVPreservesLegacyCellStates(t *testing.T) {
	wire := "SSV\x1eDataset:d\x1e_RowType_\x1fa:STRING\x1fb:STRING\x1eN\x1f\x1f\x02\x1e"
	value, err := DecodeProfile([]byte(wire), xplatformSSVProfile, DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	cells := value.Datasets[0].Rows[0].Values
	if cells["a"].State != "empty" || cells["b"].State != "null" {
		t.Fatalf("XPlatform cell states lost: %+v", cells)
	}
}

func TestDecodeNexacroSSVRejectsCustomSeparators(t *testing.T) {
	wire := "SSV:utf-8~Dataset:d~_RowType_|a:STRING~N|x~~"
	if _, err := DecodeProfile([]byte(wire), nexacroSSVProfile, DecodeOptions{
		Strict: true, SSVUnitSeparator: "|", SSVRecordSeparator: "~",
	}); err == nil {
		t.Fatal("custom SSV separators accepted")
	}
}

func TestEncodeSSVUsesProfileCellStatesAndFraming(t *testing.T) {
	value := protocol.Value{Parameters: []protocol.Parameter{}, Datasets: []protocol.Dataset{{
		ID: "d",
		Columns: []protocol.Column{
			{ID: "a", Type: "STRING", Index: 0},
			{ID: "b", Type: "STRING", Index: 1},
			{ID: "c", Type: "STRING", Index: 2},
		},
		ConstColumns: []protocol.ConstColumn{},
		Rows: []protocol.Row{{Type: "N", Values: map[string]protocol.Cell{
			"a": {State: "empty"},
			"b": {State: "missing"},
			"c": {State: "null"},
		}}},
	}}}

	nexacro, err := Encode(value, nexacroSSVProfile)
	if err != nil {
		t.Fatal(err)
	}
	nexacroExpected := []byte("SSV:UTF-8\x1eDataset:d\x1e_RowType_\x1fa:string(32)\x1fb:string(32)\x1fc:string(32)\x1eN\x1f\x03\x1f\x03\x1f\x03\x1e\x1e")
	if !bytes.Equal(nexacro, nexacroExpected) {
		t.Fatalf("Nexacro SSV:\nwant %q\n got %q", nexacroExpected, nexacro)
	}

	xplatform, err := Encode(value, xplatformSSVProfile)
	if err != nil {
		t.Fatal(err)
	}
	xplatformExpected := []byte("SSV:UTF-8\x1eDataset:d\x1e_RowType_\x1fa:string(32)\x1fb:string(32)\x1fc:string(32)\x1eN\x1f\x1f\x1f\x1e")
	if !bytes.Equal(xplatform, xplatformExpected) {
		t.Fatalf("XPlatform SSV:\nwant %q\n got %q", xplatformExpected, xplatform)
	}
}

func TestDecodeNexacroSSVAllowsMissingNullRecord(t *testing.T) {
	wire := []byte("SSV\x1eDataset:d\x1e_RowType_\x1fa:STRING\x1eN\x1fx\x1e")
	value, err := DecodeProfile(wire, nexacroSSVProfile, DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Datasets) != 1 || len(value.Datasets[0].Rows) != 1 {
		t.Fatalf("unterminated dataset lost: %+v", value.Datasets)
	}
}

func TestDecodeSSVAllowsConstantsAfterColumns(t *testing.T) {
	wire := []byte("SSV\x1eDataset:d\x1e_RowType_\x1fa:STRING\x1e_Const_\x1fc:STRING=x\x1e\x1e")
	value, err := DecodeProfile(wire, nexacroSSVProfile, DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Datasets[0].ConstColumns) != 1 || value.Datasets[0].ConstColumns[0].Index != 1 {
		t.Fatalf("late constant header lost: %+v", value.Datasets[0].ConstColumns)
	}
}

func TestDecodeSSVTypeAndItemForms(t *testing.T) {
	wire := "BAD\x1e" +
		"drop\x1fkeep:FLOAT=v\x1e" +
		"Dataset:d\x1e" +
		"_Const_\x1fdrop:STRING\x1fkeep:DECIMAL=v\x1e" +
		"_RowType_\x1fa:FLOAT\x1fb:DECIMAL\x1fc:UNKNOWN\x1fd:string:SUM:total\x1fe:string(7):SUM\x1e" +
		"N\x1f1\x1f2\x1f3\x1f4\x1f5\x1e\x1e"
	value, err := DecodeProfile([]byte(wire), nexacroSSVProfile, DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if value.Wire != nil || len(value.Parameters) != 1 || value.Parameters[0].Type != "DOUBLE" {
		t.Fatalf("parameter forms mismatch: %+v", value)
	}
	dataset := value.Datasets[0]
	wantTypes := []string{"DOUBLE", "UNDEFINED", "UNDEFINED", "UNDEFINED", "STRING"}
	for i, want := range wantTypes {
		if dataset.Columns[i].Type != want || dataset.Columns[i].Index != i {
			t.Fatalf("column %d = %+v, want type %s", i, dataset.Columns[i], want)
		}
	}
	if len(dataset.ConstColumns) != 1 || dataset.ConstColumns[0].Type != "UNDEFINED" ||
		dataset.ConstColumns[0].Index != len(dataset.Columns) {
		t.Fatalf("constant forms mismatch: %+v", dataset.ConstColumns)
	}
	if got := dataset.Rows[0].Values["keep"]; got.State != "value" || got.Lexical != "v" {
		t.Fatalf("constant value missing from row: %+v", dataset.Rows[0].Values)
	}
}

func TestEncodeSSVHeaderTypesAndXPlatformMissing(t *testing.T) {
	value := protocol.Value{Parameters: []protocol.Parameter{{
		ID: "p", Type: "DOUBLE", State: "value", Lexical: "1.0",
	}}, Datasets: []protocol.Dataset{{
		ID: "d",
		Columns: []protocol.Column{
			{ID: "a", Type: "DOUBLE", Index: 0},
			{ID: "b", Type: "BOOLEAN", Index: 1},
		},
		ConstColumns: []protocol.ConstColumn{{
			ID: "c", Type: "BLOB", Value: protocol.Cell{State: "value", Lexical: "eA=="},
		}},
		Rows: []protocol.Row{{Type: "U", Values: map[string]protocol.Cell{
			"a": {State: "value", Lexical: "1.0"},
			"b": {State: "missing"},
		}}},
	}}}
	got, err := Encode(value, xplatformSSVProfile)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("SSV:UTF-8\x1ep:float=1.0\x1eDataset:d\x1e" +
		"_Const_\x1fc:blob(256)=eA==\x1e" +
		"_RowType_\x1fa:float(8)\x1fb:int(2)\x1eN\x1f1.0\x1f\x1e")
	if !bytes.Equal(got, want) {
		t.Fatalf("XPlatform SSV:\nwant %q\n got %q", want, got)
	}
}

func TestDecodeSSVDropsDeletedAndOriginalRows(t *testing.T) {
	wire := []byte("SSV\x1eDataset:d\x1e_RowType_\x1fa:STRING\x1e" +
		"U\x1fnew\x1eO\x1fold\x1eD\x1fdeleted\x1eN\x1fkept\x1e\x1e")
	value, err := DecodeProfile(wire, nexacroSSVProfile, DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	rows := value.Datasets[0].Rows
	if len(rows) != 2 || rows[0].Type != "U" || rows[0].OrgRow != nil ||
		rows[1].Values["a"].Lexical != "kept" {
		t.Fatalf("row forms mismatch: %+v", rows)
	}
}
