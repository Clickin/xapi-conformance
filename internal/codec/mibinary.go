package codec

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Clickin/xapi-conformance/internal/protocol"
)

const (
	miBinaryProfile = "miplatform-binary-4000"
	miBinaryVersion = uint16(4000)
)

func miBinaryValue(input []byte, strict bool) (protocol.Value, error) {
	if len(input) > binaryMaxPayload {
		return protocol.Value{}, fmt.Errorf("MiBinary payload exceeds limit")
	}
	r := newBinaryReader(input)
	blockCount, err := r.u16()
	if err != nil {
		return protocol.Value{}, err
	}
	if blockCount == 0 || int(blockCount) > binaryMaxCount {
		return protocol.Value{}, fmt.Errorf("invalid MiBinary block count %d", blockCount)
	}
	value := protocol.Value{
		Parameters: []protocol.Parameter{}, Datasets: []protocol.Dataset{},
		Wire: map[string]any{"format": "MiBinary", "version": "4000"},
	}
	if err := readMiVariables(r, &value); err != nil {
		return protocol.Value{}, err
	}
	for i := 1; i < int(blockCount); i++ {
		if err := readMiDataset(r, &value); err != nil {
			return protocol.Value{}, err
		}
	}
	if strict && r.remaining() != 0 {
		return protocol.Value{}, fmt.Errorf("trailing bytes in MiBinary document")
	}
	return value, nil
}

func readMiVariables(r *binaryReader, value *protocol.Value) error {
	mark, err := r.u16()
	if err != nil {
		return err
	}
	if mark != binaryVariableMark {
		return fmt.Errorf("invalid MiBinary variable mark 0x%04x", mark)
	}
	version, err := r.u16()
	if err != nil {
		return err
	}
	if version != miBinaryVersion {
		return fmt.Errorf("invalid MiBinary variable version %d", version)
	}
	blockLen, err := r.length()
	if err != nil {
		return err
	}
	block, err := r.bytes(blockLen)
	if err != nil {
		return err
	}
	br := newBinaryReader(block)
	count, err := br.u16()
	if err != nil {
		return err
	}
	if int(count) > binaryMaxCount {
		return fmt.Errorf("too many MiBinary variables")
	}
	for i := 0; i < int(count); i++ {
		id, err := br.shortString()
		if err != nil {
			return err
		}
		tag, err := br.u16()
		if err != nil {
			return err
		}
		cell, err := readMiCell(br, tag)
		if err != nil {
			return err
		}
		value.Parameters = append(value.Parameters, protocol.Parameter{
			ID: id, Type: miVariantType(tag), State: cell.State, Lexical: cell.Lexical, Index: i,
		})
	}
	if br.remaining() != 0 {
		return fmt.Errorf("trailing bytes in MiBinary variable block")
	}
	return nil
}

