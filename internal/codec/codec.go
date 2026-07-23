package codec

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

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
	var value protocol.Value
	var err error
	switch {
	case len(trimmed) >= 2 && ((trimmed[0] == 0xfe && trimmed[1] == 0x10) || (trimmed[0] == 0xfe && trimmed[1] == 0x01)):
		value, err = binaryValue(trimmed, strict)
	case bytes.HasPrefix(trimmed, []byte("SSV")):
		value, err = ssvValue(trimmed, nexacroSSVProfile, strict, "", "")
	case trimmed[0] == '<':
		value, err = xmlValue(b, strict, "")
	default:
		value, err = jsonValue(b, strict)
	}
	return validateDecodedValue(value, err, strict)
}

func DecodeProfile(b []byte, profile string, options DecodeOptions) (protocol.Value, error) {
	var value protocol.Value
	var err error
	switch profile {
	case "nexacro-json-1.0":
		value, err = jsonValue(b, options.Strict)
	case "xplatform-xml-4000", "nexacro-xml-4000":
		value, err = xmlValue(b, options.Strict, profile)
	case nexacroSSVProfile, xplatformSSVProfile:
		value, err = ssvValue(b, profile, options.Strict, options.SSVUnitSeparator, options.SSVRecordSeparator)
	case nexacroBinaryProfile, xplatformBinaryProfile:
		value, err = binaryValue(b, options.Strict)
	default:
		return protocol.Value{}, fmt.Errorf("unsupported profile %q", profile)
	}
	return validateDecodedValue(value, err, options.Strict)
}

func validateDecodedValue(value protocol.Value, decodeErr error, strict bool) (protocol.Value, error) {
	return value, decodeErr
}

func validateBlobLexicals(value protocol.Value) error {
	for i, parameter := range value.Parameters {
		if isBlobType(parameter.Type) {
			if err := validateBlobCell(protocol.Cell{State: parameter.State, Lexical: parameter.Lexical}); err != nil {
				return fmt.Errorf("Parameters[%d].value: %w", i, err)
			}
		}
	}
	for di, dataset := range value.Datasets {
		for ci, column := range dataset.ConstColumns {
			if isBlobType(column.Type) {
				if err := validateBlobCell(column.Value); err != nil {
					return fmt.Errorf("Datasets[%d].ConstColumns[%d].value: %w", di, ci, err)
				}
			}
		}
		columnTypes := make(map[string]string, len(dataset.Columns))
		for _, column := range dataset.Columns {
			columnTypes[column.ID] = column.Type
		}
		for ri, row := range dataset.Rows {
			for id, cell := range row.Values {
				if isBlobType(columnTypes[id]) {
					if err := validateBlobCell(cell); err != nil {
						return fmt.Errorf("Datasets[%d].Rows[%d].%s: %w", di, ri, id, err)
					}
				}
			}
			if row.OrgRow != nil {
				for id, cell := range row.OrgRow.Values {
					if isBlobType(columnTypes[id]) {
						if err := validateBlobCell(cell); err != nil {
							return fmt.Errorf("Datasets[%d].Rows[%d].OrgRow.%s: %w", di, ri, id, err)
						}
					}
				}
			}
		}
	}
	return nil
}

func isBlobType(dataType string) bool {
	return strings.EqualFold(dataType, "BLOB") || strings.EqualFold(dataType, "FILE")
}

func validateBlobCell(cell protocol.Cell) error {
	if cell.State != "value" && cell.State != "empty" {
		return nil
	}
	if _, err := base64.StdEncoding.DecodeString(cell.Lexical); err != nil {
		return fmt.Errorf("invalid base64 BLOB value")
	}
	return nil
}

