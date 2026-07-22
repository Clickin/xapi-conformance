package codec

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/Clickin/xapi-conformance/internal/protocol"
)

type DecodeOptions struct {
	Strict             bool
	SSVUnitSeparator   string
	SSVRecordSeparator string
}

func Decode(b []byte) (protocol.Value, error) {
	return DecodeWithStrict(b, true)
}

func DecodeWithStrict(b []byte, strict bool) (protocol.Value, error) {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 {
		return protocol.Value{}, fmt.Errorf("empty document")
	}
	if bytes.HasPrefix(trimmed, []byte("SSV")) {
		return ssvValue(trimmed, nexacroSSVProfile, strict, "", "")
	}
	if trimmed[0] == '<' {
		return xmlValue(b, strict, "")
	}
	return jsonValue(b, strict)
}

func DecodeProfile(b []byte, profile string, options DecodeOptions) (protocol.Value, error) {
	switch profile {
	case "nexacro-json-1.0":
		return jsonValue(b, options.Strict)
	case "xplatform-xml-4000", "nexacro-xml-4000":
		return xmlValue(b, options.Strict, profile)
	case nexacroSSVProfile, xplatformSSVProfile:
		return ssvValue(b, profile, options.Strict, options.SSVUnitSeparator, options.SSVRecordSeparator)
	default:
		return protocol.Value{}, fmt.Errorf("unsupported profile %q", profile)
	}
}

func jsonValue(b []byte, strict bool) (protocol.Value, error) {
	if err := protocol.RejectDuplicateKeys(b); err != nil {
		return protocol.Value{}, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(b, &root); err != nil {
		return protocol.Value{}, err
	}
	out := protocol.Value{Parameters: []protocol.Parameter{}, Datasets: []protocol.Dataset{}}
	var version string
	versionRaw, hasVersion := root["version"]
	if hasVersion {
		if err := json.Unmarshal(versionRaw, &version); err != nil {
			return out, fmt.Errorf("version must be a string")
		}
		out.Wire = map[string]any{"version": version}
	}
	if strict && (!hasVersion || version != "1.0") {
		return out, fmt.Errorf("version must be %q", "1.0")
	}
	if raw, ok := root["Parameters"]; ok {
		if err := parseJSONParameters(raw, &out.Parameters, strict); err != nil {
			return out, err
		}
	}
	var datasets []map[string]json.RawMessage
	if raw, ok := root["Datasets"]; ok {
		if err := json.Unmarshal(raw, &datasets); err != nil {
			return out, fmt.Errorf("Datasets must be an array: %w", err)
		}
	}
	for _, d := range datasets {
		ds := protocol.Dataset{Columns: []protocol.Column{}, ConstColumns: []protocol.ConstColumn{}, Rows: []protocol.Row{}}
		if err := json.Unmarshal(d["id"], &ds.ID); err != nil || strict && ds.ID == "" {
			return out, fmt.Errorf("Dataset.id is required")
		}
		columnInfo, hasColumnInfo := d["ColumnInfo"]
		if strict && !hasColumnInfo {
			return out, fmt.Errorf("Dataset.ColumnInfo is required")
		}
		if hasColumnInfo {
			if err := parseJSONColumns(columnInfo, &ds, strict); err != nil {
				return out, err
			}
		}
		rowsRaw, hasRows := d["Rows"]
		if strict && !hasRows {
			return out, fmt.Errorf("Dataset.Rows is required")
		}
		var rows []map[string]json.RawMessage
		if hasRows {
			if err := json.Unmarshal(rowsRaw, &rows); err != nil {
				return out, fmt.Errorf("Dataset.Rows must be an array: %w", err)
			}
		}
		for _, r := range rows {
			row := protocol.Row{Type: "N", Values: map[string]protocol.Cell{}}
			if raw := r["_RowType_"]; len(raw) > 0 {
				if err := json.Unmarshal(raw, &row.Type); err != nil {
					return out, fmt.Errorf("_RowType_ must be a string")
				}
			}
			if !strings.Contains("NIUDO", row.Type) || len(row.Type) != 1 {
				if strict {
					return out, fmt.Errorf("invalid _RowType_ %q", row.Type)
				}
				continue
			}
			for _, c := range ds.Columns {
				row.Values[c.ID] = rawCell(r[c.ID])
			}
			if row.Type == "O" {
				if len(ds.Rows) == 0 || ds.Rows[len(ds.Rows)-1].Type != "U" {
					continue
				}
				org := row
				ds.Rows[len(ds.Rows)-1].OrgRow = &org
			}
			ds.Rows = append(ds.Rows, row)
		}
		out.Datasets = append(out.Datasets, ds)
	}
	return out, nil
}

func parseJSONParameters(raw json.RawMessage, out *[]protocol.Parameter, strict bool) error {
	var parameters []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &parameters); err != nil {
		return fmt.Errorf("Parameters must be an array: %w", err)
	}
	for i, item := range parameters {
		var parameter protocol.Parameter
		if err := json.Unmarshal(item["id"], &parameter.ID); err != nil || strict && parameter.ID == "" {
			return fmt.Errorf("Parameter.id is required")
		}
		_ = json.Unmarshal(item["type"], &parameter.Type)
		if parameter.Type == "" {
			parameter.Type = inferJSONType(item["value"])
		}
		parameter.Type = strings.ToUpper(parameter.Type)
		if strict && !isKnownType(parameter.Type) {
			return fmt.Errorf("unsupported Parameter.type %q", parameter.Type)
		}
		parameter.Index = i
		if value, ok := item["value"]; ok {
			parameter.State, parameter.Lexical = rawState(value)
		} else {
			parameter.State = "missing"
		}
		*out = append(*out, parameter)
	}
	return nil
}

