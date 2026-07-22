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

func Decode(b []byte) (protocol.Value, error) {
	return DecodeWithStrict(b, true)
}

func DecodeWithStrict(b []byte, strict bool) (protocol.Value, error) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return protocol.Value{}, fmt.Errorf("empty document")
	}
	if b[0] == '<' {
		return xmlValue(b, strict)
	}
	return jsonValue(b)
}

func jsonValue(b []byte) (protocol.Value, error) {
	if err := protocol.RejectDuplicateKeys(b); err != nil {
		return protocol.Value{}, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(b, &root); err != nil {
		return protocol.Value{}, err
	}
	out := protocol.Value{Parameters: []protocol.Parameter{}, Datasets: []protocol.Dataset{}}
	var version string
	if json.Unmarshal(root["version"], &version) == nil {
		out.Wire = map[string]any{"version": version}
	}
	parseJSONList(root["Parameters"], &out.Parameters)
	var datasets []map[string]json.RawMessage
	if raw := root["Datasets"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &datasets); err != nil {
			return out, err
		}
	}
	for _, d := range datasets {
		ds := protocol.Dataset{}
		if err := json.Unmarshal(d["id"], &ds.ID); err != nil {
			return out, err
		}
		parseColumns(d["ColumnInfo"], &ds)
		var rows []map[string]json.RawMessage
		if err := json.Unmarshal(d["Rows"], &rows); err != nil {
			return out, err
		}
		for _, r := range rows {
			row := protocol.Row{Type: "N", Values: map[string]protocol.Cell{}}
			if raw := r["_RowType_"]; len(raw) > 0 {
				_ = json.Unmarshal(raw, &row.Type)
			}
			for _, c := range ds.Columns {
				row.Values[c.ID] = rawCell(r[c.ID])
			}
			if row.Type == "O" && len(ds.Rows) > 0 {
				org := row
				ds.Rows[len(ds.Rows)-1].OrgRow = &org
			}
			ds.Rows = append(ds.Rows, row)
		}
		out.Datasets = append(out.Datasets, ds)
	}
	return out, nil
}
func parseJSONList(raw json.RawMessage, out *[]protocol.Parameter) {
	var xs []map[string]json.RawMessage
	if json.Unmarshal(raw, &xs) != nil {
		return
	}
	for i, x := range xs {
		var p protocol.Parameter
		_ = json.Unmarshal(x["id"], &p.ID)
		_ = json.Unmarshal(x["type"], &p.Type)
		p.Type = strings.ToUpper(p.Type)
		if p.Type == "" {
			p.Type = "STRING"
		}
		p.Index = i
		if v, ok := x["value"]; ok {
			p.State, p.Lexical = rawState(v)
		} else {
			p.State = "missing"
		}
		*out = append(*out, p)
	}
}
func parseColumns(raw json.RawMessage, ds *protocol.Dataset) {
	var x map[string]json.RawMessage
	if json.Unmarshal(raw, &x) != nil {
		return
	}
	parseColumnArray(x["Column"], &ds.Columns, &ds.ConstColumns, false)
	parseColumnArray(x["ConstColumn"], &ds.Columns, &ds.ConstColumns, true)
}
func parseColumnArray(raw json.RawMessage, cols *[]protocol.Column, consts *[]protocol.ConstColumn, isConst bool) {
	var xs []map[string]json.RawMessage
	if json.Unmarshal(raw, &xs) != nil {
		return
	}
	for _, x := range xs {
		var id, typ, size, enc string
		_ = json.Unmarshal(x["id"], &id)
		_ = json.Unmarshal(x["type"], &typ)
		_ = json.Unmarshal(x["size"], &size)
		_ = json.Unmarshal(x["enc"], &enc)
		typ = strings.ToUpper(typ)
		if isConst {
			c := protocol.ConstColumn{ID: id, Type: typ, Size: size, Encoding: enc, Value: rawCell(x["value"])}
			*consts = append(*consts, c)
		} else {
			*cols = append(*cols, protocol.Column{ID: id, Type: typ, Size: size, Encoding: enc, Index: len(*cols)})
		}
	}
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
	ID       string `xml:"id,attr"`
	Type     string `xml:"type,attr"`
	Size     string `xml:"size,attr"`
	Encoding string `xml:"enc,attr"`
	Value    string `xml:"value,attr"`
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

func xmlValue(b []byte, strict bool) (protocol.Value, error) {
	if bytes.Contains(bytes.ToUpper(b), []byte("<!DOCTYPE")) {
		return protocol.Value{}, fmt.Errorf("DTD and external entities are forbidden")
	}
	if err := rejectDuplicateXMLAttrs(b); err != nil {
		return protocol.Value{}, err
	}
	if err := validateXMLStructure(b, strict); err != nil {
		return protocol.Value{}, err
	}
	var x xroot
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = true
	if err := d.Decode(&x); err != nil {
		return protocol.Value{}, err
	}
	out := protocol.Value{Parameters: []protocol.Parameter{}, Datasets: []protocol.Dataset{}, Wire: map[string]any{}}
	rootWire := map[string]any{}
	if x.XMLName.Space != "" {
		rootWire["namespace"] = x.XMLName.Space
	}
	for _, a := range x.Attr {
		if a.Name.Local == "version" || a.Name.Local == "ver" {
			rootWire[a.Name.Local] = a.Value
		}
	}
	if len(rootWire) > 0 {
		out.Wire["root"] = rootWire
	}
	for i, p := range x.Parameters.Items {
		id, typ, attrValue := "", "", ""
		for _, a := range p.Attr {
			if a.Name.Local == "id" {
				id = a.Value
			}
			if a.Name.Local == "type" {
				typ = a.Value
			}
			if a.Name.Local == "value" {
				attrValue = a.Value
			}
		}
		lexical, form := p.Text, "text"
		if attrValue != "" {
			lexical, form = attrValue, "attribute"
		}
		if typ == "" {
			typ = "STRING"
		}
		st := "value"
		if lexical == "" {
			st = "empty"
		}
		out.Parameters = append(out.Parameters, protocol.Parameter{ID: id, Type: strings.ToUpper(typ), State: st, Lexical: lexical, Index: i, Wire: map[string]any{"valueForm": form}})
	}
	datasets := append(append([]xdataset{}, x.Datasets.Items...), x.DirectDatasets...)
	for _, d := range datasets {
		ds := protocol.Dataset{ID: d.ID, Columns: []protocol.Column{}, ConstColumns: []protocol.ConstColumn{}, Rows: []protocol.Row{}}
		for i, c := range d.Columns.Items {
			ds.Columns = append(ds.Columns, protocol.Column{ID: c.ID, Type: strings.ToUpper(c.Type), Size: c.Size, Encoding: c.Encoding, Index: i})
		}
		for _, c := range d.Columns.Consts {
			st := "value"
			if c.Value == "" {
				st = "empty"
			}
			ds.ConstColumns = append(ds.ConstColumns, protocol.ConstColumn{ID: c.ID, Type: strings.ToUpper(c.Type), Size: c.Size, Encoding: c.Encoding, Value: protocol.Cell{State: st, Lexical: c.Value}})
		}
		for _, r := range d.Rows.Items {
			known := map[string]bool{}
			for _, c := range ds.Columns {
				known[c.ID] = true
			}
			for _, c := range r.Cols {
				if !known[c.ID] {
					return protocol.Value{}, fmt.Errorf("column %q is not declared", c.ID)
				}
			}
			if r.Org != nil {
				for _, c := range r.Org.Cols {
					if !known[c.ID] {
						return protocol.Value{}, fmt.Errorf("org column %q is not declared", c.ID)
					}
				}
			}
			row := xmlRow(r, ds.Columns)
			ds.Rows = append(ds.Rows, row)
		}
		out.Datasets = append(out.Datasets, ds)
	}
	return out, nil
}

func validateXMLStructure(b []byte, strict bool) error {
	allowed := map[string]map[string]bool{
		"Root": {"Parameters": true, "Datasets": true, "Dataset": true}, "Parameters": {"Parameter": true}, "Datasets": {"Dataset": true},
		"Dataset": {"ColumnInfo": true, "Rows": true}, "ColumnInfo": {"Column": true, "ConstColumn": true},
		"Rows": {"Row": true}, "Row": {"Col": true, "OrgRow": true}, "OrgRow": {"Col": true},
	}
	valid := map[string]bool{"Root": true, "Parameters": true, "Parameter": true, "Datasets": true, "Dataset": true, "ColumnInfo": true, "Column": true, "ConstColumn": true, "Rows": true, "Row": true, "OrgRow": true, "Col": true}
	d := xml.NewDecoder(bytes.NewReader(b))
	stack := []string{}
	skipDepth := 0
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch x := tok.(type) {
		case xml.StartElement:
			if skipDepth > 0 {
				skipDepth++
				continue
			}
			if !valid[x.Name.Local] {
				if !strict {
					skipDepth = 1
					continue
				}
				return fmt.Errorf("unexpected element %s", x.Name.Local)
			}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				if children, ok := allowed[parent]; ok && !children[x.Name.Local] {
					if !strict && !(parent == "Rows" && (x.Name.Local == "Col" || x.Name.Local == "OrgRow")) {
						skipDepth = 1
						continue
					}
					return fmt.Errorf("unexpected element %s under %s", x.Name.Local, parent)
				}
			}
			stack = append(stack, x.Name.Local)
		case xml.EndElement:
			if skipDepth > 0 {
				skipDepth--
				continue
			}
			if len(stack) == 0 || stack[len(stack)-1] != x.Name.Local {
				return fmt.Errorf("unexpected closing element %s", x.Name.Local)
			}
			stack = stack[:len(stack)-1]
		}
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
