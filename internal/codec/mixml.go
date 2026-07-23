package codec

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/Clickin/xapi-conformance/internal/protocol"
)

const miXMLProfile = "miplatform-xml"

type miXMLDocument struct {
	XMLName  xml.Name       `xml:"root"`
	Params   []miXMLParam   `xml:"params>param"`
	Datasets []miXMLDataset `xml:"dataset"`
}

type miXMLParam struct {
	ID    string `xml:"id,attr"`
	Type  string `xml:"type,attr"`
	Empty string `xml:"empty,attr"`
	Text  string `xml:",chardata"`
}

type miXMLDataset struct {
	ID      string        `xml:"id,attr"`
	Columns []miXMLColumn `xml:"colinfo"`
	Consts  []miXMLColumn `xml:"column"`
	Records []miXMLRecord `xml:"record"`
}

type miXMLColumn struct {
	ID      string `xml:"id,attr"`
	Type    string `xml:"type,attr"`
	Size    string `xml:"size,attr"`
	Encrypt string `xml:"encrypt,attr"`
	Empty   string `xml:"empty,attr"`
	Text    string `xml:",chardata"`
}

type miXMLRecord struct {
	Type  string      `xml:"type,attr"`
	Cells []miXMLCell `xml:",any"`
}

type miXMLCell struct {
	XMLName xml.Name
	Empty   string      `xml:"empty,attr"`
	Text    string      `xml:",chardata"`
	Cells   []miXMLCell `xml:",any"`
}

func miXMLValue(input []byte, strict bool) (protocol.Value, error) {
	if bytes.Contains(bytes.ToUpper(input), []byte("<!DOCTYPE")) {
		return protocol.Value{}, fmt.Errorf("DTD is not allowed")
	}
	var document miXMLDocument
	decoder := xml.NewDecoder(bytes.NewReader(input))
	if err := decoder.Decode(&document); err != nil {
		return protocol.Value{}, err
	}
	if document.XMLName.Local != "root" {
		return protocol.Value{}, fmt.Errorf("MiXml root must be root")
	}
	value := protocol.Value{
		Parameters: make([]protocol.Parameter, 0, len(document.Params)),
		Datasets:   make([]protocol.Dataset, 0, len(document.Datasets)),
		Wire:       map[string]any{"format": "MiXml"},
	}
	for i, item := range document.Params {
		if strict && item.ID == "" {
			return protocol.Value{}, fmt.Errorf("MiXml param.id is required")
		}
		state, lexical := miXMLState(item.Empty, item.Text)
		value.Parameters = append(value.Parameters, protocol.Parameter{
			ID: item.ID, Type: miCanonicalType(item.Type), State: state, Lexical: lexical, Index: i,
		})
	}
	for _, item := range document.Datasets {
		if strict && item.ID == "" {
			return protocol.Value{}, fmt.Errorf("MiXml dataset.id is required")
		}
		dataset := protocol.Dataset{
			ID: item.ID, Columns: make([]protocol.Column, 0, len(item.Columns)),
			ConstColumns: make([]protocol.ConstColumn, 0, len(item.Consts)),
			Rows:         make([]protocol.Row, 0, len(item.Records)),
			Wire:         map[string]any{"format": "MiXml", "alias": item.ID},
		}
		for i, column := range item.Columns {
			if strict && column.ID == "" {
				return protocol.Value{}, fmt.Errorf("MiXml colinfo.id is required")
			}
			typeName := miCanonicalType(column.Type)
			if strict && typeName == "BLOB" && !strings.EqualFold(column.Encrypt, "base64") {
				return protocol.Value{}, fmt.Errorf("MiXml BLOB colinfo requires base64 encryption")
			}
			dataset.Columns = append(dataset.Columns, protocol.Column{
				ID: column.ID, Type: typeName, Index: i, Size: column.Size,
				Encoding: miEncoding(typeName, column.Encrypt),
			})
		}
		for _, column := range item.Consts {
			if strict && column.ID == "" {
				return protocol.Value{}, fmt.Errorf("MiXml column.id is required")
			}
			typeName := miCanonicalType(column.Type)
			state, lexical := miXMLState(column.Empty, column.Text)
			dataset.ConstColumns = append(dataset.ConstColumns, protocol.ConstColumn{
				ID: column.ID, Type: typeName, Size: column.Size,
				Encoding: miEncoding(typeName, column.Encrypt),
				Value:    protocol.Cell{State: state, Lexical: lexical},
			})
		}
		known := make(map[string]bool, len(dataset.Columns))
		for _, column := range dataset.Columns {
			known[column.ID] = true
		}
		for _, record := range item.Records {
			row := protocol.Row{Type: miRowType(record.Type), Values: map[string]protocol.Cell{}}
			for _, cell := range record.Cells {
				if cell.XMLName.Local == "org_record" {
					org := protocol.Row{Type: "O", Values: map[string]protocol.Cell{}}
					for _, saved := range cell.Cells {
						if strict && !known[saved.XMLName.Local] {
							return protocol.Value{}, fmt.Errorf("MiXml org column %q is not declared", saved.XMLName.Local)
						}
						state, lexical := miXMLState(saved.Empty, saved.Text)
						org.Values[saved.XMLName.Local] = protocol.Cell{State: state, Lexical: lexical}
					}
					row.OrgRow = &org
					continue
				}
				if strict && !known[cell.XMLName.Local] {
					return protocol.Value{}, fmt.Errorf("MiXml row column %q is not declared", cell.XMLName.Local)
				}
				state, lexical := miXMLState(cell.Empty, cell.Text)
				row.Values[cell.XMLName.Local] = protocol.Cell{State: state, Lexical: lexical}
			}
			for _, column := range dataset.Columns {
				if _, ok := row.Values[column.ID]; !ok {
					row.Values[column.ID] = protocol.Cell{State: "missing"}
				}
			}
			dataset.Rows = append(dataset.Rows, row)
		}
		value.Datasets = append(value.Datasets, dataset)
	}
	return value, nil
}