func parseJSONColumns(raw json.RawMessage, dataset *protocol.Dataset, strict bool) error {
	var info map[string]json.RawMessage
	if err := json.Unmarshal(raw, &info); err != nil {
		return fmt.Errorf("ColumnInfo must be an object: %w", err)
	}
	columnsRaw, hasColumns := info["Column"]
	if strict && !hasColumns {
		return fmt.Errorf("ColumnInfo.Column is required")
	}
	if hasColumns {
		if err := parseJSONColumnArray(columnsRaw, dataset, false, strict); err != nil {
			return err
		}
	}
	if constantsRaw, ok := info["ConstColumn"]; ok {
		if err := parseJSONColumnArray(constantsRaw, dataset, true, strict); err != nil {
			return err
		}
	}
	return nil
}

func parseJSONColumnArray(raw json.RawMessage, dataset *protocol.Dataset, isConst, strict bool) error {
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("column list must be an array: %w", err)
	}
	for _, item := range items {
		var id, dataType, encoding, prop, sumText string
		_ = json.Unmarshal(item["id"], &id)
		if strict && id == "" {
			return fmt.Errorf("column id is required")
		}
		_ = json.Unmarshal(item["type"], &dataType)
		_ = json.Unmarshal(item["enc"], &encoding)
		_ = json.Unmarshal(item["prop"], &prop)
		_ = json.Unmarshal(item["sumtext"], &sumText)
		size := rawScalarString(item["size"])
		if dataType == "" {
			if isConst {
				dataType = inferJSONType(item["value"])
			} else {
				dataType = "STRING"
			}
		}
		dataType = strings.ToUpper(dataType)
		if strict && !isKnownType(dataType) {
			return fmt.Errorf("unsupported column type %q", dataType)
		}
		if isConst {
			dataset.ConstColumns = append(dataset.ConstColumns, protocol.ConstColumn{ID: id, Type: dataType, Size: size, Encoding: encoding, Value: rawCell(item["value"])})
		} else {
			dataset.Columns = append(dataset.Columns, protocol.Column{ID: id, Type: dataType, Size: size, Encoding: encoding, Prop: prop, SumText: sumText, Index: len(dataset.Columns)})
		}
	}
	return nil
}

func inferJSONType(raw json.RawMessage) string {
	if len(raw) == 0 || raw[0] == '"' || string(raw) == "null" {
		return "STRING"
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&number) == nil {
		if strings.ContainsAny(number.String(), ".eE") {
			return "FLOAT"
		}
		return "INT"
	}
	return "STRING"
}

func rawScalarString(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	if len(raw) > 0 && string(raw) != "null" {
		return string(raw)
	}
	return ""
}
func rawCell(raw json.RawMessage) protocol.Cell {
	if len(raw) == 0 {
		return protocol.Cell{State: "missing"}
	}
	s, v := rawState(raw)
	return protocol.Cell{State: s, Lexical: v}
}
func rawState(raw json.RawMessage) (string, string) {
	if string(raw) == "null" {
		return "null", ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if s == "" {
			return "empty", ""
		}
		return "value", s
	}
	return "value", string(raw)
}

