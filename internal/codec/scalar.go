package codec

import (
	"encoding/json"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/Clickin/xapi-conformance/internal/protocol"
)

var sourceDecimalPattern = regexp.MustCompile(`^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$`)

// applyScalarCompatibility reproduces StringTypeConverter normalization before
// a canonical value reaches a commercial serializer. Decode remains lexical:
// normalization applies only to emitted wire values.
func applyScalarCompatibility(value protocol.Value) protocol.Value {
	parameters := append([]protocol.Parameter(nil), value.Parameters...)
	for i := range parameters {
		cell := normalizeSourceCell(parameters[i].Type, protocol.Cell{State: parameters[i].State, Lexical: parameters[i].Lexical})
		parameters[i].State, parameters[i].Lexical = cell.State, cell.Lexical
	}
	value.Parameters = parameters

	datasets := append([]protocol.Dataset(nil), value.Datasets...)
	for di := range datasets {
		dataset := &datasets[di]
		constants := append([]protocol.ConstColumn(nil), dataset.ConstColumns...)
		for i := range constants {
			constants[i].Value = normalizeSourceCell(constants[i].Type, constants[i].Value)
		}
		dataset.ConstColumns = constants

		types := make(map[string]string, len(dataset.Columns))
		for _, column := range dataset.Columns {
			types[column.ID] = column.Type
		}
		rows := append([]protocol.Row(nil), dataset.Rows...)
		for i := range rows {
			rows[i].Values = normalizeSourceCells(rows[i].Values, types)
			if rows[i].OrgRow != nil {
				org := *rows[i].OrgRow
				org.Values = normalizeSourceCells(org.Values, types)
				rows[i].OrgRow = &org
			}
		}
		dataset.Rows = rows
	}
	value.Datasets = datasets
	return value
}

func normalizeSourceCells(cells map[string]protocol.Cell, types map[string]string) map[string]protocol.Cell {
	if cells == nil {
		return nil
	}
	out := make(map[string]protocol.Cell, len(cells))
	for id, cell := range cells {
		out[id] = normalizeSourceCell(types[id], cell)
	}
	return out
}

func normalizeSourceCell(dataType string, cell protocol.Cell) protocol.Cell {
	if cell.State != "value" {
		return cell
	}
	switch strings.ToUpper(defaultType(dataType)) {
	case "BOOLEAN":
		if miBoolean(cell.Lexical) {
			cell.Lexical = "1"
		} else {
			cell.Lexical = "0"
		}
	case "SHORT", "USHORT", "INT", "UINT":
		cell.Lexical = normalizeSourceInteger(cell.Lexical, true)
	case "LONG", "ULONG":
		cell.Lexical = normalizeSourceInteger(cell.Lexical, false)
	case "FLOAT":
		cell.Lexical = normalizeSourceFloat(cell.Lexical, 32)
	case "DOUBLE":
		cell.Lexical = normalizeSourceFloat(cell.Lexical, 64)
	case "DECIMAL", "BIGDECIMAL":
		cell.Lexical = normalizeSourceDecimal(cell.Lexical)
	}
	return cell
}

func normalizeSourceInteger(value string, allowHex bool) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), ",", "")
	if allowHex && (strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X")) {
		if parsed, err := strconv.ParseInt(value[2:], 16, 64); err == nil {
			return strconv.FormatInt(parsed, 10)
		}
		return "0"
	}
	if parsed, ok := new(big.Rat).SetString(value); ok {
		integer := new(big.Int).Quo(parsed.Num(), parsed.Denom())
		if integer.IsInt64() {
			return integer.String()
		}
	}
	return "0"
}

func normalizeSourceFloat(value string, bits int) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), ",", "")
	var parsed float64
	var err error
	switch value {
	case "Infinity", "+Infinity":
		parsed = math.Inf(1)
	case "-Infinity":
		parsed = math.Inf(-1)
	default:
		parsed, err = strconv.ParseFloat(value, bits)
		if err != nil {
			return "0.0"
		}
	}
	if math.IsNaN(parsed) {
		return "NaN"
	}
	if math.IsInf(parsed, 1) {
		return "Infinity"
	}
	if math.IsInf(parsed, -1) {
		return "-Infinity"
	}
	formatted := strconv.FormatFloat(parsed, 'g', -1, bits)
	if !strings.ContainsAny(formatted, ".eE") {
		formatted += ".0"
	}
	return formatted
}

func normalizeSourceDecimal(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), ",", "")
	if !sourceDecimalPattern.MatchString(value) {
		return "0"
	}
	exponent := ""
	if i := strings.IndexAny(value, "eE"); i >= 0 {
		exponent, value = value[i:], value[:i]
	}
	sign := ""
	if strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		sign, value = value[:1], value[1:]
	}
	integer, fraction, found := strings.Cut(value, ".")
	integer = strings.TrimLeft(integer, "0")
	if integer == "" {
		integer = "0"
	}
	if sign == "+" {
		sign = ""
	}
	if found {
		return sign + integer + "." + fraction + exponent
	}
	return sign + integer + exponent
}

func jsonScalar(dataType string, cell protocol.Cell) any {
	if cell.State == "null" {
		return nil
	}
	if cell.State != "value" {
		return cell.Lexical
	}
	switch strings.ToUpper(defaultType(dataType)) {
	case "BOOLEAN", "SHORT", "USHORT", "INT", "UINT", "LONG", "ULONG", "DECIMAL", "BIGDECIMAL":
		return json.Number(cell.Lexical)
	case "FLOAT", "DOUBLE":
		if cell.Lexical == "NaN" || cell.Lexical == "Infinity" || cell.Lexical == "-Infinity" {
			return nil
		}
		return json.Number(cell.Lexical)
	default:
		return cell.Lexical
	}
}
