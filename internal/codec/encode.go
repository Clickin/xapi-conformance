package codec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
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
		encoded, err := json.Marshal(toJSON(v))
		if err != nil {
			return nil, err
		}
		return bytes.ReplaceAll(encoded, []byte("/"), []byte(`\/`)), nil
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

type jsonField struct {
	key   string
	value any
}

type jsonObject []jsonField

func orderedJSON(fields ...jsonField) jsonObject {
	capacity := 16
	for len(fields) > capacity*3/4 {
		capacity *= 2
	}
	sort.SliceStable(fields, func(i, j int) bool {
		return javaJSONBucket(fields[i].key, capacity) < javaJSONBucket(fields[j].key, capacity)
	})
	return fields
}

func javaJSONBucket(key string, capacity int) uint32 {
	var hash uint32
	for _, r := range key {
		if r <= 0xffff {
			hash = 31*hash + uint32(r)
			continue
		}
		r -= 0x10000
		hash = 31*hash + uint32(0xd800+(r>>10))
		hash = 31*hash + uint32(0xdc00+(r&0x3ff))
	}
	hash ^= hash >> 16
	return hash & uint32(capacity-1)
}

func (object jsonObject) MarshalJSON() ([]byte, error) {
	var out strings.Builder
	out.WriteByte('{')
	for i, field := range object {
		if i > 0 {
			out.WriteByte(',')
		}
		key, err := marshalJSONString(field.key)
		if err != nil {
			return nil, err
		}
		value, err := marshalJSONValue(field.value)
		if err != nil {
			return nil, err
		}
		out.Write(key)
		out.WriteByte(':')
		out.Write(value)
	}
	out.WriteByte('}')
	return []byte(out.String()), nil
}

func marshalJSONValue(value any) ([]byte, error) {
	if text, ok := value.(string); ok {
		return marshalJSONString(text)
	}
	return json.Marshal(value)
}

func marshalJSONString(value string) ([]byte, error) {
	return json.Marshal(value)
}

func defaultWireSize(dataType string) string {
	switch strings.ToUpper(defaultType(dataType)) {
	case "STRING", "CHAR":
		return "32"
	case "SHORT", "USHORT", "INT", "UINT":
		return "4"
	case "LONG", "ULONG", "DECIMAL", "BIGDECIMAL":
		return "16"
	case "FLOAT":
		return "4"
	case "DOUBLE":
		return "8"
	case "BOOLEAN":
		return "2"
	case "DATE":
		return "6"
	case "TIME":
		return "9"
	case "DATETIME":
		return "17"
	case "FILE", "BLOB":
		return "256"
	default:
		return "32"
	}
}

func toJSON(v protocol.Value) jsonObject {
	root := []jsonField{{key: "version", value: "1.0"}}
	if v.Wire != nil {
		if version, ok := v.Wire["version"].(string); ok {
			root[0].value = version
		}
	}

	parameters := make([]any, 0, len(v.Parameters))
	for _, parameter := range v.Parameters {
		fields := []jsonField{
			{key: "id", value: parameter.ID},
			{key: "type", value: wireType(parameter.Type)},
		}
		if scalar, ok := jsonScalar(parameter.Type, protocol.Cell{State: parameter.State, Lexical: parameter.Lexical}); ok {
			fields = append(fields, jsonField{key: "value", value: scalar})
		}
		parameters = append(parameters, orderedJSON(fields...))
	}
	if len(parameters) > 0 {
		root = append(root, jsonField{key: "Parameters", value: parameters})
	}

	datasets := make([]any, 0, len(v.Datasets))
	for _, dataset := range v.Datasets {
		columns := make([]any, 0, len(dataset.Columns))
		for _, column := range dataset.Columns {
			columns = append(columns, orderedJSON(
				jsonField{key: "id", value: column.ID},
				jsonField{key: "type", value: wireType(column.Type)},
				jsonField{key: "size", value: defaultWireSize(column.Type)},
			))
		}

		constants := make([]any, 0, len(dataset.ConstColumns))
		for _, column := range dataset.ConstColumns {
			fields := []jsonField{
				{key: "id", value: column.ID},
				{key: "type", value: wireType(column.Type)},
				{key: "size", value: defaultWireSize(column.Type)},
			}
			if column.Value.State == "value" {
				fields = append(fields, jsonField{key: "value", value: column.Value.Lexical})
			} else if strings.EqualFold(defaultType(column.Type), "BOOLEAN") {
				fields = append(fields, jsonField{key: "value", value: "0"})
			}
			constants = append(constants, orderedJSON(fields...))
		}

		rows := make([]any, 0, len(dataset.Rows))
		for _, row := range dataset.Rows {
			if strings.EqualFold(row.Type, "O") {
				continue
			}
			fields := []jsonField{{key: "_RowType_", value: "N"}}
			for _, column := range dataset.Columns {
				cell, ok := row.Values[column.ID]
				if !ok {
					cell.State = "missing"
				}
				if scalar, include := jsonScalar(column.Type, cell); include {
					fields = append(fields, jsonField{key: column.ID, value: scalar})
				}
			}
			rows = append(rows, orderedJSON(fields...))
		}

		fields := []jsonField{{key: "id", value: dataset.ID}}
		if len(constants) > 0 || len(columns) > 0 {
			columnInfo := make([]jsonField, 0, 2)
			if len(constants) > 0 {
				columnInfo = append(columnInfo, jsonField{key: "ConstColumn", value: constants})
			}
			if len(columns) > 0 {
				columnInfo = append(columnInfo, jsonField{key: "Column", value: columns})
			}
			fields = append(fields, jsonField{key: "ColumnInfo", value: orderedJSON(columnInfo...)})
		}
		if len(rows) > 0 {
			fields = append(fields, jsonField{key: "Rows", value: rows})
		}
		datasets = append(datasets, orderedJSON(fields...))
	}
	if len(datasets) > 0 {
		root = append(root, jsonField{key: "Datasets", value: datasets})
	}
	return orderedJSON(root...)
}