func miXMLState(empty, text string) (string, string) {
	if strings.EqualFold(empty, "true") {
		return "null", ""
	}
	if text == "" {
		return "empty", ""
	}
	return "value", text
}

func miCanonicalType(dataType string) string {
	switch strings.ToUpper(dataType) {
	case "INT":
		return "INT"
	case "DECIMAL":
		return "DOUBLE"
	case "CURRENCY":
		return "BIGDECIMAL"
	case "DATE":
		return "DATETIME"
	case "BLOB", "FILE":
		return "BLOB"
	default:
		return "STRING"
	}
}

func miEncoding(dataType, encoding string) string {
	if dataType == "BLOB" && strings.EqualFold(encoding, "base64") {
		return "base64"
	}
	return ""
}

func miRowType(rowType string) string {
	switch rowType {
	case "insert":
		return "I"
	case "update":
		return "U"
	case "delete":
		return "D"
	default:
		return "N"
	}
}

func encodeMiXML(value protocol.Value) ([]byte, error) {
	var out strings.Builder
	out.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<root>\n")
	if len(value.Parameters) == 0 {
		out.WriteString("\t<params/>\n")
	} else {
		out.WriteString("\t<params>\n")
		for _, parameter := range value.Parameters {
			out.WriteString("\t\t<param id=\"")
			out.WriteString(escapeMiXMLAttribute(parameter.ID))
			out.WriteString("\" type=\"")
			out.WriteString(miWireType(parameter.Type))
			if isBlobType(parameter.Type) {
				out.WriteString("\" encrypt=\"base64")
			}
			if parameter.State == "missing" || parameter.State == "null" {
				out.WriteString("\"/>\n")
				continue
			}
			out.WriteString("\">")
			out.WriteString(miXMLLexical(parameter.Type, parameter.Lexical))
			out.WriteString("</param>\n")
		}
		out.WriteString("\t</params>\n")
	}
	for _, dataset := range value.Datasets {
		out.WriteString("\t<dataset id=\"")
		out.WriteString(escapeMiXMLAttribute(miDatasetAlias(dataset)))
		out.WriteString("\">\n")
		for _, column := range dataset.ConstColumns {
			out.WriteString("\t\t<column id=\"")
			out.WriteString(escapeMiXMLAttribute(column.ID))
			out.WriteString("\" type=\"")
			out.WriteString(miWireType(column.Type))
			out.WriteString("\" size=\"")
			out.WriteString(escapeMiXMLAttribute(miSize(column.Size)))
			if column.Value.State == "missing" || column.Value.State == "null" {
				out.WriteString("\"/>\n")
				continue
			}
			if isBlobType(column.Type) {
				out.WriteString("\" encrypt=\"base64")
			}
			out.WriteString("\">")
			out.WriteString(miXMLLexical(column.Type, column.Value.Lexical))
			out.WriteString("</column>\n")
		}
		for _, column := range dataset.Columns {
			out.WriteString("\t\t<colinfo id=\"")
			out.WriteString(escapeMiXMLAttribute(column.ID))
			out.WriteString("\" type=\"")
			out.WriteString(miWireType(column.Type))
			out.WriteString("\" size=\"")
			out.WriteString(escapeMiXMLAttribute(miSize(column.Size)))
			if isBlobType(column.Type) {
				out.WriteString("\" encrypt=\"base64")
			}
			out.WriteString("\"/>\n")
		}
		for _, row := range dataset.Rows {
			out.WriteString("\t\t<record")
			switch strings.ToUpper(row.Type) {
			case "I":
				out.WriteString(" type=\"insert\"")
			case "U":
				out.WriteString(" type=\"update\"")
			case "D":
				out.WriteString(" type=\"delete\"")
			}
			out.WriteString(">\n")
			writeMiXMLRow(&out, dataset.Columns, row.Values, 3)
			if strings.EqualFold(row.Type, "U") && row.OrgRow != nil {
				out.WriteString("\t\t\t<org_record>\n")
				writeMiXMLRow(&out, dataset.Columns, row.OrgRow.Values, 4)
				out.WriteString("\t\t\t</org_record>\n")
			}
			out.WriteString("\t\t</record>\n")
		}
		out.WriteString("\t</dataset>\n")
	}
	out.WriteString("</root>\n")
	return []byte(out.String()), nil
}

