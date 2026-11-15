// xapi-capture records the licensed original drivers as the expected result.
//
//	xapi-capture capture -cases capture/cases -out vectors/driver
//	xapi-capture diff -n 300 -seed 1
//
// Both subcommands talk to meta/xapi-original-adapter HTTP servers (one per
// driver). capture turns each case into a vector whose expectation is exactly
// what the driver returned; diff sends the same random canonical values to the
// drivers and to a second adapter (normally the Go reference) and reports every
// difference.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type caseFile struct {
	Description string       `json:"description"`
	Cases       []recordCase `json:"cases"`
}

type recordCase struct {
	ID        string         `json:"id"`
	Profile   string         `json:"profile"`
	Operation string         `json:"operation"`
	Options   map[string]any `json:"options,omitempty"`
	Value     map[string]any `json:"value,omitempty"`
	// Text is decode input written as a UTF-8 string; Base64 is raw bytes.
	Text   *string `json:"text,omitempty"`
	Base64 string  `json:"base64,omitempty"`
	// AlsoDecode adds "<id>.decode": the driver decoding its own encode output.
	AlsoDecode bool `json:"alsoDecode,omitempty"`
	// Faithful keeps the case only when the driver reads its own encode
	// output back to the same cells. Output the driver itself misreads (SSV
	// separators inside values, characters XML 1.0 forbids) is a driver
	// defect, not a format rule, so it is not recorded.
	Faithful bool   `json:"faithful,omitempty"`
	Note     string `json:"note,omitempty"`
}

var client = &http.Client{Timeout: 30 * time.Second}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "capture":
		capture(os.Args[2:])
	case "diff":
		diff(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: xapi-capture capture|diff [flags]")
	os.Exit(2)
}

func driverFor(profile string, nexa, xp string) string {
	if strings.HasPrefix(profile, "xplatform") {
		return xp
	}
	return nexa
}

func capture(args []string) {
	fs := flag.NewFlagSet("capture", flag.ExitOnError)
	cases := fs.String("cases", "capture/cases", "directory of case files")
	out := fs.String("out", "vectors/driver", "output directory")
	nexa := fs.String("nexa", "http://127.0.0.1:8790", "Nexacro driver adapter")
	xp := fs.String("xp", "http://127.0.0.1:8789", "XPLATFORM driver adapter")
	fs.Parse(args)

	files, err := filepath.Glob(filepath.Join(*cases, "*.json"))
	check(err)
	sort.Strings(files)
	seen := map[string]string{}
	written, skipped := 0, 0
	for _, file := range files {
		var cf caseFile
		b, err := os.ReadFile(file)
		check(err)
		check(json.Unmarshal(b, &cf))
		group := strings.TrimSuffix(filepath.Base(file), ".json")
		check(os.MkdirAll(filepath.Join(*out, group), 0o755))
		for _, c := range cf.Cases {
			if prev, dup := seen[c.ID]; dup {
				fatalf("duplicate case id %s in %s and %s", c.ID, prev, file)
			}
			seen[c.ID] = file
			driver := driverFor(c.Profile, *nexa, *xp)
			v := record(c, filepath.ToSlash(file), driver)
			var dv map[string]any
			if c.AlsoDecode || c.Faithful {
				exp := v["expect"].(map[string]any)
				if exp["kind"] != "wire" {
					fatalf("%s: alsoDecode needs a successful encode", c.ID)
				}
				output := exp["output"].(map[string]any)
				raw, err := base64.StdEncoding.DecodeString(output["data"].(string))
				check(err)
				text := string(raw)
				d := recordCase{ID: c.ID + ".decode", Profile: c.Profile, Operation: "decode", Text: &text}
				dv = record(d, filepath.ToSlash(file), driver)
			}
			if c.Faithful {
				if why := unfaithful(c.Value, dv["expect"].(map[string]any)); why != "" {
					fmt.Printf("skip %s: driver reads its output back differently (%s)\n", c.ID, why)
					skipped++
					continue
				}
			}
			write(filepath.Join(*out, group, c.ID+".json"), v)
			written++
			if c.AlsoDecode {
				write(filepath.Join(*out, group, dv["id"].(string)+".json"), dv)
				written++
			}
		}
	}
	fmt.Printf("wrote %d vectors, skipped %d cases\n", written, skipped)
}

