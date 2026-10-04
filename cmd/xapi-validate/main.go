package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type vector struct {
	ID        string `json:"id"`
	Suite     string `json:"suite,omitempty"`
	Operation string `json:"operation"`
	Profile   string `json:"profile"`
	Input     *struct {
		Encoding string `json:"encoding"`
		Data     string `json:"data"`
	} `json:"input"`
	Value  json.RawMessage `json:"value"`
	Expect struct {
		Kind    string          `json:"kind"`
		Value   json.RawMessage `json:"value"`
		Output  json.RawMessage `json:"output"`
		Options map[string]any  `json:"options"`
		Error   struct {
			Class string `json:"class"`
			Path  string `json:"path"`
		} `json:"error"`
	} `json:"expect"`
	Source struct {
		Repository string `json:"repository"`
		Commit     string `json:"commit"`
		Path       string `json:"path"`
		License    string `json:"license"`
	} `json:"source"`
}

var idRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func main() {
	dir := flag.String("vectors", "vectors", "vector directory")
	schemaDir := flag.String("schemas", "protocol", "schema directory")
	checkSources := flag.Bool("check-sources", true, "verify source paths in pinned checkouts")
	suite := flag.String("suite", "all", "suite filter: all, vendor, security, limitation")
	flag.Parse()
	if err := validateSchemas(*schemaDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var files []string
	_ = filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() && filepath.Ext(path) == ".json" {
			files = append(files, path)
		}
		return err
	})
	sort.Strings(files)
	errs := []string{}
	validated := 0
	for _, path := range files {
		b, e := os.ReadFile(path)
		if e != nil {
			errs = append(errs, path+": "+e.Error())
			continue
		}
		var v vector
		if e = json.Unmarshal(b, &v); e != nil {
			errs = append(errs, path+": invalid JSON: "+e.Error())
			continue
		}
		vectorSuite := v.Suite
		if vectorSuite == "" {
			vectorSuite = "vendor"
		}
		inSuite := *suite == "all" || vectorSuite == *suite || (*suite == "vendor" && vectorSuite == "limitation")
		if !inSuite {
			continue
		}
		validated++
		for _, msg := range validate(v, *checkSources) {
			errs = append(errs, path+": "+msg)
		}
	}
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
		os.Exit(1)
	}
	fmt.Printf("validated %d vectors\n", validated)
}

func validateSchemas(dir string) error {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".json" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var doc map[string]any
		if err = json.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("%s: invalid schema JSON: %w", path, err)
		}
		if doc["$schema"] == nil || doc["type"] == nil {
			return fmt.Errorf("%s: schema requires $schema and type", path)
		}
	}
	return nil
}
func validate(v vector, checkSources bool) []string {
	e := []string{}
	if v.Suite != "" && v.Suite != "vendor" && v.Suite != "security" && v.Suite != "limitation" {
		e = append(e, "suite must be vendor, security, or limitation")
	}
	if !idRE.MatchString(v.ID) {
		e = append(e, "id must match [A-Za-z0-9][A-Za-z0-9._-]*")
	}
	if v.Operation != "decode" && v.Operation != "encode" && v.Operation != "roundtrip" {
		e = append(e, "unsupported operation")
	}
	if v.Profile == "" {
		e = append(e, "profile is required")
	}
	if v.Source.Repository == "" || v.Source.Commit == "" || v.Source.Path == "" || v.Source.License == "" {
		e = append(e, "source.repository, source.commit, source.path, and source.license are required")
	} else if checkSources && !retiredSource(v.Source.Repository) {
		if root := sourceRoot(v.Source.Repository); root != "" {
			if _, err := os.Stat(filepath.Join(root, v.Source.Path)); err != nil {
				e = append(e, "source.path does not exist in pinned checkout: "+v.Source.Path)
			}
		} else if _, err := os.Stat(v.Source.Path); err != nil {
			e = append(e, "source.path does not exist: "+v.Source.Path)
		}
	}
	if v.Expect.Kind != "canonical" && v.Expect.Kind != "wire" && v.Expect.Kind != "error" {
		e = append(e, "expect.kind must be canonical, wire, or error")
	}
	if v.Operation == "decode" || v.Operation == "roundtrip" {
		if v.Input == nil {
			e = append(e, "input is required")
		} else {
			if v.Input.Encoding != "base64" {
				e = append(e, "input.encoding must be base64")
			}
			if v.Expect.Kind != "error" {
				data := strings.Map(func(r rune) rune {
					if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
						return -1
					}
					return r
				}, v.Input.Data)
				if _, x := base64.StdEncoding.DecodeString(data); x != nil {
					e = append(e, "input.data is not valid base64")
				}
			}
		}
	}
	if v.Operation == "encode" && len(v.Value) == 0 {
		e = append(e, "value is required")
	}
	if v.Expect.Kind == "canonical" && len(v.Expect.Value) == 0 {
		e = append(e, "expect.value is required")
	}
	if v.Expect.Kind == "wire" && len(v.Expect.Output) == 0 {
		e = append(e, "expect.output is required")
	}
	if v.Expect.Kind == "error" && v.Expect.Error.Class == "" {
		e = append(e, "expect.error.class is required")
	}
	return e
}
func sourceRoot(repo string) string {
	switch repo {
	case "Clickin/xapi-js", "xapi-js":
		return "sources/xapi-js"
	case "Clickin/xplatform-xml", "xplatform-xml":
		return "sources/xplatform-xml"
	default:
		return ""
	}
}

func retiredSource(repo string) bool {
	return repo == "Clickin/xapi" || repo == "xapi"
}
