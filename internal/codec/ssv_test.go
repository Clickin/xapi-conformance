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
	if dataset.Columns[0].Prop != "SUM" || dataset.Columns[0].SumText != "total" {
		t.Fatalf("summary metadata lost: %+v", dataset.Columns[0])
	}
	if dataset.Rows[0].OrgRow == nil || dataset.Rows[0].OrgRow.Values["a"].Lexical != "old" {
		t.Fatalf("original row lost: %+v", dataset.Rows)
	}
	if dataset.Rows[2].Values["a"].State != "empty" || dataset.Rows[2].Values["b"].State != "missing" {
		t.Fatalf("Nexacro cell states lost: %+v", dataset.Rows[2].Values)
	}
}

func TestDecodeXPlatformSSVPreservesLegacyCellStates(t *testing.T) {
	wire := "SSV\x1eDataset:d\x1e_RowType_\x1fa:STRING\x1fb:STRING\x1eN\x1f\x1f\x02\x1e"
	value, err := DecodeProfile([]byte(wire), xplatformSSVProfile, DecodeOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	cells := value.Datasets[0].Rows[0].Values
	if cells["a"].State != "null" || cells["b"].State != "empty" {
		t.Fatalf("XPlatform cell states lost: %+v", cells)
	}
}

func TestDecodeNexacroSSVCustomSeparators(t *testing.T) {
	wire := "SSV:utf-8~Dataset:d~_RowType_|a:STRING~N|x~~"
	value, err := DecodeProfile([]byte(wire), nexacroSSVProfile, DecodeOptions{
		Strict: true, SSVUnitSeparator: "|", SSVRecordSeparator: "~",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := value.Datasets[0].Rows[0].Values["a"].Lexical; got != "x" {
		t.Fatalf("custom separators decoded %q", got)
	}
}

func TestEncodeSSVUsesProfileCellStatesAndFraming(t *testing.T) {
	value := protocol.Value{Parameters: []protocol.Parameter{}, Datasets: []protocol.Dataset{{
		ID: "d",
		Columns: []protocol.Column{
			{ID: "a", Type: "STRING", Index: 0},
			{ID: "b", Type: "STRING", Index: 1},
		},
		ConstColumns: []protocol.ConstColumn{},
		Rows: []protocol.Row{{Type: "N", Values: map[string]protocol.Cell{
			"a": {State: "empty"},
			"b": {State: "missing"},
		}}},
	}}}

	nexacro, err := Encode(value, nexacroSSVProfile)
	if err != nil {
		t.Fatal(err)
	}
	nexacroExpected := []byte("SSV:utf-8\x1eDataset:d\x1e_RowType_\x1fa:STRING\x1fb:STRING\x1eN\x1f\x1f\x03\x1e\x1e")
	if !bytes.Equal(nexacro, nexacroExpected) {
		t.Fatalf("Nexacro SSV:\nwant %q\n got %q", nexacroExpected, nexacro)
	}

	xplatform, err := Encode(value, xplatformSSVProfile)
	if err != nil {
		t.Fatal(err)
	}
	xplatformExpected := []byte("SSV:utf-8\x1eDataset:d\x1e_RowType_\x1fa:STRING\x1fb:STRING\x1eN\x1f\x02\x1f\x1e")
	if !bytes.Equal(xplatform, xplatformExpected) {
		t.Fatalf("XPlatform SSV:\nwant %q\n got %q", xplatformExpected, xplatform)
	}
}

func TestDecodeNexacroSSVRequiresNullRecord(t *testing.T) {
	wire := []byte("SSV\x1eDataset:d\x1e_RowType_\x1fa:STRING\x1eN\x1fx\x1e")
	if _, err := DecodeProfile(wire, nexacroSSVProfile, DecodeOptions{Strict: true}); err == nil {
		t.Fatal("missing SSV null record accepted")
	}
}

func TestDecodeSSVRejectsConstantsAfterColumns(t *testing.T) {
	wire := []byte("SSV\x1eDataset:d\x1e_RowType_\x1fa:STRING\x1e_Const_\x1fc:STRING=x\x1e\x1e")
	if _, err := DecodeProfile(wire, nexacroSSVProfile, DecodeOptions{Strict: true}); err == nil {
		t.Fatal("late constant header accepted")
	}
}