func write(path string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	check(err)
	check(os.WriteFile(path, append(b, '\n'), 0o644))
}

func record(c recordCase, source, driver string) map[string]any {
	options := c.Options
	if options == nil {
		options = map[string]any{}
	}
	body := map[string]any{"case": c.ID, "operation": c.Operation, "profile": c.Profile, "options": options}
	switch c.Operation {
	case "encode":
		if c.Value == nil {
			fatalf("%s: encode needs value", c.ID)
		}
		body["value"] = c.Value
	case "decode", "roundtrip":
		data := c.Base64
		if c.Text != nil {
			data = base64.StdEncoding.EncodeToString([]byte(*c.Text))
		}
		if c.Text == nil && c.Base64 == "" {
			fatalf("%s: %s needs text or base64", c.ID, c.Operation)
		}
		body["input"] = map[string]any{"encoding": "base64", "data": data}
	default:
		fatalf("%s: unknown operation %q", c.ID, c.Operation)
	}
	resp := post(driver, c.Operation, body)

	v := map[string]any{"id": c.ID, "operation": c.Operation, "profile": c.Profile}
	if len(options) > 0 {
		v["options"] = options
	}
	if c.Value != nil {
		v["value"] = c.Value
	}
	if in, ok := body["input"]; ok {
		v["input"] = in
	}
	ok, _ := resp["ok"].(bool)
	switch {
	case !ok:
		e, _ := resp["error"].(map[string]any)
		if e == nil {
			fatalf("%s: driver returned neither ok nor error: %v", c.ID, resp)
		}
		v["expect"] = map[string]any{"kind": "error", "error": map[string]any{"class": e["class"]}}
	case c.Operation == "encode":
		v["expect"] = map[string]any{"kind": "wire", "output": resp["output"]}
	case c.Operation == "roundtrip":
		v["expect"] = map[string]any{"kind": "wire", "output": resp["output"], "value": resp["value"]}
	default:
		v["expect"] = map[string]any{"kind": "canonical", "value": resp["value"]}
	}
	v["source"] = map[string]any{
		"repository": "xapi-conformance",
		"commit":     "working-tree",
		"path":       source,
		"license":    "repository",
	}
	v["required"] = true
	return v
}

// unfaithful compares every cell of the encoded value with the driver's
// decoding of its own output. Decoded constants appear as row values.
func unfaithful(in map[string]any, exp map[string]any) string {
	if exp["kind"] != "canonical" {
		return "decode failed"
	}
	out := exp["value"].(map[string]any)
	cell := func(m map[string]any) string {
		if m == nil {
			return "<absent>"
		}
		return fmt.Sprintf("%v:%q", m["state"], m["lexical"])
	}
	asMap := func(v any) map[string]any { m, _ := v.(map[string]any); return m }
	asList := func(v any) []any { l, _ := v.([]any); return l }
	outParams := map[string]map[string]any{}
	for _, p := range asList(out["parameters"]) {
		outParams[asMap(p)["id"].(string)] = asMap(p)
	}
	for _, p := range asList(in["parameters"]) {
		pm := asMap(p)
		if a, b := cell(pm), cell(outParams[pm["id"].(string)]); a != b {
			return fmt.Sprintf("parameter %v: %s -> %s", pm["id"], a, b)
		}
	}
	outSets := asList(out["datasets"])
	for i, d := range asList(in["datasets"]) {
		if i >= len(outSets) {
			return "dataset missing"
		}
		dm, om := asMap(d), asMap(outSets[i])
		inRows, outRows := asList(dm["rows"]), asList(om["rows"])
		if len(inRows) != len(outRows) {
			return "row count"
		}
		for r := range inRows {
			iv, ov := asMap(asMap(inRows[r])["values"]), asMap(asMap(outRows[r])["values"])
			for k, c := range iv {
				if a, b := cell(asMap(c)), cell(asMap(ov[k])); a != b {
					return fmt.Sprintf("row %d %s: %s -> %s", r, k, a, b)
				}
			}
			for _, k := range asList(dm["constColumns"]) {
				km := asMap(k)
				if a, b := cell(asMap(km["value"])), cell(asMap(ov[km["id"].(string)])); a != b {
					return fmt.Sprintf("constant %v: %s -> %s", km["id"], a, b)
				}
			}
		}
	}
	return ""
}

