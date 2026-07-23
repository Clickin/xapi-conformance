package codec

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Clickin/xapi-conformance/internal/protocol"
)

func validateValueTypes(value protocol.Value) error {
	if value.SaveType < 0 || value.SaveType > 5 {
		return fmt.Errorf("invalid root saveType %d", value.SaveType)
	}
	for _, parameter := range value.Parameters {
		if !isKnownType(defaultType(parameter.Type)) {
			return fmt.Errorf("unsupported parameter type %q", parameter.Type)
		}
	}
	for _, dataset := range value.Datasets {
		if dataset.SaveType < 0 || dataset.SaveType > 5 {
			return fmt.Errorf("invalid Dataset %q saveType %d", dataset.ID, dataset.SaveType)
		}
		for _, column := range dataset.Columns {
			if !isKnownType(defaultType(column.Type)) {
				return fmt.Errorf("unsupported column type %q", column.Type)
			}
		}
		for _, column := range dataset.ConstColumns {
			if !isKnownType(defaultType(column.Type)) {
				return fmt.Errorf("unsupported constant column type %q", column.Type)
			}
		}
	}
	return nil
}

func applySaveTypes(value protocol.Value) protocol.Value {
	needsFiltering := value.SaveType != 0
	if !needsFiltering {
		for _, dataset := range value.Datasets {
			if dataset.SaveType != 0 {
				needsFiltering = true
				break
			}
		}
	}
	if !needsFiltering {
		return value
	}

	datasets := make([]protocol.Dataset, len(value.Datasets))
	copy(datasets, value.Datasets)
	value.Datasets = datasets
	for i := range value.Datasets {
		saveType := value.Datasets[i].SaveType
		if saveType == 0 {
			saveType = value.SaveType
		}
		if saveType == 0 || saveType == 1 {
			continue
		}
		rows := make([]protocol.Row, 0, len(value.Datasets[i].Rows))
		for _, row := range value.Datasets[i].Rows {
			rowType := strings.ToUpper(row.Type)
			include := saveType == 2 && rowType != "D" ||
				saveType == 3 && (rowType == "I" || rowType == "U") ||
				saveType == 4 && rowType == "D" ||
				saveType == 5 && rowType != "N" && rowType != ""
			if !include {
				continue
			}
			if saveType == 2 {
				row.Type = "N"
				row.OrgRow = nil
			}
			rows = append(rows, row)
		}
		value.Datasets[i].Rows = rows
	}
	return value
}

func Encode(v protocol.Value, profile string) ([]byte, error) {
	if err := validateValueTypes(v); err != nil {
		return nil, err
	}
	if err := validateBlobLexicals(v); err != nil {
		return nil, err
	}
	v = applyScalarCompatibility(v)
	v = applySaveTypes(v)
	switch profile {
	case "nexacro-json-1.0":
		return json.Marshal(toJSON(v))
	case "xplatform-xml-4000", "nexacro-xml-4000":
		return encodeXML(v, profile)
	case nexacroSSVProfile, xplatformSSVProfile:
		return encodeSSV(v, profile)
	case nexacroBinaryProfile, xplatformBinaryProfile:
		return encodeBinary(v)
	default:
		return nil, fmt.Errorf("unsupported profile %q", profile)
	}
}

func wireType(dataType string) string {
	switch strings.ToUpper(dataType) {
	case "BOOLEAN":
		return "int"
	case "LONG", "ULONG", "DECIMAL", "BIGDECIMAL":
		return "bigdecimal"
	case "DOUBLE":
		return "float"
	case "FILE", "BLOB":
		return "blob"
	case "STRING", "CHAR":
		return "string"
	case "SHORT", "USHORT", "INT", "UINT":
		return "int"
	case "FLOAT":
		return "float"
	case "DATE":
		return "date"
	case "TIME":
		return "time"
	case "DATETIME":
		return "datetime"
	case "NULL":
		return "null"
	default:
		return strings.ToLower(dataType)
	}
}

