package codec

import (
	"bytes"
	"testing"

	"github.com/Clickin/xapi-conformance/internal/protocol"
)

func TestPlatformBinaryRoundtrip(t *testing.T) {
	value := protocol.Value{
		Parameters: []protocol.Parameter{
			{ID: "message", Type: "STRING", State: "value", Lexical: "hello"},
			{ID: "count", Type: "INT", State: "value", Lexical: "42"},
		},
		Datasets: []protocol.Dataset{{
			ID:   "orders",
			Wire: map[string]any{"alias": "ordersAlias"},
			Columns: []protocol.Column{
				{ID: "name", Type: "STRING", Index: 0, Size: "20"},
				{ID: "amount", Type: "INT", Index: 1, Size: "10"},
				{ID: "created", Type: "DATETIME", Index: 2, Size: "30"},
			},
			Rows: []protocol.Row{{
				Type: "N",
				Values: map[string]protocol.Cell{
					"name":    {State: "value", Lexical: "alpha"},
					"amount":  {State: "value", Lexical: "7"},
					"created": {State: "value", Lexical: "1735689600000"},
				},
			}},
		}},
	}
	for _, profile := range []string{nexacroBinaryProfile, xplatformBinaryProfile} {
		wire, err := Encode(value, profile)
		if err != nil {
			t.Fatalf("encode %s: %v", profile, err)
		}
		if !bytes.HasPrefix(wire, []byte{0xfe, 0x10}) {
			t.Fatalf("%s marker = %x", profile, wire[:2])
		}
		decoded, err := DecodeProfile(wire, profile, DecodeOptions{Strict: true})
		if err != nil {
			t.Fatalf("decode %s: %v", profile, err)
		}
		if decoded.Parameters[0].Lexical != "hello" || decoded.Parameters[1].Lexical != "42" {
			t.Fatalf("parameters = %+v", decoded.Parameters)
		}
		if decoded.Datasets[0].ID != "ordersAlias" || decoded.Datasets[0].Rows[0].Values["amount"].Lexical != "7" {
			t.Fatalf("dataset = %+v", decoded.Datasets[0])
		}
	}
}

func TestPlatformBinaryEmptyDocumentHasVariableBlock(t *testing.T) {
	for _, profile := range []string{nexacroBinaryProfile, xplatformBinaryProfile} {
		wire, err := Encode(protocol.Value{Parameters: []protocol.Parameter{}, Datasets: []protocol.Dataset{}}, profile)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(wire, []byte{0xfe, 0x10, 0x13, 0x88, 0, 2, 0, 0}) {
			t.Fatalf("%s empty wire = %x", profile, wire)
		}
		decoded, err := DecodeProfile(wire, profile, DecodeOptions{Strict: true})
		if err != nil || len(decoded.Parameters) != 0 || len(decoded.Datasets) != 0 {
			t.Fatalf("%s empty decode = %+v, %v", profile, decoded, err)
		}
	}
}

func TestPlatformBinaryRejectsMalformedLength(t *testing.T) {
	_, err := DecodeProfile([]byte{0xfe, 0x10, 0x13, 0x88, 0xff, 0xff}, nexacroBinaryProfile, DecodeOptions{Strict: true})
	if err == nil {
		t.Fatal("expected malformed binary length error")
	}
}