func writeMiXMLRow(out *strings.Builder, columns []protocol.Column, values map[string]protocol.Cell, depth int) {
	indent := strings.Repeat("\t", depth)
	for _, column := range columns {
		cell, ok := values[column.ID]
		out.WriteString(indent)
		out.WriteByte('<')
		out.WriteString(column.ID)
		if !ok || cell.State == "missing" || cell.State == "null" {
			out.WriteString("/>\n")
			continue
		}
		out.WriteByte('>')
		out.WriteString(miXMLLexical(column.Type, cell.Lexical))
		out.WriteString("</")
		out.WriteString(column.ID)
		out.WriteString(">\n")
	}
}

func miWireType(dataType string) string {
	switch strings.ToUpper(defaultType(dataType)) {
	case "SHORT", "USHORT", "INT", "UINT", "BOOLEAN":
		return "INT"
	case "LONG", "ULONG":
		return "CURRENCY"
	case "FLOAT", "DOUBLE":
		return "DECIMAL"
	case "DATE", "TIME", "DATETIME":
		return "DATE"
	case "BLOB", "FILE":
		return "BLOB"
	default:
		return "STRING"
	}
}

func miXMLLexical(dataType, lexical string) string {
	if strings.EqualFold(dataType, "BOOLEAN") {
		if miBoolean(lexical) {
			lexical = "1"
		} else {
			lexical = "0"
		}
	}
	if isBlobType(dataType) {
		return lexical
	}
	return escapeMiXMLText(lexical)
}

func miBoolean(value string) bool {
	switch value {
	case "true", "True", "TRUE", "yes", "Yes", "YES", "y", "Y", "on", "On", "ON", "1":
		return true
	default:
		return false
	}
}

func miDatasetAlias(dataset protocol.Dataset) string {
	if dataset.Wire != nil {
		if alias, ok := dataset.Wire["alias"].(string); ok && alias != "" {
			return alias
		}
	}
	return dataset.ID
}

func miSize(size string) string {
	if size == "" {
		return "0"
	}
	return size
}

func escapeMiXMLText(value string) string {
	var out strings.Builder
	for _, ch := range value {
		switch ch {
		case ' ':
			out.WriteString("&#32;")
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		case '&':
			out.WriteString("&amp;")
		case '"':
			out.WriteString("&quot;")
		case '\'':
			out.WriteString("&apos;")
		default:
			if ch < ' ' {
				fmt.Fprintf(&out, "&#%d;", ch)
			} else {
				out.WriteRune(ch)
			}
		}
	}
	return out.String()
}

func escapeMiXMLAttribute(value string) string {
	var out strings.Builder
	for _, ch := range value {
		switch ch {
		case '&':
			out.WriteString("&amp;")
		case '<':
			out.WriteString("&lt;")
		case '"':
			out.WriteString("&quot;")
		default:
			out.WriteRune(ch)
		}
	}
	return out.String()
}