func readMiDataset(r *binaryReader, value *protocol.Value) error {
	mark, err := r.u16()
	if err != nil {
		return err
	}
	if mark == 0xffff {
		return nil
	}
	if mark != binaryDatasetMark {
		return fmt.Errorf("invalid MiBinary dataset mark 0x%04x", mark)
	}
	version, err := r.u16()
	if err != nil {
		return err
	}
	if version != miBinaryVersion {
		return fmt.Errorf("invalid MiBinary dataset version %d", version)
	}
	headerLen, err := r.length()
	if err != nil {
		return err
	}
	header, err := r.bytes(headerLen)
	if err != nil {
		return err
	}
	hr := newBinaryReader(header)
	alias, err := hr.shortString()
	if err != nil {
		return err
	}
	constantMark, err := hr.u16()
	if err != nil {
		return err
	}
	if constantMark != binaryVariableMark {
		return fmt.Errorf("invalid MiBinary constant-column mark 0x%04x", constantMark)
	}
	constantVersion, err := hr.u16()
	if err != nil {
		return err
	}
	if constantVersion != miBinaryVersion {
		return fmt.Errorf("invalid MiBinary constant-column version %d", constantVersion)
	}
	constantLen, err := hr.length()
	if err != nil {
		return err
	}
	constantBlock, err := hr.bytes(constantLen)
	if err != nil {
		return err
	}
	cr := newBinaryReader(constantBlock)
	constantCount, err := cr.u16()
	if err != nil {
		return err
	}
	if int(constantCount) > binaryMaxCount {
		return fmt.Errorf("too many MiBinary constant columns")
	}
	constants := make([]protocol.ConstColumn, 0, constantCount)
	for i := 0; i < int(constantCount); i++ {
		id, err := cr.shortString()
		if err != nil {
			return err
		}
		tag, err := cr.u16()
		if err != nil {
			return err
		}
		cell, err := readMiCell(cr, tag)
		if err != nil {
			return err
		}
		constants = append(constants, protocol.ConstColumn{ID: id, Type: miVariantType(tag), Value: cell})
	}
	if cr.remaining() != 0 {
		return fmt.Errorf("trailing bytes in MiBinary constant block")
	}
	columnCount, err := hr.u16()
	if err != nil {
		return err
	}
	if int(columnCount) > binaryMaxCount {
		return fmt.Errorf("too many MiBinary columns")
	}
	columns := make([]protocol.Column, 0, columnCount)
	for i := 0; i < int(columnCount); i++ {
		id, err := hr.shortString()
		if err != nil {
			return err
		}
		code, err := hr.u16()
		if err != nil {
			return err
		}
		size, err := hr.u16()
		if err != nil {
			return err
		}
		attr, err := hr.u16()
		if err != nil {
			return err
		}
		column := protocol.Column{ID: id, Type: miColumnType(code), Index: i}
		if size != 0 {
			column.Size = strconv.Itoa(int(size))
		}
		if attr&0x00f0 == 0x0060 {
			sumText, err := hr.shortString()
			if err != nil {
				return err
			}
			column.Prop, column.SumText = "SUM", sumText
		}
		columns = append(columns, column)
	}
	if hr.remaining() != 0 {
		return fmt.Errorf("trailing bytes in MiBinary dataset header")
	}
	dataset := protocol.Dataset{
		ID: alias, Columns: columns, ConstColumns: constants, Rows: []protocol.Row{},
		Wire: map[string]any{"format": "MiBinary", "alias": alias, "version": "4000"},
	}
	orderedColumns := miRowColumns(columns)
	for {
		rowLen, err := r.length()
		if err != nil {
			return err
		}
		rowType, err := r.u16()
		if err != nil {
			return err
		}
		if rowLen == 0 && rowType == 0 {
			break
		}
		if rowLen < 4 {
			return fmt.Errorf("invalid MiBinary row length %d", rowLen)
		}
		rowBody, err := r.bytes(rowLen - 2)
		if err != nil {
			return err
		}
		rr := newBinaryReader(rowBody)
		count, err := rr.u16()
		if err != nil {
			return err
		}
		if int(count) > binaryMaxCount {
			return fmt.Errorf("too many MiBinary row values")
		}
		row := protocol.Row{Type: binaryRowType(rowType), Values: map[string]protocol.Cell{}}
		for i := 0; i < int(count); i++ {
			tag, err := rr.u16()
			if err != nil {
				return err
			}
			cell, err := readMiCell(rr, tag)
			if err != nil {
				return err
			}
			if i < len(orderedColumns) {
				row.Values[orderedColumns[i].ID] = miCellForType(cell, orderedColumns[i].Type, tag)
			}
		}
		for _, column := range columns {
			if _, ok := row.Values[column.ID]; !ok {
				row.Values[column.ID] = protocol.Cell{State: "missing"}
			}
		}
		if row.Type == "U" {
			savedMarker, err := rr.i16()
			if err != nil {
				return err
			}
			if savedMarker >= 0 {
				org := &protocol.Row{Type: "O", Values: map[string]protocol.Cell{}}
				for i := 0; i < int(count); i++ {
					tag, err := rr.u16()
					if err != nil {
						return err
					}
					cell, err := readMiCell(rr, tag)
					if err != nil {
						return err
					}
					if i < len(orderedColumns) {
						org.Values[orderedColumns[i].ID] = miCellForType(cell, orderedColumns[i].Type, tag)
					}
				}
				row.OrgRow = org
			}
		}
		if rr.remaining() != 0 {
			return fmt.Errorf("trailing bytes in MiBinary row")
		}
		dataset.Rows = append(dataset.Rows, row)
		if len(dataset.Rows) > binaryMaxCount {
			return fmt.Errorf("too many MiBinary rows")
		}
	}
	value.Datasets = append(value.Datasets, dataset)
	return nil
}