type xroot struct {
	XMLName    xml.Name
	Attr       []xml.Attr `xml:",any,attr"`
	Parameters struct {
		Items []xparam `xml:"Parameter"`
	} `xml:"Parameters"`
	Datasets struct {
		Items []xdataset `xml:"Dataset"`
	} `xml:"Datasets"`
	DirectDatasets []xdataset `xml:"Dataset"`
}
type xparam struct {
	Attr []xml.Attr `xml:",any,attr"`
	Text string     `xml:",chardata"`
}
type xdataset struct {
	ID      string `xml:"id,attr"`
	Columns struct {
		Items  []xcol `xml:"Column"`
		Consts []xcol `xml:"ConstColumn"`
	} `xml:"ColumnInfo"`
	Rows struct {
		Items []xrow `xml:"Row"`
	} `xml:"Rows"`
}
type xcol struct {
	ID       string  `xml:"id,attr"`
	Type     string  `xml:"type,attr"`
	Size     string  `xml:"size,attr"`
	Encoding string  `xml:"enc,attr"`
	Value    *string `xml:"value,attr"`
	Encrypt  string  `xml:"encrypt,attr"`
	Prop     string  `xml:"prop,attr"`
	SumText  string  `xml:"sumtext,attr"`
}
type xrow struct {
	Type string      `xml:"type,attr"`
	Cols []xcolvalue `xml:"Col"`
	Org  *xorgrow    `xml:"OrgRow"`
}
type xorgrow struct {
	Cols []xcolvalue `xml:"Col"`
}
type xcolvalue struct {
	ID   string `xml:"id,attr"`
	Text string `xml:",chardata"`
}

type xmlSyntax struct {
	ParameterSelfClosing []bool
	HasDatasetsWrapper   bool
}

func xmlValue(b []byte, strict bool, profile string) (protocol.Value, error) {
	if bytes.Contains(bytes.ToUpper(b), []byte("<!DOCTYPE")) {
		return protocol.Value{}, fmt.Errorf("DTD and external entities are forbidden")
	}
	if strict && profile != "" && !bytes.HasPrefix(b, []byte("<?xml ")) {
		return protocol.Value{}, fmt.Errorf("XML declaration must be the first bytes")
	}
	if err := rejectDuplicateXMLAttrs(b); err != nil {
		return protocol.Value{}, err
	}
	syntax, err := validateXMLStructure(b, strict, profile)
	if err != nil {
		return protocol.Value{}, err
	}
	var root xroot
	decoder := xml.NewDecoder(bytes.NewReader(b))
	decoder.Strict = true
	if err := decoder.Decode(&root); err != nil {
		return protocol.Value{}, err
	}

	out := protocol.Value{Parameters: []protocol.Parameter{}, Datasets: []protocol.Dataset{}}
	rootWire := map[string]any{}
	if root.XMLName.Space != "" {
		rootWire["namespace"] = root.XMLName.Space
	}
	for _, attribute := range root.Attr {
		if attribute.Name.Local == "version" || attribute.Name.Local == "ver" {
			rootWire[attribute.Name.Local] = attribute.Value
		}
	}
	if len(rootWire) > 0 {
		out.Wire = map[string]any{"root": rootWire}
	}

	for i, item := range root.Parameters.Items {
		id, dataType, attributeValue := "", "", ""
		hasAttributeValue := false
		for _, attribute := range item.Attr {
			switch attribute.Name.Local {
			case "id":
				id = attribute.Value
			case "type":
				dataType = attribute.Value
			case "value":
				attributeValue = attribute.Value
				hasAttributeValue = true
			}
		}
		if strict && id == "" {
			return protocol.Value{}, fmt.Errorf("Parameter.id is required")
		}
		lexical, form := item.Text, "text"
		if hasAttributeValue {
			lexical, form = attributeValue, "attribute"
		}
		if dataType == "" {
			dataType = "STRING"
		}
		state := "value"
		if lexical == "" {
			state = "empty"
		}
		if !hasAttributeValue && i < len(syntax.ParameterSelfClosing) && syntax.ParameterSelfClosing[i] {
			state = "null"
		}
		out.Parameters = append(out.Parameters, protocol.Parameter{
			ID: id, Type: strings.ToUpper(dataType), State: state, Lexical: lexical, Index: i,
			Wire: map[string]any{"valueForm": form},
		})
	}

	datasets := append(append([]xdataset{}, root.Datasets.Items...), root.DirectDatasets...)
	for _, item := range datasets {
		if strict && item.ID == "" {
			return protocol.Value{}, fmt.Errorf("Dataset.id is required")
		}
		dataset := protocol.Dataset{ID: item.ID, Columns: []protocol.Column{}, ConstColumns: []protocol.ConstColumn{}, Rows: []protocol.Row{}}
		for i, column := range item.Columns.Items {
			if strict && column.ID == "" {
				return protocol.Value{}, fmt.Errorf("Column.id is required")
			}
			dataType := defaultType(column.Type)
			if strict && !isKnownType(dataType) {
				return protocol.Value{}, fmt.Errorf("unsupported Column.type %q", column.Type)
			}
			encoding := column.Encoding
			if encoding == "" {
				encoding = column.Encrypt
			}
			if strict && strings.EqualFold(dataType, "BLOB") && !strings.EqualFold(encoding, "base64") {
				return protocol.Value{}, fmt.Errorf("BLOB Column requires enc=\"base64\"")
			}
			dataset.Columns = append(dataset.Columns, protocol.Column{
				ID: column.ID, Type: dataType, Size: column.Size, Encoding: strings.ToLower(encoding),
				Prop: column.Prop, SumText: column.SumText, Index: i,
			})
		}
		for _, column := range item.Columns.Consts {
			dataType := defaultType(column.Type)
			if strict && !isKnownType(dataType) {
				return protocol.Value{}, fmt.Errorf("unsupported ConstColumn.type %q", column.Type)
			}
			if strict && column.ID == "" {
				return protocol.Value{}, fmt.Errorf("ConstColumn.id is required")
			}
			encoding := column.Encoding
			if encoding == "" {
				encoding = column.Encrypt
			}
			if strict && strings.EqualFold(dataType, "BLOB") && !strings.EqualFold(encoding, "base64") {
				return protocol.Value{}, fmt.Errorf("BLOB ConstColumn requires enc=\"base64\"")
			}
			value := protocol.Cell{State: "missing"}
			if column.Value != nil {
				value.State, value.Lexical = "value", *column.Value
				if *column.Value == "" {
					value.State = "empty"
				}
			}
			dataset.ConstColumns = append(dataset.ConstColumns, protocol.ConstColumn{
				ID: column.ID, Type: dataType, Size: column.Size, Encoding: strings.ToLower(encoding), Value: value,
			})
		}
		known := map[string]bool{}
		for _, column := range dataset.Columns {
			known[column.ID] = true
		}
		for _, itemRow := range item.Rows.Items {
			for _, cell := range itemRow.Cols {
				if !known[cell.ID] {
					return protocol.Value{}, fmt.Errorf("column %q is not declared", cell.ID)
				}
			}
			if itemRow.Org != nil {
				for _, cell := range itemRow.Org.Cols {
					if !known[cell.ID] {
						return protocol.Value{}, fmt.Errorf("org column %q is not declared", cell.ID)
					}
				}
			}
			if strict {
				isUpdate := strings.EqualFold(itemRow.Type, "update")
				if isUpdate != (itemRow.Org != nil) {
					return protocol.Value{}, fmt.Errorf("OrgRow is required only for update rows")
				}
			}
			dataset.Rows = append(dataset.Rows, xmlRow(itemRow, dataset.Columns))
		}
		out.Datasets = append(out.Datasets, dataset)
	}
	return out, nil
}