func toJSON(v protocol.Value) map[string]any {
	root := map[string]any{"version": "1.0"}
	if v.Wire != nil {
		if s, ok := v.Wire["version"].(string); ok {
			root["version"] = s
		}
	}
	params := make([]any, 0, len(v.Parameters))
	for _, p := range v.Parameters {
		x := map[string]any{"id": p.ID, "type": wireType(p.Type)}
		if p.State != "missing" {
			if p.State == "null" {
				x["value"] = nil
			} else {
				x["value"] = jsonScalar(p.Type, protocol.Cell{State: p.State, Lexical: p.Lexical})
			}
		}
		params = append(params, x)
	}
	if len(params) > 0 {
		root["Parameters"] = params
	}
	datasets := make([]any, 0, len(v.Datasets))
	for _, d := range v.Datasets {
		cols := make([]any, 0, len(d.Columns))
		for _, c := range d.Columns {
			column := map[string]any{"id": c.ID, "type": wireType(c.Type)}
			if c.Size != "" {
				column["size"] = c.Size
			}
			if c.Prop != "" {
				column["prop"] = c.Prop
			}
			if c.SumText != "" {
				column["sumtext"] = c.SumText
			}
			cols = append(cols, column)
		}
		consts := make([]any, 0, len(d.ConstColumns))
		for _, c := range d.ConstColumns {
			column := map[string]any{"id": c.ID, "type": wireType(c.Type)}
			if c.Size != "" {
				column["size"] = c.Size
			}
			if c.Value.State != "missing" {
				column["value"] = jsonScalar(c.Type, c.Value)
			}
			consts = append(consts, column)
		}
		columnTypes := make(map[string]string, len(d.Columns))
		for _, column := range d.Columns {
			columnTypes[column.ID] = column.Type
		}
		rows := []any{}
		for ri, r := range d.Rows {
			if r.Type == "O" && ri > 0 && d.Rows[ri-1].OrgRow != nil && sameRow(*d.Rows[ri-1].OrgRow, r) {
				continue
			}
			x := map[string]any{}
			if r.Type != "N" {
				x["_RowType_"] = r.Type
			}
			for id, c := range r.Values {
				if c.State != "missing" {
					x[id] = jsonScalar(columnTypes[id], c)
				}
			}
			rows = append(rows, x)
			if r.OrgRow != nil {
				ox := map[string]any{"_RowType_": "O"}
				for id, c := range r.OrgRow.Values {
					if c.State != "missing" {
						ox[id] = jsonScalar(columnTypes[id], c)
					}
				}
				rows = append(rows, ox)
			}
		}
		dataset := map[string]any{"id": d.ID}
		if len(consts) > 0 || len(cols) > 0 {
			dataset["ColumnInfo"] = map[string]any{"ConstColumn": consts, "Column": cols}
		}
		if len(rows) > 0 {
			dataset["Rows"] = rows
		}
		datasets = append(datasets, dataset)
	}
	if len(datasets) > 0 {
		root["Datasets"] = datasets
	}
	return root
}

func encodeXML(value protocol.Value, profile string) ([]byte, error) {
	namespace := xmlNamespace(profile)
	versionName, version := "ver", "4000"
	if root, ok := value.Wire["root"].(map[string]any); ok {
		if configured, ok := root["namespace"].(string); ok && configured != "" {
			namespace = configured
		}
		if configured, ok := root["version"].(string); ok {
			versionName, version = "version", configured
		}
		if configured, ok := root["ver"].(string); ok {
			versionName, version = "ver", configured
		}
	}

	var out strings.Builder
	out.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	out.WriteString(`<Root xmlns="`)
	escaped, err := escapeXMLScalar(namespace)
	if err != nil {
		return nil, err
	}
	out.WriteString(escaped)
	out.WriteString(`" `)
	out.WriteString(versionName)
	out.WriteString(`="`)
	escaped, err = escapeXMLScalar(version)
	if err != nil {
		return nil, err
	}
	out.WriteString(escaped)
	out.WriteString(`">`)

	if len(value.Parameters) > 0 {
		out.WriteString("<Parameters>")
		for _, parameter := range value.Parameters {
			if err := writeXMLParameter(&out, parameter); err != nil {
				return nil, err
			}
		}
		out.WriteString("</Parameters>")
	}
	for _, dataset := range value.Datasets {
		if err := writeXMLDataset(&out, dataset); err != nil {
			return nil, err
		}
	}
	out.WriteString("</Root>")
	return []byte(out.String()), nil
}