func readMiCell(r *binaryReader, tag uint16) (protocol.Cell, error) {
	switch tag {
	case 0:
		return protocol.Cell{State: "empty"}, nil
	case 3:
		value, err := r.i32()
		return protocol.Cell{State: "value", Lexical: strconv.FormatInt(int64(value), 10)}, err
	case 5:
		value, err := r.f64()
		return protocol.Cell{State: "value", Lexical: strconv.FormatFloat(value, 'g', -1, 64)}, err
	case 6:
		hi, err := r.i32()
		if err != nil {
			return protocol.Cell{}, err
		}
		low, err := r.i32()
		if err != nil {
			return protocol.Cell{}, err
		}
		return protocol.Cell{State: "value", Lexical: miDecimalString(hi, low)}, nil
	case 7:
		value, err := r.f64()
		if err != nil {
			return protocol.Cell{}, err
		}
		lexical, err := miDateLexical(value)
		return protocol.Cell{State: "value", Lexical: lexical}, err
	case 8:
		length, err := r.length()
		if err != nil {
			return protocol.Cell{}, err
		}
		value, err := r.bytes(length)
		if err != nil {
			return protocol.Cell{}, err
		}
		if len(value) == 0 {
			return protocol.Cell{State: "empty"}, nil
		}
		return protocol.Cell{State: "value", Lexical: string(value)}, nil
	case 13:
		length, err := r.length()
		if err != nil {
			return protocol.Cell{}, err
		}
		value, err := r.bytes(length)
		if err != nil {
			return protocol.Cell{}, err
		}
		return protocol.Cell{State: "value", Lexical: base64.StdEncoding.EncodeToString(value)}, nil
	default:
		return protocol.Cell{}, fmt.Errorf("invalid MiBinary value type 0x%04x", tag)
	}
}

func miCellForType(cell protocol.Cell, dataType string, tag uint16) protocol.Cell {
	if cell.State != "value" {
		return cell
	}
	if isBlobType(dataType) && tag == 8 {
		cell.Lexical = base64.StdEncoding.EncodeToString([]byte(cell.Lexical))
	}
	return cell
}

func miVariantType(tag uint16) string {
	switch tag {
	case 3:
		return "INT"
	case 5:
		return "DOUBLE"
	case 6:
		return "BIGDECIMAL"
	case 7:
		return "DATETIME"
	case 8:
		return "STRING"
	case 13:
		return "BLOB"
	default:
		return "UNDEFINED"
	}
}

func miColumnType(code uint16) string {
	switch code {
	case 1, 10, 11, 12:
		return "STRING"
	case 2:
		return "INT"
	case 4:
		return "DOUBLE"
	case 5:
		return "BIGDECIMAL"
	case 8:
		return "DATETIME"
	case 9:
		return "BLOB"
	default:
		return "UNDEFINED"
	}
}