func validateXMLStructure(b []byte, strict bool, profile string) (xmlSyntax, error) {
	allowed := map[string]map[string]bool{
		"Root":       {"Parameters": true, "Datasets": true, "Dataset": true},
		"Parameters": {"Parameter": true},
		"Datasets":   {"Dataset": true},
		"Dataset":    {"ColumnInfo": true, "Rows": true},
		"ColumnInfo": {"Column": true, "ConstColumn": true},
		"Rows":       {"Row": true},
		"Row":        {"Col": true, "OrgRow": true},
		"OrgRow":     {"Col": true},
	}
	allowedAttributes := map[string]map[string]bool{
		"Root":        {"xmlns": true, "ver": true, "version": true},
		"Parameter":   {"id": true, "type": true, "value": true, "enc": true, "encrypt": true},
		"Dataset":     {"id": true},
		"Column":      {"id": true, "type": true, "size": true, "enc": true, "encrypt": true, "prop": true, "sumtext": true},
		"ConstColumn": {"id": true, "type": true, "size": true, "enc": true, "encrypt": true, "value": true},
		"Row":         {"type": true},
		"Col":         {"id": true},
	}
	valid := map[string]bool{
		"Root": true, "Parameters": true, "Parameter": true, "Datasets": true,
		"Dataset": true, "ColumnInfo": true, "Column": true, "ConstColumn": true,
		"Rows": true, "Row": true, "OrgRow": true, "Col": true,
	}
	decoder := xml.NewDecoder(bytes.NewReader(b))
	stack := []string{}
	skipDepth := 0
	rootSeen := false
	columnInfoSawColumn := false
	syntax := xmlSyntax{}
	for {
		before := decoder.InputOffset()
		token, err := decoder.Token()
		if err == io.EOF {
			if !rootSeen || len(stack) != 0 {
				return syntax, fmt.Errorf("XML root is incomplete")
			}
			return syntax, nil
		}
		if err != nil {
			return syntax, err
		}
		switch element := token.(type) {
		case xml.StartElement:
			if skipDepth > 0 {
				skipDepth++
				continue
			}
			if !rootSeen {
				if element.Name.Local != "Root" {
					return syntax, fmt.Errorf("root element must be Root")
				}
				rootSeen = true
				if strict {
					expected := xmlNamespace(profile)
					if expected != "" && element.Name.Space != expected {
						return syntax, fmt.Errorf("unexpected XML namespace %q", element.Name.Space)
					}
				}
			}
			if strict {
				attributes := allowedAttributes[element.Name.Local]
				for _, attribute := range element.Attr {
					if attribute.Name.Space == "xmlns" {
						continue
					}
					if !attributes[attribute.Name.Local] {
						return syntax, fmt.Errorf("unexpected attribute %s on %s", attribute.Name.Local, element.Name.Local)
					}
				}
			}
			if !valid[element.Name.Local] {
				if !strict {
					skipDepth = 1
					continue
				}
				return syntax, fmt.Errorf("unexpected element %s", element.Name.Local)
			}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				if children, ok := allowed[parent]; ok && !children[element.Name.Local] {
					if !strict && !(parent == "Rows" && (element.Name.Local == "Col" || element.Name.Local == "OrgRow")) {
						skipDepth = 1
						continue
					}
					return syntax, fmt.Errorf("unexpected element %s under %s", element.Name.Local, parent)
				}
			}
			if element.Name.Local == "Datasets" {
				syntax.HasDatasetsWrapper = true
				if strict {
					return syntax, fmt.Errorf("Datasets wrapper is not part of the XML format")
				}
			}
			if element.Name.Local == "ColumnInfo" {
				columnInfoSawColumn = false
			}
			if element.Name.Local == "Column" {
				columnInfoSawColumn = true
			}
			if strict && element.Name.Local == "ConstColumn" && columnInfoSawColumn {
				return syntax, fmt.Errorf("ConstColumn must precede Column")
			}
			if element.Name.Local == "Parameter" {
				segment := bytes.TrimSpace(b[before:decoder.InputOffset()])
				syntax.ParameterSelfClosing = append(syntax.ParameterSelfClosing, bytes.HasSuffix(segment, []byte("/>")))
			}
			stack = append(stack, element.Name.Local)
		case xml.EndElement:
			if skipDepth > 0 {
				skipDepth--
				continue
			}
			if len(stack) == 0 || stack[len(stack)-1] != element.Name.Local {
				return syntax, fmt.Errorf("unexpected closing element %s", element.Name.Local)
			}
			stack = stack[:len(stack)-1]
		}
	}
}