func encodeXML(value protocol.Value, profile string) ([]byte, error) {
	var out strings.Builder
	namespace := "http://www.nexacroplatform.com/platform/dataset"
	if profile == "xplatform-xml-4000" {
		namespace = "http://www.tobesoft.com/platform/dataset"
	}
	out.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	out.WriteByte('\n')
	out.WriteString(`<Root xmlns="`)
	out.WriteString(namespace)
	out.WriteByte('"')
	if profile == "xplatform-xml-4000" {
		out.WriteString(` ver="5000"`)
	}
	out.WriteString(">\n")

	if len(value.Parameters) == 0 {
		out.WriteString("\t<Parameters/>\n")
	} else {
		out.WriteString("\t<Parameters>\n")
		for _, parameter := range value.Parameters {
			if err := writeXMLParameter(&out, parameter, profile == "nexacro-xml-4000"); err != nil {
				return nil, err
			}
		}
		out.WriteString("\t</Parameters>\n")
	}
	for _, dataset := range value.Datasets {
		if err := writeXMLDataset(&out, dataset, profile); err != nil {
			return nil, err
		}
	}
	out.WriteString("</Root>\n")
	return []byte(out.String()), nil
}

func writeXMLParameter(out *strings.Builder, parameter protocol.Parameter, rawText bool) error {
	id, err := escapeXMLScalar(parameter.ID)
	if err != nil {
		return err
	}
	dataType, err := escapeXMLScalar(wireType(parameter.Type))
	if err != nil {
		return err
	}
	scalar, hasValue := jsonScalar(parameter.Type, protocol.Cell{State: parameter.State, Lexical: parameter.Lexical})
	out.WriteString(`		<Parameter id="`)
	out.WriteString(id)
	out.WriteString(`" type="`)
	out.WriteString(dataType)
	out.WriteByte('"')
	if strings.EqualFold(defaultType(parameter.Type), "BLOB") && hasValue {
		out.WriteString(` encrypt="base64"`)
	}
	if !hasValue {
		out.WriteString("/>\n")
		return nil
	}
	out.WriteByte('>')
	escaped, err := escapeXMLText(scalar, rawText)
	if err != nil {
		return err
	}
	out.WriteString(escaped)
	out.WriteString("</Parameter>\n")
	return nil
}