func encodeMiBinary(value protocol.Value) ([]byte, error) {
	if len(value.Datasets)+1 > math.MaxUint16 {
		return nil, fmt.Errorf("too many MiBinary datasets")
	}
	var out bytes.Buffer
	writeU16(&out, uint16(len(value.Datasets)+1))
	variables := &bytes.Buffer{}
	writeU16(variables, uint16(len(value.Parameters)))
	for _, parameter := range value.Parameters {
		if err := writeBinaryShortString(variables, parameter.ID); err != nil {
			return nil, err
		}
		if err := writeMiTypedValue(variables, parameter.Type, protocol.Cell{State: parameter.State, Lexical: parameter.Lexical}); err != nil {
			return nil, err
		}
	}
	writeU16(&out, binaryVariableMark)
	writeU16(&out, miBinaryVersion)
	writeBinaryLength(&out, variables.Len())
	out.Write(variables.Bytes())
	for _, dataset := range value.Datasets {
		if err := writeMiDataset(&out, dataset); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

func writeMiDataset(out *bytes.Buffer, dataset protocol.Dataset) error {
	header := &bytes.Buffer{}
	if err := writeBinaryShortString(header, miDatasetAlias(dataset)); err != nil {
		return err
	}
	constants := &bytes.Buffer{}
	writeU16(constants, uint16(len(dataset.ConstColumns)))
	for _, column := range dataset.ConstColumns {
		if err := writeBinaryShortString(constants, column.ID); err != nil {
			return err
		}
		if err := writeMiTypedValue(constants, column.Type, column.Value); err != nil {
			return err
		}
	}
	writeU16(header, binaryVariableMark)
	writeU16(header, miBinaryVersion)
	writeBinaryLength(header, constants.Len())
	header.Write(constants.Bytes())
	writeU16(header, uint16(len(dataset.Columns)))
	for _, column := range dataset.Columns {
		if err := writeBinaryShortString(header, column.ID); err != nil {
			return err
		}
		writeU16(header, miColumnCode(column.Type))
		size := 0
		if column.Size != "" {
			size, _ = strconv.Atoi(column.Size)
		}
		writeU16(header, uint16(size))
		writeU16(header, 1)
	}
	writeU16(out, binaryDatasetMark)
	writeU16(out, miBinaryVersion)
	writeBinaryLength(out, header.Len())
	out.Write(header.Bytes())
	orderedColumns := miRowColumns(dataset.Columns)
	for _, row := range dataset.Rows {
		body := &bytes.Buffer{}
		writeU16(body, binaryRowCode(row.Type))
		writeU16(body, uint16(len(orderedColumns)))
		for _, column := range orderedColumns {
			if err := writeMiTypedValue(body, column.Type, row.Values[column.ID]); err != nil {
				return err
			}
		}
		if strings.EqualFold(row.Type, "U") {
			if row.OrgRow == nil {
				writeU16(body, 0xffff)
			} else {
				writeU16(body, 0)
				for _, column := range orderedColumns {
					if err := writeMiTypedValue(body, column.Type, row.OrgRow.Values[column.ID]); err != nil {
						return err
					}
				}
			}
		}
		writeBinaryLength(out, body.Len())
		out.Write(body.Bytes())
	}
	writeU32(out, 0)
	return nil
}

func writeMiTypedValue(out *bytes.Buffer, dataType string, cell protocol.Cell) error {
	if cell.State != "value" && cell.State != "empty" {
		writeU16(out, 0)
		return nil
	}
	typeName := strings.ToUpper(defaultType(dataType))
	switch typeName {
	case "STRING", "CHAR", "DECIMAL", "BIGDECIMAL":
		writeU16(out, 8)
		return writeBinaryLengthBytes(out, []byte(cell.Lexical))
	case "SHORT", "USHORT", "INT", "UINT":
		value, err := parseBinaryInt(cell.Lexical)
		if err != nil {
			value = 0
		}
		writeU16(out, 3)
		writeI32(out, int32(value))
		return nil
	case "BOOLEAN":
		writeU16(out, 3)
		if miBoolean(cell.Lexical) {
			writeI32(out, 1)
		} else {
			writeI32(out, 0)
		}
		return nil
	case "LONG", "ULONG":
		return writeMiDecimal(out, cell.Lexical)
	case "FLOAT", "DOUBLE":
		value, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(cell.Lexical), ",", ""), 64)
		if err != nil {
			value = 0
		}
		writeU16(out, 5)
		writeF64(out, value)
		return nil
	case "DATE", "TIME", "DATETIME":
		value, err := miDateDouble(cell.Lexical)
		if err != nil {
			writeU16(out, 0)
			return nil
		}
		writeU16(out, 7)
		writeF64(out, value)
		return nil
	case "BLOB", "FILE":
		value, err := base64.StdEncoding.DecodeString(cell.Lexical)
		if err != nil {
			return err
		}
		writeU16(out, 13)
		return writeBinaryLengthBytes(out, value)
	default:
		writeU16(out, 0)
		return nil
	}
}