func post(base, op string, body any) map[string]any {
	b, err := json.Marshal(body)
	check(err)
	resp, err := client.Post(base+"/"+op, "application/json", bytes.NewReader(b))
	check(err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	check(err)
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		fatalf("%s/%s: %v: %s", base, op, err, raw)
	}
	return out
}

// ---- differential check ----

var profiles = []string{"nexacro-xml-4000", "xplatform-xml-5000", "nexacro-ssv", "xplatform-ssv", "nexacro-json-1.0", "nexacro-binary-5000", "xplatform-binary-5000"}

var chars = []string{"a", "b", "c", "X", "Y", "Z", "0", "1", "9", " ", "&", "<", ">", "\"", "'", "\t", "\r", "\n", "한", "글", "]]>", "&amp;", "&#10;"}

type gen struct {
	r        *rand.Rand
	types    []string
	features map[string]bool
}

func diff(args []string) {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	n := fs.Int("n", 200, "number of random values")
	seed := fs.Int64("seed", 1, "random seed")
	nexa := fs.String("nexa", "http://127.0.0.1:8790", "Nexacro driver adapter")
	xp := fs.String("xp", "http://127.0.0.1:8789", "XPLATFORM driver adapter")
	other := fs.String("other", "http://127.0.0.1:8791", "adapter compared with the drivers")
	feat := fs.String("features", "types,str,states,rows,orgrow,consts,params", "generator features")
	only := fs.String("profile", "", "limit to one profile")
	examples := fs.String("examples", "", "write up to 8 examples per difference kind to this JSON file")
	fs.Parse(args)

	g := gen{r: rand.New(rand.NewSource(*seed)), features: map[string]bool{}}
	for _, f := range strings.Split(*feat, ",") {
		g.features[f] = true
	}
	g.types = []string{"STRING"}
	if g.features["types"] {
		g.types = []string{"STRING", "INT", "FLOAT", "DECIMAL", "BIGDECIMAL", "DATE", "DATETIME", "TIME", "BLOB", "LONG", "BOOLEAN"}
	}
	stats := map[string]map[string]int{}
	ex := map[string][]any{}
	note := func(p, k string, e any) {
		if stats[p] == nil {
			stats[p] = map[string]int{}
		}
		stats[p][k]++
		if e != nil && len(ex[p+"|"+k]) < 8 {
			ex[p+"|"+k] = append(ex[p+"|"+k], e)
		}
	}
	for i := 0; i < *n; i++ {
		v := g.value()
		for _, p := range profiles {
			if *only != "" && p != *only {
				continue
			}
			if strings.Contains(p, "binary") && hasTimeConstant(v) {
				// The binary drivers put a TIME constant on the current date.
				note(p, "skipped TIME constant", nil)
				continue
			}
			drv := driverFor(p, *nexa, *xp)
			body := map[string]any{"case": fmt.Sprintf("diff-%d", i), "operation": "encode", "profile": p, "options": map[string]any{"strict": false}, "value": v}
			o, g2 := post(drv, "encode", body), post(*other, "encode", body)
			ook, _ := o["ok"].(bool)
			gok, _ := g2["ok"].(bool)
			switch {
			case !ook && !gok:
				note(p, "encode both-error", nil)
				continue
			case ook != gok:
				note(p, fmt.Sprintf("encode driver-ok=%v other-ok=%v", ook, gok), map[string]any{"value": v, "driver": o["error"], "other": g2["error"]})
				continue
			}
			ob, gb := outBytes(o), outBytes(g2)
			if bytes.Equal(ob, gb) {
				note(p, "encode equal", nil)
			} else {
				at := firstDiff(ob, gb)
				note(p, "encode bytes differ", map[string]any{"value": v, "at": at, "driver": window(ob, at), "other": window(gb, at)})
			}
			db := map[string]any{"case": body["case"], "operation": "decode", "profile": p, "options": map[string]any{"strict": false}, "input": o["output"]}
			od, gd := post(drv, "decode", db), post(*other, "decode", db)
			odok, _ := od["ok"].(bool)
			gdok, _ := gd["ok"].(bool)
			switch {
			case odok && gdok:
				if canon(od["value"]) == canon(gd["value"]) {
					note(p, "decode equal", nil)
				} else {
					note(p, "decode canonical differ", map[string]any{"input": string(ob), "driver": od["value"], "other": gd["value"]})
				}
			case odok != gdok:
				note(p, fmt.Sprintf("decode driver-ok=%v other-ok=%v", odok, gdok), map[string]any{"input": string(ob), "driver": od["error"], "other": gd["error"]})
			default:
				note(p, "decode both-error", nil)
			}
		}
	}
	for _, p := range profiles {
		if s, ok := stats[p]; ok {
			keys := make([]string, 0, len(s))
			for k := range s {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := make([]string, 0, len(keys))
			for _, k := range keys {
				parts = append(parts, fmt.Sprintf("%s=%d", k, s[k]))
			}
			fmt.Printf("%-22s %s\n", p, strings.Join(parts, " "))
		}
	}
	if *examples != "" {
		b, _ := json.MarshalIndent(ex, "", " ")
		check(os.WriteFile(*examples, b, 0o644))
	}
}

func outBytes(r map[string]any) []byte {
	o, _ := r["output"].(map[string]any)
	s, _ := o["data"].(string)
	b, _ := base64.StdEncoding.DecodeString(s)
	return b
}

func firstDiff(a, b []byte) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}