func writeXMLParameter(out *strings.Builder, parameter protocol.Parameter) error {
	id, err := escapeXMLScalar(parameter.ID)
	if err != nil {
		return err
	}
	dataType, err := escapeXMLScalar(wireType(parameter.Type))
	if err != nil {
		return err
	}
	out.WriteString(`<Parameter id="`)
	out.WriteString(id)
	out.WriteString(`" type="`)
	out.WriteString(dataType)
	out.WriteByte('"')
	if form, ok := parameter.Wire["valueForm"].(string); ok && form == "attribute" && parameter.State != "missing" && parameter.State != "null" {
		value, err := escapeXMLScalar(parameter.Lexical)
		if err != nil {
			return err
		}
		out.WriteString(` value="`)
		out.WriteString(value)
		out.WriteString(`"/>`)
		return nil
	}
	if parameter.State == "missing" || parameter.State == "null" {
		out.WriteString("/>")
		return nil
	}
	out.WriteByte('>')
	value, err := escapeXMLScalar(parameter.Lexical)
	if err != nil {
		return err
	}
	out.WriteString(value)
	out.WriteString("</Parameter>")
	return nil
}

func writeXMLDataset(out *strings.Builder, dataset protocol.Dataset) error {
	id, err := escapeXMLScalar(dataset.ID)
	if err != nil {
		return err
	}
	out.WriteString(`<Dataset id="`)
	out.WriteString(id)
	out.WriteByte('"')
	if len(dataset.ConstColumns) > 0 || len(dataset.Columns) > 0 {
		out.WriteString("><ColumnInfo>")
		for _, column := range dataset.ConstColumns {
			if err := writeXMLConstColumn(out, column); err != nil {
				return err
			}
		}
		for _, column := range dataset.Columns {
			if err := writeXMLColumn(out, column); err != nil {
				return err
			}
		}
		out.WriteString("</ColumnInfo>")
	} else {
		out.WriteByte('>')
	}
	if len(dataset.Rows) > 0 {
		out.WriteString("<Rows>")
		for rowIndex, row := range dataset.Rows {
			if row.Type == "O" && rowIndex > 0 && dataset.Rows[rowIndex-1].OrgRow != nil && sameRow(*dataset.Rows[rowIndex-1].OrgRow, row) {
				continue
			}
			if row.Type == "O" {
				continue
			}
			if err := writeXMLRow(out, row, dataset.Columns); err != nil {
				return err
			}
		}
		out.WriteString("</Rows>")
	}
	out.WriteString("</Dataset>")
	return nil
}

func writeXMLConstColumn(out *strings.Builder, column protocol.ConstColumn) error {
	id, err := escapeXMLScalar(column.ID)
	if err != nil {
		return err
	}
	dataType, err := escapeXMLScalar(wireType(column.Type))
	if err != nil {
		return err
	}
	out.WriteString(`<ConstColumn id="`)
	out.WriteString(id)
	out.WriteString(`" type="`)
	out.WriteString(dataType)
	out.WriteByte('"')
	if column.Size != "" {
		size, err := escapeXMLScalar(column.Size)
		if err != nil {
			return err
		}
		out.WriteString(` size="`)
		out.WriteString(size)
		out.WriteByte('"')
	}
	encoding := column.Encoding
	if strings.EqualFold(dataType, "BLOB") && encoding == "" {
		encoding = "base64"
	}
	if strings.EqualFold(dataType, "BLOB") && !strings.EqualFold(encoding, "base64") {
		return fmt.Errorf("BLOB ConstColumn requires base64 encoding")
	}
	if encoding != "" {
		escapedEncoding, err := escapeXMLScalar(strings.ToLower(encoding))
		if err != nil {
			return err
		}
		out.WriteString(` enc="`)
		out.WriteString(escapedEncoding)
		out.WriteByte('"')
	}
	if column.Value.State != "missing" && column.Value.State != "null" {
		value, err := escapeXMLScalar(column.Value.Lexical)
		if err != nil {
			return err
		}
		out.WriteString(` value="`)
		out.WriteString(value)
		out.WriteByte('"')
	}
	out.WriteString("/>")
	return nil
}

