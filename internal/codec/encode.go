package codec

import (
	"encoding/json"
	"encoding/xml"
	"fmt"

	"github.com/Clickin/xapi-conformance/internal/protocol"
)

func Encode(v protocol.Value, profile string) ([]byte, error) {
	switch profile {
	case "nexacro-json-1.0":
		return json.Marshal(toJSON(v))
	case "xplatform-xml-4000", "nexacro-xml-4000":
		r := toXML(v)
		b, err := xml.Marshal(r)
		if err != nil {
			return nil, err
		}
		return append([]byte(`<?xml version="1.0" encoding="UTF-8"?>`), b...), nil
	default:
		return nil, fmt.Errorf("unsupported profile %q", profile)
	}
}

func toJSON(v protocol.Value) map[string]any {
	root := map[string]any{"version": "1.0", "Parameters": []any{}, "Datasets": []any{}}
	if v.Wire != nil {
		if s, ok := v.Wire["version"].(string); ok {
			root["version"] = s
		}
	}
	params := make([]any, 0, len(v.Parameters))
	for _, p := range v.Parameters {
		x := map[string]any{"id": p.ID, "type": p.Type}
		if p.State != "missing" {
			if p.State == "null" {
				x["value"] = nil
			} else {
				x["value"] = p.Lexical
			}
		}
		params = append(params, x)
	}
	root["Parameters"] = params
	datasets := make([]any, 0, len(v.Datasets))
	for _, d := range v.Datasets {
		cols := make([]any, 0, len(d.Columns))
		for _, c := range d.Columns {
			cols = append(cols, map[string]any{"id": c.ID, "type": c.Type, "size": c.Size})
		}
		consts := make([]any, 0, len(d.ConstColumns))
		for _, c := range d.ConstColumns {
			consts = append(consts, map[string]any{"id": c.ID, "type": c.Type, "size": c.Size, "value": cellJSON(c.Value)})
		}
		rows := []any{}
		for ri, r := range d.Rows {
			if r.Type == "O" && ri > 0 && d.Rows[ri-1].OrgRow != nil && sameRow(*d.Rows[ri-1].OrgRow, r) {
				continue
			}
			x := map[string]any{"_RowType_": r.Type}
			for id, c := range r.Values {
				if c.State != "missing" {
					x[id] = cellJSON(c)
				}
			}
			rows = append(rows, x)
			if r.OrgRow != nil {
				ox := map[string]any{"_RowType_": "O"}
				for id, c := range r.OrgRow.Values {
					if c.State != "missing" {
						ox[id] = cellJSON(c)
					}
				}
				rows = append(rows, ox)
			}
		}
		datasets = append(datasets, map[string]any{"id": d.ID, "ColumnInfo": map[string]any{"ConstColumn": consts, "Column": cols}, "Rows": rows})
	}
	root["Datasets"] = datasets
	return root
}
func cellJSON(c protocol.Cell) any {
	if c.State == "null" {
		return nil
	}
	return c.Lexical
}

type xmlRoot struct {
	XMLName    xml.Name     `xml:"Root"`
	Namespace  string       `xml:"xmlns,attr"`
	Version    string       `xml:"version,attr"`
	Ver        string       `xml:"ver,attr,omitempty"`
	Parameters *xmlParams   `xml:"Parameters,omitempty"`
	Datasets   *xmlDatasets `xml:"Datasets,omitempty"`
}
type xmlParams struct {
	Items []xmlParam `xml:"Parameter"`
}
type xmlParam struct {
	ID        string `xml:"id,attr"`
	Type      string `xml:"type,attr"`
	ValueAttr string `xml:"value,attr,omitempty"`
	Value     string `xml:",chardata"`
}
type xmlDatasets struct {
	Items []xmlDataset `xml:"Dataset"`
}
type xmlDataset struct {
	ID   string        `xml:"id,attr"`
	Info xmlColumnInfo `xml:"ColumnInfo"`
	Rows xmlRows       `xml:"Rows"`
}
type xmlColumnInfo struct {
	Columns []xmlColumn `xml:"Column"`
	Consts  []xmlConst  `xml:"ConstColumn"`
}
type xmlColumn struct {
	ID       string `xml:"id,attr"`
	Type     string `xml:"type,attr"`
	Size     string `xml:"size,attr,omitempty"`
	Encoding string `xml:"enc,attr,omitempty"`
}
type xmlConst struct {
	ID       string `xml:"id,attr"`
	Type     string `xml:"type,attr"`
	Size     string `xml:"size,attr,omitempty"`
	Encoding string `xml:"enc,attr,omitempty"`
	Value    string `xml:"value,attr"`
}
type xmlRows struct {
	Items []xmlRowOut `xml:"Row"`
}
type xmlRowOut struct {
	Type string      `xml:"type,attr,omitempty"`
	Cols []xmlColOut `xml:"Col"`
	Org  *xmlOrgOut  `xml:"OrgRow,omitempty"`
}
type xmlOrgOut struct {
	Cols []xmlColOut `xml:"Col"`
}
type xmlColOut struct {
	ID    string `xml:"id,attr"`
	Value string `xml:",chardata"`
}

func toXML(v protocol.Value) xmlRoot {
	r := xmlRoot{Namespace: "http://www.nexacroplatform.com/platform/dataset", Version: "4000"}
	if root, ok := v.Wire["root"].(map[string]any); ok {
		if ver, ok := root["ver"].(string); ok {
			r.Version = ""
			r.Ver = ver
		}
		if version, ok := root["version"].(string); ok {
			r.Version = version
		}
	}
	if v.Parameters != nil {
		r.Parameters = &xmlParams{}
	}
	for _, p := range v.Parameters {
		xp := xmlParam{ID: p.ID, Type: p.Type, Value: p.Lexical}
		if p.Wire != nil {
			if form, ok := p.Wire["valueForm"].(string); ok && form == "attribute" {
				xp.ValueAttr = p.Lexical
				xp.Value = ""
			}
		}
		r.Parameters.Items = append(r.Parameters.Items, xp)
	}
	if v.Datasets != nil {
		r.Datasets = &xmlDatasets{}
	}
	for _, d := range v.Datasets {
		xd := xmlDataset{ID: d.ID}
		for _, c := range d.Columns {
			xd.Info.Columns = append(xd.Info.Columns, xmlColumn{ID: c.ID, Type: c.Type, Size: c.Size, Encoding: c.Encoding})
		}
		for _, c := range d.ConstColumns {
			xd.Info.Consts = append(xd.Info.Consts, xmlConst{ID: c.ID, Type: c.Type, Size: c.Size, Encoding: c.Encoding, Value: c.Value.Lexical})
		}
		for ri, row := range d.Rows {
			if row.Type == "O" && ri > 0 && d.Rows[ri-1].OrgRow != nil && sameRow(*d.Rows[ri-1].OrgRow, row) {
				continue
			}
			xo := xmlRowOut{Type: map[string]string{"I": "insert", "U": "update", "D": "delete", "N": "normal"}[row.Type]}
			for id, c := range row.Values {
				if c.State != "missing" {
					xo.Cols = append(xo.Cols, xmlColOut{ID: id, Value: c.Lexical})
				}
			}
			if row.OrgRow != nil {
				org := &xmlOrgOut{}
				for id, c := range row.OrgRow.Values {
					if c.State != "missing" {
						org.Cols = append(org.Cols, xmlColOut{ID: id, Value: c.Lexical})
					}
				}
				xo.Org = org
			}
			xd.Rows.Items = append(xd.Rows.Items, xo)
		}
		r.Datasets.Items = append(r.Datasets.Items, xd)
	}
	return r
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
