package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeInputWhitespacePolicy(t *testing.T) {
	if _, err := DecodeInput(Input{Encoding: "base64", Data: "eA =="}, false); err == nil {
		t.Fatal("whitespace accepted by default")
	}
	b, err := DecodeInput(Input{Encoding: "base64", Data: "eA =="}, true)
	if err != nil || string(b) != "x" {
		t.Fatalf("whitespace option failed: %q %v", b, err)
	}
}

func TestDecodeJSONRejectsTrailingData(t *testing.T) {
	var v Envelope
	if err := DecodeJSON(strings.NewReader(`{} {}`), &v, 1024); err == nil {
		t.Fatal("trailing JSON accepted")
	}
}

func TestRejectDuplicateKeys(t *testing.T) {
	if err := RejectDuplicateKeys([]byte(`{"a":1,"a":2}`)); err == nil {
		t.Fatal("duplicate key accepted")
	}
	if err := RejectDuplicateKeys([]byte(`{"a":{"b":1,"b":2}}`)); err == nil {
		t.Fatal("nested duplicate key accepted")
	}
}

func TestConstColumnCanonicalShape(t *testing.T) {
	decoded, err := json.Marshal(ConstColumn{ID: "c", Type: "STRING", Index: 0})
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != `{"id":"c","type":"STRING","index":0}` {
		t.Fatalf("decoded shape = %s", decoded)
	}

	encodeInput, err := json.Marshal(ConstColumn{
		ID: "c", Type: "STRING", Index: 3,
		Value: Cell{State: "value", Lexical: "v"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(encodeInput) != `{"id":"c","type":"STRING","value":{"state":"value","lexical":"v"}}` {
		t.Fatalf("encode input shape = %s", encodeInput)
	}
}

func TestCanonicalPresenceRoundTrip(t *testing.T) {
	cell, err := json.Marshal(Cell{State: "empty"})
	if err != nil {
		t.Fatal(err)
	}
	if string(cell) != `{"state":"empty"}` {
		t.Fatalf("omitted empty cell lexical = %s", cell)
	}
	var decodedCell Cell
	if err := json.Unmarshal([]byte(`{"state":"empty","lexical":""}`), &decodedCell); err != nil {
		t.Fatal(err)
	}
	cell, err = json.Marshal(decodedCell)
	if err != nil {
		t.Fatal(err)
	}
	if string(cell) != `{"state":"empty","lexical":""}` {
		t.Fatalf("explicit empty cell lexical = %s", cell)
	}

	var parameter Parameter
	if err := json.Unmarshal([]byte(`{"id":"p","type":"STRING","state":"empty","lexical":""}`), &parameter); err != nil {
		t.Fatal(err)
	}
	encodedParameter, err := json.Marshal(parameter)
	if err != nil {
		t.Fatal(err)
	}
	if string(encodedParameter) != `{"id":"p","type":"STRING","state":"empty","lexical":""}` {
		t.Fatalf("explicit empty parameter lexical = %s", encodedParameter)
	}

	var row Row
	if err := json.Unmarshal([]byte(`{"type":"N","orgRow":null,"values":{}}`), &row); err != nil {
		t.Fatal(err)
	}
	encodedRow, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	if string(encodedRow) != `{"type":"N","orgRow":null,"values":{}}` {
		t.Fatalf("explicit null orgRow = %s", encodedRow)
	}
}

func TestDecodeJSONRejectsUnknownFieldsInCustomValues(t *testing.T) {
	documents := []string{
		`{"parameters":[{"id":"p","type":"STRING","state":"null","bogus":1}],"datasets":[]}`,
		`{"parameters":[],"datasets":[{"id":"d","columns":[],"constColumns":[],"rows":[{"type":"N","values":{},"bogus":1}]}]}`,
		`{"parameters":[],"datasets":[{"id":"d","columns":[],"constColumns":[],"rows":[{"type":"N","values":{"a":{"state":"null","bogus":1}}}]}]}`,
	}
	for _, document := range documents {
		var value Value
		if err := DecodeJSON(strings.NewReader(document), &value, 1024); err == nil {
			t.Fatalf("unknown nested field accepted: %s", document)
		}
	}
}