func jsonValue(b []byte, strict bool) (protocol.Value, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(b, &root); err != nil {
		return protocol.Value{}, err
	}
	out := protocol.Value{
		Parameters: []protocol.Parameter{},
		Datasets:   []protocol.Dataset{},
		Wire:       map[string]any{"version": "1.0"},
	}
	if versionRaw, ok := root["version"]; ok {
		var version string
		if err := json.Unmarshal(versionRaw, &version); err != nil {
			return out, fmt.Errorf("version must be a string")
		}
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
		if err := json.Unmarshal(d["id"], &ds.ID); err != nil || ds.ID == "" {
			return out, fmt.Errorf("Dataset.id is required")
		}
		constValues := map[string]protocol.Cell{}
		if columnInfo, ok := d["ColumnInfo"]; ok {
			var err error
			constValues, err = parseJSONColumns(columnInfo, &ds, strict)
			if err != nil {
				return out, err
			}
		}
		var rows []map[string]json.RawMessage
		if rowsRaw, ok := d["Rows"]; ok {
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
			if row.Type == "O" {
				if len(ds.Rows) == 0 {
					return out, fmt.Errorf("orphan _RowType_ O")
				}
				continue
			}
			if row.Type == "D" {
				continue
			}
			for _, column := range ds.Columns {
				row.Values[column.ID] = jsonCell(r[column.ID], column.Type)
			}
			for _, column := range ds.ConstColumns {
				row.Values[column.ID] = constValues[column.ID]
			}
			row.MarkOrgRowPresent()
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
	for _, item := range parameters {
		var parameter protocol.Parameter
		if err := json.Unmarshal(item["id"], &parameter.ID); err != nil || parameter.ID == "" {
			return fmt.Errorf("Parameter.id is required")
		}
		_ = json.Unmarshal(item["type"], &parameter.Type)
		if parameter.Type == "" {
			parameter.Type = inferJSONType(item["value"])
		}
		parameter.Type = normalizeDecodedType(parameter.Type, "STRING")
		if value, ok := item["value"]; ok {
			parameter.State, parameter.Lexical = rawState(value)
		} else {
			parameter.State = "null"
		}
		if isBlobType(parameter.Type) && (parameter.State == "value" || parameter.State == "empty") {
			parameter.Lexical = base64.StdEncoding.EncodeToString([]byte(parameter.Lexical))
		}
		if parameter.State == "empty" {
			parameter.MarkLexicalPresent()
		}
		*out = append(*out, parameter)
	}
	return nil
}

func parseJSONColumns(raw json.RawMessage, dataset *protocol.Dataset, strict bool) (map[string]protocol.Cell, error) {
	var info map[string]json.RawMessage
	if err := json.Unmarshal(raw, &info); err != nil {
		return nil, fmt.Errorf("ColumnInfo must be an object: %w", err)
	}
	constValues := map[string]protocol.Cell{}
	if columnsRaw, ok := info["Column"]; ok {
		if err := parseJSONColumnArray(columnsRaw, dataset, false, nil); err != nil {
			return nil, err
		}
	}
	if constantsRaw, ok := info["ConstColumn"]; ok {
		if err := parseJSONColumnArray(constantsRaw, dataset, true, constValues); err != nil {
			return nil, err
		}
	}
	return constValues, nil
}

func parseJSONColumnArray(raw json.RawMessage, dataset *protocol.Dataset, isConst bool, constValues map[string]protocol.Cell) error {
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("column list must be an array: %w", err)
	}
	for _, item := range items {
		var id, dataType string
		_ = json.Unmarshal(item["id"], &id)
		if id == "" {
			return fmt.Errorf("column id is required")
		}
		_ = json.Unmarshal(item["type"], &dataType)
		if dataType == "" {
			if isConst {
				dataType = inferJSONType(item["value"])
			} else {
				dataType = "STRING"
			}
		}
		dataType = normalizeDecodedType(dataType, "STRING")
		if isConst {
			index := len(dataset.Columns) + len(dataset.ConstColumns)
			dataset.ConstColumns = append(dataset.ConstColumns, protocol.ConstColumn{ID: id, Type: dataType, Index: index})
			constValues[id] = jsonCell(item["value"], dataType)
		} else {
			dataset.Columns = append(dataset.Columns, protocol.Column{ID: id, Type: dataType, Index: len(dataset.Columns)})
		}
	}
	return nil
}

func inferJSONType(raw json.RawMessage) string {
	return "STRING"
}
func normalizeDecodedType(dataType, missingDefault string) string {
	dataType = strings.ToUpper(dataType)
	if dataType == "" {
		dataType = missingDefault
	}
	if dataType == "FLOAT" {
		return "DOUBLE"
	}
	if dataType == "DECIMAL" || !isKnownType(dataType) {
		return "UNDEFINED"
	}
	return dataType
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
		return protocol.Cell{State: "null"}
	}
	state, lexical := rawState(raw)
	cell := protocol.Cell{State: state, Lexical: lexical}
	if state == "empty" {
		cell.MarkLexicalPresent()
	}
	return cell
}
func jsonCell(raw json.RawMessage, dataType string) protocol.Cell {
	cell := rawCell(raw)
	if isBlobType(dataType) && (cell.State == "value" || cell.State == "empty") {
		cell.Lexical = base64.StdEncoding.EncodeToString([]byte(cell.Lexical))
	}
	return cell
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
	Type          string      `xml:"type,attr"`
	Cols          []xcolvalue `xml:"Col"`
	Org           *xorgrow    `xml:"OrgRow"`
	OrgUnderscore *xorgrow    `xml:"Org_Row"`
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
	DatasetColumnIndexes []map[string]int
	DatasetIDs            []string
}