func xmlNamespace(profile string) string {
	switch profile {
	case "xplatform-xml-4000":
		return "http://www.tobesoft.com/platform/Dataset"
	case "nexacro-xml-4000":
		return "http://www.nexacroplatform.com/platform/dataset"
	default:
		return ""
	}
}

func rejectDuplicateXMLAttrs(b []byte) error {
	d := xml.NewDecoder(bytes.NewReader(b))
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if start, ok := tok.(xml.StartElement); ok {
			seen := map[string]bool{}
			for _, a := range start.Attr {
				key := a.Name.Space + ":" + a.Name.Local
				if seen[key] {
					return fmt.Errorf("duplicate XML attribute %q", a.Name.Local)
				}
				seen[key] = true
			}
		}
	}
}

func xmlRow(r xrow, columns []protocol.Column) protocol.Row {
	typ := map[string]string{"insert": "I", "update": "U", "delete": "D", "normal": "N"}[strings.ToLower(r.Type)]
	if typ == "" {
		typ = "N"
	}
	row := protocol.Row{Type: typ, Values: map[string]protocol.Cell{}}
	for _, c := range columns {
		row.Values[c.ID] = protocol.Cell{State: "missing"}
	}
	for _, c := range r.Cols {
		row.Values[c.ID] = xmlCell(c.Text)
	}
	if r.Org != nil {
		org := xmlRow(xrow{Cols: r.Org.Cols}, columns)
		org.Type = "O"
		row.OrgRow = &org
	}
	return row
}
func xmlCell(s string) protocol.Cell {
	if s == "" {
		return protocol.Cell{State: "empty"}
	}
	return protocol.Cell{State: "value", Lexical: s}
}
