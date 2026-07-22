package protocol

import (
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