func xmlValue(b []byte, strict bool, profile string) (protocol.Value, error) {
	if bytes.Contains(bytes.ToUpper(b), []byte("<!DOCTYPE")) {
		return protocol.Value{}, fmt.Errorf("DTD and external entities are forbidden")
	}
	if declaration := bytes.Index(b, []byte("<?xml")); declaration > 0 {
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
	if namespace := xmlNamespace(profile); namespace != "" {
		rootWire["namespace"] = namespace
		switch profile {
		case "nexacro-xml-4000":
			rootWire["version"] = "4000"
		case "xplatform-xml-4000":
			rootWire["ver"] = "4000"
		}
	} else {
		if root.XMLName.Space != "" {
			rootWire["namespace"] = root.XMLName.Space
		}
		for _, attribute := range root.Attr {
			if attribute.Name.Local == "version" || attribute.Name.Local == "ver" {
				rootWire[attribute.Name.Local] = attribute.Value
			}
		}
	}
	if len(rootWire) > 0 {
		out.Wire = map[string]any{"root": rootWire}
	}

	for _, item := range root.Parameters.Items {
		id, dataType := "", ""
		for _, attribute := range item.Attr {
			switch attribute.Name.Local {
			case "id":
				id = attribute.Value
			case "type":
				dataType = attribute.Value
			}
		}
		if id == "" {
			return protocol.Value{}, fmt.Errorf("Parameter.id is required")
		}
		dataType = normalizeDecodedParameterType(dataType)
		cell := normalizeXMLCell(xmlCell(item.Text), dataType)
		parameter := protocol.Parameter{
			ID: id, Type: dataType, State: cell.State, Lexical: cell.Lexical,
		}
		if cell.State == "empty" {
			parameter.MarkLexicalPresent()
		}
		out.Parameters = append(out.Parameters, parameter)
	}

	datasets := append(append([]xdataset{}, root.DirectDatasets...), root.Datasets.Items...)
	datasets = orderXMLDatasets(datasets, syntax.DatasetIDs)
	for datasetIndex, item := range datasets {
		if item.ID == "" {
			return protocol.Value{}, fmt.Errorf("Dataset.id is required")
		}
		dataset := protocol.Dataset{ID: item.ID, Columns: []protocol.Column{}, ConstColumns: []protocol.ConstColumn{}, Rows: []protocol.Row{}}
		constValues := map[string]protocol.Cell{}
		wireTypes := map[string]string{}
		for i, column := range item.Columns.Items {
			if column.ID == "" {
				return protocol.Value{}, fmt.Errorf("Column.id is required")
			}
			wireTypes[column.ID] = strings.ToUpper(column.Type)
			dataType := normalizeDecodedType(column.Type, "UNDEFINED")
			index := xmlColumnIndex(syntax, datasetIndex, "Column", column.ID, len(item.Columns.Consts)+i)
			dataset.Columns = append(dataset.Columns, protocol.Column{ID: column.ID, Type: dataType, Index: index})
		}
		for i, column := range item.Columns.Consts {
			if column.ID == "" {
				return protocol.Value{}, fmt.Errorf("ConstColumn.id is required")
			}
			dataType := normalizeDecodedType(column.Type, "UNDEFINED")
			index := xmlColumnIndex(syntax, datasetIndex, "ConstColumn", column.ID, i)
			dataset.ConstColumns = append(dataset.ConstColumns, protocol.ConstColumn{ID: column.ID, Type: dataType, Index: index})
			value := protocol.Cell{State: "null"}
			if column.Value != nil {
				value = normalizeXMLCell(xmlCell(*column.Value), strings.ToUpper(column.Type))
			}
			constValues[column.ID] = value
			wireTypes[column.ID] = strings.ToUpper(column.Type)
		}
		known := map[string]bool{}
		for _, column := range dataset.Columns {
			known[column.ID] = true
		}
		for _, column := range dataset.ConstColumns {
			known[column.ID] = true
		}
		for _, itemRow := range item.Rows.Items {
			if strings.EqualFold(itemRow.Type, "delete") {
				continue
			}
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
			if itemRow.OrgUnderscore != nil {
				for _, cell := range itemRow.OrgUnderscore.Cols {
					if !known[cell.ID] {
						return protocol.Value{}, fmt.Errorf("org column %q is not declared", cell.ID)
					}
				}
			}
			row := xmlRow(itemRow, dataset.Columns, constValues, wireTypes)
			row.MarkOrgRowPresent()
			dataset.Rows = append(dataset.Rows, row)
		}
		out.Datasets = append(out.Datasets, dataset)
	}
	return out, nil
}

func validateXMLStructure(b []byte, strict bool, profile string) (xmlSyntax, error) {
	decoder := xml.NewDecoder(bytes.NewReader(b))
	stack := []string{}
	rootSeen := false
	currentDataset := -1
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
			if !rootSeen {
				if element.Name.Local != "Root" {
					return syntax, fmt.Errorf("root element must be Root")
				}
				rootSeen = true
			}
			if element.Name.Local == "OrgRow" && len(stack) > 0 && stack[len(stack)-1] == "Rows" {
				return syntax, fmt.Errorf("OrgRow must be inside Row")
			}
			switch element.Name.Local {
			case "Dataset":
				currentDataset = len(syntax.DatasetColumnIndexes)
				syntax.DatasetColumnIndexes = append(syntax.DatasetColumnIndexes, map[string]int{})
				id := ""
				for _, attribute := range element.Attr {
					if attribute.Name.Local == "id" {
						id = attribute.Value
						break
					}
				}
				syntax.DatasetIDs = append(syntax.DatasetIDs, id)
			case "Column", "ConstColumn":
				if currentDataset >= 0 {
					id := ""
					for _, attribute := range element.Attr {
						if attribute.Name.Local == "id" {
							id = attribute.Value
							break
						}
					}
					indexes := syntax.DatasetColumnIndexes[currentDataset]
					indexes[element.Name.Local+"\x00"+id] = len(indexes)
				}
			case "Parameter":
				segment := bytes.TrimSpace(b[before:decoder.InputOffset()])
				syntax.ParameterSelfClosing = append(syntax.ParameterSelfClosing, bytes.HasSuffix(segment, []byte("/>")))
			}
			stack = append(stack, element.Name.Local)
		case xml.EndElement:
			if len(stack) == 0 || stack[len(stack)-1] != element.Name.Local {
				return syntax, fmt.Errorf("unexpected closing element %s", element.Name.Local)
			}
			stack = stack[:len(stack)-1]
			if element.Name.Local == "Dataset" {
				currentDataset = -1
			}
		}
	}
}