func window(b []byte, at int) string {
	return fmt.Sprintf("%q", b[max(0, at-60):min(len(b), at+60)])
}

// canon drops "orgRow": null so absent and null compare equal.
func canon(v any) string {
	var strip func(any) any
	strip = func(v any) any {
		switch x := v.(type) {
		case map[string]any:
			m := map[string]any{}
			for k, e := range x {
				if k == "orgRow" && e == nil {
					continue
				}
				m[k] = strip(e)
			}
			return m
		case []any:
			for i := range x {
				x[i] = strip(x[i])
			}
			return x
		}
		return v
	}
	b, _ := json.Marshal(strip(v))
	return string(b)
}

func hasTimeConstant(v map[string]any) bool {
	for _, d := range v["datasets"].([]any) {
		for _, c := range d.(map[string]any)["constColumns"].([]any) {
			if c.(map[string]any)["type"] == "TIME" {
				return true
			}
		}
	}
	return false
}

func (g gen) pick(xs ...string) string { return xs[g.r.Intn(len(xs))] }

func (g gen) lexical(t string) string {
	switch t {
	case "STRING":
		if !g.features["str"] {
			n := []int{1, 3, 8}[g.r.Intn(3)]
			var sb strings.Builder
			for i := 0; i < n; i++ {
				sb.WriteByte("abcXYZ019"[g.r.Intn(9)])
			}
			return sb.String()
		}
		n := []int{0, 1, 3, 8}[g.r.Intn(4)]
		var sb strings.Builder
		for i := 0; i < n; i++ {
			sb.WriteString(chars[g.r.Intn(len(chars))])
		}
		s := sb.String()
		if g.r.Float64() < 0.2 {
			s = " " + s + " "
		}
		return s
	case "INT", "LONG":
		return g.pick("0", "1", "-7", "123456", "2147483647")
	case "BOOLEAN":
		return g.pick("0", "1")
	case "FLOAT", "DECIMAL", "BIGDECIMAL":
		return g.pick("0", "1.5", "-2.25", "123456.789", "10")
	case "DATE":
		return g.pick("20261116", "19991231")
	case "DATETIME":
		return g.pick("20261116123456789", "20261116123456000")
	case "TIME":
		return g.pick("123456789", "000000000")
	case "BLOB":
		b := make([]byte, []int{0, 1, 4}[g.r.Intn(3)])
		g.r.Read(b)
		return base64.StdEncoding.EncodeToString(b)
	}
	panic(t)
}