func writeXMLDataset(out *strings.Builder, dataset protocol.Dataset, profile string) error {
	id, err := escapeXMLScalar(dataset.ID)
	if err != nil {
		return err
	}
	out.WriteString(`	<Dataset id="`)
	out.WriteString(id)
	out.WriteString(`">` + "\n")
	out.WriteString("\t\t<ColumnInfo>\n")
	if profile == "nexacro-xml-4000" {
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
	} else {
		for _, column := range dataset.Columns {
			if err := writeXMLColumn(out, column); err != nil {
				return err
			}
		}
		for _, column := range dataset.ConstColumns {
			if err := writeXMLConstColumn(out, column); err != nil {
				return err
			}
		}
	}
	out.WriteString("\t\t</ColumnInfo>\n")
	out.WriteString("\t\t<Rows>\n")
	for _, row := range dataset.Rows {
		if strings.EqualFold(row.Type, "O") {
			continue
		}
		if err := writeXMLRow(out, row, dataset.Columns, profile == "nexacro-xml-4000"); err != nil {
			return err
		}
	}
	out.WriteString("\t\t</Rows>\n")
	out.WriteString("\t</Dataset>\n")
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
	size, err := escapeXMLScalar(defaultWireSize(column.Type))
	if err != nil {
		return err
	}
	out.WriteString(`			<ConstColumn id="`)
	out.WriteString(id)
	out.WriteString(`" type="`)
	out.WriteString(dataType)
	out.WriteString(`" size="`)
	out.WriteString(size)
	out.WriteByte('"')
	if strings.EqualFold(defaultType(column.Type), "BLOB") {
		out.WriteString(` encrypt="base64"`)
	}
	if column.Value.State != "value" {
		out.WriteString("/>\n")
		return nil
	}
	value, err := escapeXMLScalar(column.Value.Lexical)
	if err != nil {
		return err
	}
	out.WriteString(` value="`)
	out.WriteString(value)
	out.WriteByte('"')
	if strings.EqualFold(defaultType(column.Type), "BLOB") {
		out.WriteString("></ConstColumn>\n")
	} else {
		out.WriteString("/>\n")
	}
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
	size, err := escapeXMLScalar(defaultWireSize(column.Type))
	if err != nil {
		return err
	}
	out.WriteString(`			<Column id="`)
	out.WriteString(id)
	out.WriteString(`" type="`)
	out.WriteString(dataType)
	out.WriteString(`" size="`)
	out.WriteString(size)
	out.WriteByte('"')
	if strings.EqualFold(defaultType(column.Type), "BLOB") {
		out.WriteString(` encrypt="base64"`)
	}
	out.WriteString("/>\n")
	return nil
}

func writeXMLRow(out *strings.Builder, row protocol.Row, columns []protocol.Column, rawText bool) error {
	rowType := map[string]string{"N": "", "I": "insert", "U": "update", "D": "delete"}[strings.ToUpper(row.Type)]
	if rowType == "" && !strings.EqualFold(row.Type, "N") {
		return fmt.Errorf("invalid XML row type %q", row.Type)
	}
	out.WriteString("\t\t\t<Row")
	if rowType != "" {
		out.WriteString(` type="`)
		out.WriteString(rowType)
		out.WriteByte('"')
	}
	out.WriteString(">\n")
	if err := writeXMLCells(out, row.Values, columns, "\t\t\t\t", rawText); err != nil {
		return err
	}
	if row.OrgRow != nil {
		out.WriteString("\t\t\t\t<OrgRow>\n")
		if err := writeXMLCells(out, row.OrgRow.Values, columns, "\t\t\t\t\t", rawText); err != nil {
			return err
		}
		out.WriteString("\t\t\t\t</OrgRow>\n")
	}
	out.WriteString("\t\t\t</Row>\n")
	return nil
}

func writeXMLCells(out *strings.Builder, cells map[string]protocol.Cell, columns []protocol.Column, indent string, rawText bool) error {
	for _, column := range columns {
		cell, ok := cells[column.ID]
		if !ok || cell.State == "missing" || cell.State == "null" {
			continue
		}
		id, err := escapeXMLScalar(column.ID)
		if err != nil {
			return err
		}
		value, err := escapeXMLText(cell.Lexical, rawText)
		if err != nil {
			return err
		}
		out.WriteString(indent)
		out.WriteString(`<Col id="`)
		out.WriteString(id)
		out.WriteString(`">`)
		out.WriteString(value)
		out.WriteString("</Col>\n")
	}
	return nil
}

func escapeXMLScalar(value string) (string, error) {
	return escapeXML(value, true)
}

func escapeXMLText(value string, rawWhitespace bool) (string, error) {
	return escapeXML(value, !rawWhitespace)
}

func escapeXML(value string, escapeWhitespace bool) (string, error) {
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
			if escapeWhitespace {
				out.WriteString("&#9;")
			} else {
				out.WriteRune(r)
			}
		case '\n':
			if escapeWhitespace {
				out.WriteString("&#10;")
			} else {
				out.WriteRune(r)
			}
		case '\r':
			if escapeWhitespace {
				out.WriteString("&#13;")
			} else {
				out.WriteRune(r)
			}
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