func xmlColumnIndex(syntax xmlSyntax, datasetIndex int, kind, id string, fallback int) int {
	if datasetIndex < len(syntax.DatasetColumnIndexes) {
		if index, ok := syntax.DatasetColumnIndexes[datasetIndex][kind+"\x00"+id]; ok {
			return index
		}
	}
	return fallback
}

func orderXMLDatasets(datasets []xdataset, ids []string) []xdataset {
	if len(ids) != len(datasets) {
		return datasets
	}
	ordered := make([]xdataset, 0, len(datasets))
	used := make([]bool, len(datasets))
	for _, id := range ids {
		for i, dataset := range datasets {
			if !used[i] && dataset.ID == id {
				ordered = append(ordered, dataset)
				used[i] = true
				break
			}
		}
	}
	if len(ordered) != len(datasets) {
		return datasets
	}
	return ordered
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

// xmlAcceptedNamespace reports whether a decoded root namespace is acceptable
// for the profile. Decode is intentionally permissive: it accepts both the
// canonical namespace emitted by Encode and the variant forms the commercial
// jars emit (tobesoft lowercase "dataset"; nexacro "nexacro.com"). Encode
// still emits only the canonical form returned by xmlNamespace.
func xmlAcceptedNamespace(profile, namespace string) bool {
	accepted := xmlAcceptedNamespaces(profile)
	if len(accepted) == 0 {
		return true
	}
	for _, value := range accepted {
		if namespace == value {
			return true
		}
	}
	return false
}

func xmlAcceptedNamespaces(profile string) []string {
	switch profile {
	case "xplatform-xml-4000":
		return []string{"http://www.tobesoft.com/platform/Dataset", "http://www.tobesoft.com/platform/dataset"}
	case "nexacro-xml-4000":
		return []string{"http://www.nexacroplatform.com/platform/dataset", "http://www.nexacro.com/platform/dataset"}
	default:
		return nil
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

func xmlRow(r xrow, columns []protocol.Column, constValues map[string]protocol.Cell, wireTypes map[string]string) protocol.Row {
	typ := map[string]string{"insert": "I", "update": "U", "normal": "N"}[strings.ToLower(r.Type)]
	if typ == "" {
		typ = "N"
	}
	row := protocol.Row{Type: typ, Values: map[string]protocol.Cell{}}
	for _, column := range columns {
		row.Values[column.ID] = protocol.Cell{State: "null"}
	}
	for id, value := range constValues {
		row.Values[id] = value
	}
	for _, cell := range r.Cols {
		row.Values[cell.ID] = normalizeXMLCell(xmlCell(cell.Text), wireTypes[cell.ID])
	}
	if r.OrgUnderscore != nil {
		for _, cell := range r.OrgUnderscore.Cols {
			row.Values[cell.ID] = normalizeXMLCell(xmlCell(cell.Text), wireTypes[cell.ID])
		}
	}
	return row
}

func normalizeDecodedParameterType(dataType string) string {
	dataType = strings.ToUpper(dataType)
	switch dataType {
	case "":
		return "STRING"
	case "FLOAT":
		return "DOUBLE"
	case "DECIMAL":
		return "STRING"
	default:
		if !isKnownType(dataType) {
			return "STRING"
		}
		return dataType
	}
}

func normalizeXMLCell(cell protocol.Cell, dataType string) protocol.Cell {
	if cell.State == "null" {
		return cell
	}
	value := cell.Lexical
	switch strings.ToUpper(dataType) {
	case "CHAR", "SHORT", "USHORT", "INT", "UINT", "LONG", "ULONG":
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			value = "0"
		}
	case "FLOAT", "DOUBLE":
		switch strings.ToLower(value) {
		case "nan":
			value = "NaN"
		case "inf", "+inf", "infinity", "+infinity":
			value = "Infinity"
		case "-inf", "-infinity":
			value = "-Infinity"
		default:
			number, err := strconv.ParseFloat(value, 64)
			if err != nil || value == "" {
				value = "0.0"
			} else {
				value = strconv.FormatFloat(number, 'g', -1, 64)
				if !strings.ContainsAny(value, ".eE") {
					value += ".0"
				}
			}
		}
	case "BIGDECIMAL":
		if _, err := strconv.ParseFloat(value, 64); err != nil || value == "" ||
			strings.EqualFold(value, "nan") || strings.Contains(strings.ToLower(value), "inf") {
			return protocol.Cell{State: "null"}
		}
		if strings.IndexByte(value, '.') >= 0 {
			value = strings.TrimRight(value, "0")
			if strings.HasSuffix(value, ".") {
				value += "0"
			}
		}
	case "DATE":
		if len(value) != 8 {
			return protocol.Cell{State: "null"}
		}
		if _, err := strconv.ParseUint(value, 10, 64); err != nil {
			return protocol.Cell{State: "null"}
		}
	case "TIME":
		if len(value) == 6 {
			value += "000"
		}
		if len(value) != 9 {
			return protocol.Cell{State: "null"}
		}
		if _, err := strconv.ParseUint(value, 10, 64); err != nil {
			return protocol.Cell{State: "null"}
		}
	case "DATETIME":
		if len(value) == 14 {
			value = normalizeLenientDateTime(value)
		} else {
			if len(value) != 17 {
				return protocol.Cell{State: "null"}
			}
			if _, err := strconv.ParseUint(value, 10, 64); err != nil {
				return protocol.Cell{State: "null"}
			}
		}
	case "BLOB", "FILE":
		if _, err := base64.StdEncoding.DecodeString(value); err != nil || value == "" {
			return protocol.Cell{State: "null"}
		}
	}
	if cell.State == "empty" && value == "" {
		return cell
	}
	cell.State = "value"
	cell.Lexical = value
	return cell
}

func xmlCell(s string) protocol.Cell {
	cell := protocol.Cell{State: "value", Lexical: s}
	if s == "" {
		cell.State = "empty"
		cell.MarkLexicalPresent()
	}
	return cell
}

func normalizeLenientDateTime(value string) string {
	component := func(offset, length int) int {
		number := 0
		for i := offset; i < offset+length; i++ {
			number = number*10 + int(value[i]) - int('0')
		}
		return number
	}
	year := component(0, 4)
	cycles := (year - 2000) / 400
	if year < 2000 && (year-2000)%400 != 0 {
		cycles--
	}
	baseYear := year - cycles*400
	normalized := time.Date(
		baseYear, time.Month(component(4, 2)), component(6, 2),
		component(8, 2), component(10, 2), component(12, 2), 0, time.UTC,
	)
	return fmt.Sprintf("%04d%02d%02d%02d%02d%02d000",
		normalized.Year()+cycles*400, normalized.Month(), normalized.Day(),
		normalized.Hour(), normalized.Minute(), normalized.Second())
}