func writeXMLColumn(out *strings.Builder, column protocol.Column) error {
	id, err := escapeXMLScalar(column.ID)
	if err != nil {
		return err
	}
	dataType, err := escapeXMLScalar(wireType(column.Type))
	if err != nil {
		return err
	}
	out.WriteString(`<Column id="`)
	out.WriteString(id)
	out.WriteString(`" type="`)
	out.WriteString(dataType)
	out.WriteByte('"')
	encoding := column.Encoding
	if strings.EqualFold(dataType, "BLOB") && encoding == "" {
		encoding = "base64"
	}
	if strings.EqualFold(dataType, "BLOB") && !strings.EqualFold(encoding, "base64") {
		return fmt.Errorf("BLOB Column requires base64 encoding")
	}
	attributes := [][2]string{{"size", column.Size}, {"enc", strings.ToLower(encoding)}, {"prop", column.Prop}, {"sumtext", column.SumText}}
	for _, attribute := range attributes {
		if attribute[1] == "" {
			continue
		}
		value, err := escapeXMLScalar(attribute[1])
		if err != nil {
			return err
		}
		out.WriteByte(' ')
		out.WriteString(attribute[0])
		out.WriteString(`="`)
		out.WriteString(value)
		out.WriteByte('"')
	}
	out.WriteString("/>")
	return nil
}

func writeXMLRow(out *strings.Builder, row protocol.Row, columns []protocol.Column) error {
	rowType := map[string]string{"N": "", "I": "insert", "U": "update", "D": "delete"}[row.Type]
	if rowType == "" && row.Type != "N" {
		return fmt.Errorf("invalid XML row type %q", row.Type)
	}
	out.WriteString("<Row")
	if rowType != "" {
		out.WriteString(` type="`)
		out.WriteString(rowType)
		out.WriteByte('"')
	}
	out.WriteByte('>')
	if err := writeXMLCells(out, row.Values, columns); err != nil {
		return err
	}
	if row.OrgRow != nil {
		out.WriteString("<OrgRow>")
		if err := writeXMLCells(out, row.OrgRow.Values, columns); err != nil {
			return err
		}
		out.WriteString("</OrgRow>")
	}
	out.WriteString("</Row>")
	return nil
}

func writeXMLCells(out *strings.Builder, cells map[string]protocol.Cell, columns []protocol.Column) error {
	for _, column := range columns {
		cell, ok := cells[column.ID]
		if !ok || cell.State == "missing" || cell.State == "null" {
			continue
		}
		id, err := escapeXMLScalar(column.ID)
		if err != nil {
			return err
		}
		value, err := escapeXMLScalar(cell.Lexical)
		if err != nil {
			return err
		}
		out.WriteString(`<Col id="`)
		out.WriteString(id)
		out.WriteString(`">`)
		out.WriteString(value)
		out.WriteString("</Col>")
	}
	return nil
}

func escapeXMLScalar(value string) (string, error) {
	var out strings.Builder
	for _, r := range value {
		switch r {
		case '&':
			out.WriteString("&amp;")
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		case '"':
			out.WriteString("&quot;")
		case '\'':
			out.WriteString("&apos;")
		case '\t':
			out.WriteString("&#9;")
		case '\n':
			out.WriteString("&#10;")
		case '\r':
			out.WriteString("&#13;")
		default:
			if r < 0x20 {
				return "", fmt.Errorf("character U+%04X is not valid XML 1.0", r)
			}
			out.WriteRune(r)
		}
	}
	return out.String(), nil
}

func sameRow(a, b protocol.Row) bool {
	if a.Type != b.Type || len(a.Values) != len(b.Values) {
		return false
	}
	for id, x := range a.Values {
		y, ok := b.Values[id]
		if !ok || x.State != y.State || x.Lexical != y.Lexical {
			return false
		}
	}
	return true
}
