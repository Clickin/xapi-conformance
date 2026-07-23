package codec

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
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

func TestPlatformBinaryEmptyDocumentHasNoFraming(t *testing.T) {
	for _, profile := range []string{nexacroBinaryProfile, xplatformBinaryProfile} {
		wire, err := Encode(protocol.Value{Parameters: []protocol.Parameter{}, Datasets: []protocol.Dataset{}}, profile)
		if err != nil {
			t.Fatal(err)
		}
		if len(wire) != 0 {
			t.Fatalf("%s empty wire = %x", profile, wire)
		}
		decoded, err := DecodeProfile(wire, profile, DecodeOptions{Strict: true})
		if err != nil || len(decoded.Parameters) != 0 || len(decoded.Datasets) != 0 {
			t.Fatalf("%s empty decode = %+v, %v", profile, decoded, err)
		}
	}
}

func TestPlatformBinaryInternalTypeEncoding(t *testing.T) {
	value := protocol.Value{
		Parameters: []protocol.Parameter{
			{ID: "u", Type: "UNDEFINED", State: "empty"},
			{ID: "n", Type: "NULL", State: "empty", Index: 1},
			{ID: "d", Type: "DATASET", State: "empty", Index: 2},
			{ID: "i", Type: "INVALID", State: "empty", Index: 3},
		},
		Datasets: []protocol.Dataset{},
	}
	expected, err := base64.StdEncoding.DecodeString("/hATiAAcAAQAAXUAFQAAAAFuAAAAAWQAFQAAAAFpABUAAA==")
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{nexacroBinaryProfile, xplatformBinaryProfile} {
		wire, err := Encode(value, profile)
		if err != nil {
			t.Fatalf("encode %s: %v", profile, err)
		}
		if !bytes.Equal(wire, expected) {
			t.Fatalf("%s internal type wire = %x, want %x", profile, wire, expected)
		}
	}
}

func TestPlatformBinaryDecodedCanonicalMetadata(t *testing.T) {
	tests := []struct {
		name     string
		base64   string
		expected string
	}{
		{
			name:     "parameters omit indexes",
			base64:   "/hATiAATAAIAAXAAFQABeAABcQADAAAABw==",
			expected: `{"parameters":[{"id":"p","type":"STRING","state":"value","lexical":"x"},{"id":"q","type":"INT","state":"value","lexical":"7"}],"datasets":[]}`,
		},
		{
			name:     "dataset omits wire and declaration details",
			base64:   "/gETiAAeAAFk/hATiAAKAAEAAWMAFQABeAABAAFhAAEAIAABAAAAAA==",
			expected: `{"parameters":[],"datasets":[{"id":"d","columns":[{"id":"a","type":"STRING","index":1}],"constColumns":[{"id":"c","type":"STRING","index":0}],"rows":[]}]}`,
		},
	}
	for _, profile := range []string{nexacroBinaryProfile, xplatformBinaryProfile} {
		for _, test := range tests {
			t.Run(profile+"/"+test.name, func(t *testing.T) {
				wire, err := base64.StdEncoding.DecodeString(test.base64)
				if err != nil {
					t.Fatal(err)
				}
				value, err := DecodeProfile(wire, profile, DecodeOptions{Strict: true})
				if err != nil {
					t.Fatal(err)
				}
				canonical, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				if string(canonical) != test.expected {
					t.Fatalf("canonical = %s, want %s", canonical, test.expected)
				}
			})
		}
	}
}

func TestPlatformBinaryRejectsMalformedLength(t *testing.T) {
	_, err := DecodeProfile([]byte{0xfe, 0x10, 0x13, 0x88, 0xff, 0xff}, nexacroBinaryProfile, DecodeOptions{Strict: true})
	if err == nil {
		t.Fatal("expected malformed binary length error")
	}
}