func writeMiDecimal(out *bytes.Buffer, lexical string) error {
	value, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(lexical), ",", ""), 64)
	if err != nil {
		value = 0
	}
	scaled := int64(math.RoundToEven(value * 10000))
	writeU16(out, 6)
	writeI32(out, int32(scaled>>32))
	writeI32(out, int32(scaled))
	return nil
}

func miColumnCode(dataType string) uint16 {
	switch strings.ToUpper(defaultType(dataType)) {
	case "STRING", "CHAR", "DECIMAL", "BIGDECIMAL":
		return 1
	case "SHORT", "USHORT", "INT", "UINT", "BOOLEAN":
		return 2
	case "FLOAT", "DOUBLE":
		return 4
	case "LONG", "ULONG":
		return 5
	case "DATE", "TIME", "DATETIME":
		return 8
	case "BLOB", "FILE":
		return 9
	default:
		return 0
	}
}

func miRowColumns(columns []protocol.Column) []protocol.Column {
	ordered := append([]protocol.Column(nil), columns...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return strings.ToLower(ordered[i].ID) < strings.ToLower(ordered[j].ID)
	})
	return ordered
}

func miDecimalString(hi, low int32) string {
	value := int64(uint64(uint32(hi))<<32 | uint64(uint32(low)))
	text := strconv.FormatInt(value, 10)
	length := len(text)
	switch {
	case length < 4:
		return "0"
	case length == 4:
		return "0." + text
	default:
		return text[:length-4] + "." + text[length-4:]
	}
}

func miDateDouble(lexical string) (float64, error) {
	lexical = strings.TrimSpace(lexical)
	layouts := []string{"20060102150405000", "20060102150405", "20060102", "150405000", "150405", time.RFC3339Nano}
	var parsed time.Time
	var err error
	for _, layout := range layouts {
		parsed, err = time.ParseInLocation(layout, lexical, time.UTC)
		if err == nil {
			ordinal := miOrdinal(parsed.Year(), int(parsed.Month()), parsed.Day())
			seconds := float64(parsed.Hour()*3600+parsed.Minute()*60+parsed.Second()) + float64(parsed.Nanosecond())/1e9
			return float64(ordinal) + seconds/86400, nil
		}
	}
	return 0, fmt.Errorf("invalid MiBinary date %q", lexical)
}

func miDateLexical(value float64) (string, error) {
	if value < 0 || value > 3652424 {
		return "", fmt.Errorf("invalid MiBinary date value %v", value)
	}
	whole := int64(math.Floor(value))
	fraction := value - float64(whole)
	lo, hi := 0, 10000
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if miOrdinal(mid, 1, 1) <= whole {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	year := lo
	dayOfYear := int(whole-miOrdinal(year, 1, 1)) + 1
	if dayOfYear <= 0 {
		year--
		dayOfYear = int(whole-miOrdinal(year, 1, 1)) + 1
	}
	date := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, dayOfYear-1)
	millis := int64(math.Round(fraction * 86400000))
	date = date.Add(time.Duration(millis) * time.Millisecond)
	return date.Format("20060102150405000"), nil
}

func miOrdinal(year, month, day int) int64 {
	monthDays := [...]int{0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334, 365}
	leap := year&3 == 0 && (year%100 != 0 || year%400 == 0)
	ordinal := int64(year*365 + year/4 - year/100 + year/400 + monthDays[month-1] + day)
	if month <= 2 && leap {
		ordinal--
	}
	return ordinal
}