// cell avoids "empty" on non-string types: the adapter would hand the driver a
// Java "" string, which no application does, so the result says nothing about
// the wire format.
func (g gen) cell(t string, allowMissing bool) map[string]any {
	states := []string{"value"}
	if g.features["states"] {
		states = []string{"value", "value", "value", "value", "null"}
		if t == "STRING" || t == "BLOB" {
			states = append(states, "empty")
		}
		if allowMissing {
			states = append(states, "missing")
		}
	}
	switch st := states[g.r.Intn(len(states))]; st {
	case "value":
		return map[string]any{"state": "value", "lexical": g.lexical(t)}
	case "empty":
		return map[string]any{"state": "empty", "lexical": ""}
	default:
		return map[string]any{"state": st}
	}
}

func (g gen) value() map[string]any {
	params := []any{}
	if g.features["params"] {
		for i := 0; i < g.r.Intn(3); i++ {
			t := g.types[g.r.Intn(len(g.types))]
			p := map[string]any{"id": fmt.Sprintf("p%d", i), "type": t}
			for k, v := range g.cell(t, false) {
				p[k] = v
			}
			params = append(params, p)
		}
	}
	datasets := []any{}
	for d := 0; d < []int{0, 1, 1, 2}[g.r.Intn(4)]; d++ {
		cols := []map[string]any{}
		for i := 0; i < 1+g.r.Intn(3); i++ {
			cols = append(cols, map[string]any{"id": fmt.Sprintf("c%d", i), "type": g.types[g.r.Intn(len(g.types))], "index": i})
		}
		consts := []any{}
		if g.features["consts"] && g.r.Intn(3) == 0 {
			t := g.types[g.r.Intn(len(g.types))]
			consts = append(consts, map[string]any{"id": "k0", "type": t, "value": map[string]any{"state": "value", "lexical": g.lexical(t)}})
		}
		rows := []any{}
		for r := 0; r < g.r.Intn(4); r++ {
			rt := "N"
			if g.features["rows"] {
				rt = g.pick("N", "N", "I", "U", "D")
			}
			vals := map[string]any{}
			for _, c := range cols {
				if cell := g.cell(c["type"].(string), true); cell["state"] != "missing" {
					vals[c["id"].(string)] = cell
				}
			}
			row := map[string]any{"type": rt, "values": vals}
			if rt == "U" && g.features["orgrow"] && g.r.Float64() < 0.6 {
				org := map[string]any{}
				for _, c := range cols {
					org[c["id"].(string)] = map[string]any{"state": "value", "lexical": g.lexical(c["type"].(string))}
				}
				row["orgRow"] = map[string]any{"type": "O", "values": org}
			}
			rows = append(rows, row)
		}
		colsAny := make([]any, len(cols))
		for i, c := range cols {
			colsAny[i] = c
		}
		ds := map[string]any{"id": fmt.Sprintf("d%d", d), "columns": colsAny, "constColumns": consts, "rows": rows}
		if g.features["savetype"] {
			ds["saveType"] = 1 + g.r.Intn(5)
		}
		datasets = append(datasets, ds)
	}
	return map[string]any{"parameters": params, "datasets": datasets}
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}
