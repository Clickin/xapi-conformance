package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type vector struct {
	ID        string         `json:"id"`
	Operation string         `json:"operation"`
	Profile   string         `json:"profile"`
	Input     map[string]any `json:"input,omitempty"`
	Value     map[string]any `json:"value,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
	Expect    struct {
		Kind   string         `json:"kind"`
		Value  map[string]any `json:"value"`
		Output map[string]any `json:"output"`
		Error  struct {
			Class string `json:"class"`
			Path  string `json:"path"`
		} `json:"error"`
	} `json:"expect"`
	Required bool `json:"required"`
}
type capabilities struct {
	ProtocolVersion string `json:"protocolVersion"`
	Profiles        []struct {
		Name       string   `json:"name"`
		Operations []string `json:"operations"`
	} `json:"profiles"`
}

type result struct {
	ID       string `json:"id"`
	Pass     bool   `json:"pass"`
	Error    string `json:"error,omitempty"`
	Request  any    `json:"request,omitempty"`
	Expected any    `json:"expected,omitempty"`
	Actual   any    `json:"actual,omitempty"`
	Diff     string `json:"diff,omitempty"`
}
type junitSuite struct {
	XMLName  xml.Name    `xml:"testsuite"`
	Name     string      `xml:"name,attr"`
	Tests    int         `xml:"tests,attr"`
	Failures int         `xml:"failures,attr"`
	Cases    []junitCase `xml:"testcase"`
}
type junitCase struct {
	Name    string        `xml:"name,attr"`
	Failure *junitFailure `xml:"failure,omitempty"`
}
type junitFailure struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

func main() {
	base := flag.String("url", "http://127.0.0.1:8787", "adapter URL")
	dir := flag.String("vectors", "vectors", "vector directory")
	filter := flag.String("filter", "", "substring filter")
	profile := flag.String("profile", "", "profile filter")
	operation := flag.String("operation", "", "operation filter")
	parallel := flag.Int("parallel", 1, "concurrent requests")
	command := flag.String("command", "", "adapter subprocess, e.g. 'go run ./cmd/xapi-reference stdio'")
	timeout := flag.Duration("timeout", 10*time.Second, "request timeout")
	jsonOut := flag.String("json", "", "JSON result path")
	junit := flag.String("junit", "", "JUnit XML result path")
	flag.Parse()
	vs, err := load(*dir, *filter, *profile, *operation)
	if err != nil {
		fatal(err)
	}
	if *command != "" {
		rs := runCommand(*command, vs, *timeout)
		finish(rs, *jsonOut, *junit)
		if anyFailed(rs) {
			os.Exit(1)
		}
		return
	}
	client := &http.Client{Timeout: *timeout}
	vs, err = checkCapabilities(client, *base, vs)
	if err != nil {
		fatal(err)
	}
	results := make([]result, len(vs))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for n := 0; n < *parallel; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = run(client, *base, vs[i])
			}
		}()
	}
	for i := range vs {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })
	finish(results, *jsonOut, *junit)
	if anyFailed(results) {
		os.Exit(1)
	}
}

func finish(results []result, jsonOut, junit string) {
	for _, r := range results {
		if !r.Pass {
			fmt.Fprintf(os.Stderr, "FAIL %s: %s\n", r.ID, r.Error)
		}
	}
	if jsonOut != "" {
		write(jsonOut, results)
	}
	if junit != "" {
		writeJunit(junit, results)
	}
}
func anyFailed(rs []result) bool {
	for _, r := range rs {
		if !r.Pass {
			return true
		}
	}
	return false
}
func load(dir, filter, profile, operation string) ([]vector, error) {
	var vs []vector
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".json" || !strings.Contains(path, "vectors/") {
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		var v vector
		if e = json.Unmarshal(b, &v); e != nil {
			return e
		}
		if (filter == "" || strings.Contains(v.ID, filter)) && (profile == "" || v.Profile == profile) && (operation == "" || v.Operation == operation) {
			vs = append(vs, v)
		}
		return nil
	})
	sort.Slice(vs, func(i, j int) bool { return vs[i].ID < vs[j].ID })
	return vs, err
}
func checkCapabilities(c *http.Client, base string, vs []vector) ([]vector, error) {
	r, e := c.Get(strings.TrimRight(base, "/") + "/capabilities")
	if e != nil {
		return nil, e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, fmt.Errorf("capabilities returned %s", r.Status)
	}
	var cap capabilities
	if err := json.NewDecoder(r.Body).Decode(&cap); err != nil {
		return nil, fmt.Errorf("invalid capabilities: %w", err)
	}
	return selectVectors(cap, vs)
}
func selectVectors(cap capabilities, vs []vector) ([]vector, error) {
	if cap.ProtocolVersion != "1.0" {
		return nil, fmt.Errorf("unsupported protocol version %q", cap.ProtocolVersion)
	}
	profiles := map[string]map[string]bool{}
	for _, p := range cap.Profiles {
		profiles[p.Name] = map[string]bool{}
		for _, op := range p.Operations {
			profiles[p.Name][op] = true
		}
	}
	selected := make([]vector, 0, len(vs))
	for _, v := range vs {
		operations, ok := profiles[v.Profile]
		if !ok {
			if v.Required {
				return nil, fmt.Errorf("required profile %q is not advertised", v.Profile)
			}
			continue
		}
		if !operations[v.Operation] {
			if v.Required {
				return nil, fmt.Errorf("required operation %q for profile %q is not advertised", v.Operation, v.Profile)
			}
			continue
		}
		selected = append(selected, v)
	}
	return selected, nil
}
func run(c *http.Client, base string, v vector) result {
	body := requestBody(v)
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, strings.TrimRight(base, "/")+"/"+v.Operation, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, e := c.Do(req)
	if e != nil {
		return result{ID: v.ID, Error: e.Error(), Request: body, Expected: expected(v)}
	}
	defer resp.Body.Close()
	var actual map[string]any
	_ = json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&actual)
	return evaluate(v, actual)
}
func requestBody(v vector) map[string]any {
	body := map[string]any{"case": v.ID, "operation": v.Operation, "profile": v.Profile}
	if v.Options != nil {
		body["options"] = v.Options
	}
	if v.Input != nil {
		body["input"] = v.Input
	}
	if v.Value != nil {
		body["value"] = v.Value
	}
	return body
}
func evaluate(v vector, actual map[string]any) result {
	r := result{ID: v.ID, Request: requestBody(v), Expected: expected(v), Actual: actual}
	if v.Expect.Kind == "error" {
		ok := actual["error"] != nil
		if ok {
			er, _ := actual["error"].(map[string]any)
			ok = er["class"] == v.Expect.Error.Class && (v.Expect.Error.Path == "" || er["path"] == v.Expect.Error.Path)
		}
		err := ""
		if !ok {
			err = fmt.Sprintf("expected error %s", v.Expect.Error.Class)
		}
		r.Pass, r.Error = ok, err
		if !ok {
			r.Diff = diff(r.Expected, actual)
		}
		return r
	}
	if v.Expect.Kind == "wire" {
		output, ok := actual["output"].(map[string]any)
		pass := ok && jsonEqual(output, v.Expect.Output)
		if pass && v.Expect.Value != nil {
			value, valueOK := actual["value"].(map[string]any)
			pass = valueOK && jsonEqual(value, v.Expect.Value)
		}
		r.Pass = pass
		if !pass {
			r.Error = "wire output differs"
			r.Diff = diff(r.Expected, actual)
		}
		return r
	}
	av, ok := actual["value"].(map[string]any)
	if !ok {
		r.Error = "missing response value"
		return r
	}
	expected := v.Expect.Value
	if expected == nil {
		expected = v.Value
	}
	pass := jsonEqual(av, expected)
	err := ""
	if !pass {
		err = "canonical value differs"
	}
	r.Pass, r.Error = pass, err
	if !pass {
		r.Diff = diff(r.Expected, av)
	}
	return r
}
func expected(v vector) any {
	if v.Expect.Kind == "error" {
		return map[string]any{"error": v.Expect.Error}
	}
	if v.Expect.Kind == "wire" {
		expected := map[string]any{"output": v.Expect.Output}
		if v.Expect.Value != nil {
			expected["value"] = v.Expect.Value
		}
		return expected
	}
	if v.Expect.Value != nil {
		return v.Expect.Value
	}
	return v.Value
}
func diff(expected, actual any) string {
	a, _ := json.MarshalIndent(expected, "", "  ")
	b, _ := json.MarshalIndent(actual, "", "  ")
	return "expected:\n" + string(a) + "\nactual:\n" + string(b)
}

func runCommand(command string, vs []vector, timeout time.Duration) []result {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		fatal(fmt.Errorf("empty command"))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	in, err := cmd.StdinPipe()
	if err != nil {
		fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		fatal(err)
	}
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 64<<10), 10<<20)
	lines := make(chan []byte, 1)
	go func() {
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			lines <- line
		}
		lines <- nil
	}()
	enc := json.NewEncoder(in)
	if err := enc.Encode(map[string]any{"operation": "capabilities"}); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return []result{{ID: "capabilities", Error: err.Error()}}
	}
	timer := time.NewTimer(timeout)
	var capabilityLine []byte
	select {
	case capabilityLine = <-lines:
	case <-timer.C:
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return []result{{ID: "capabilities", Error: "timeout"}}
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	if capabilityLine == nil {
		_ = cmd.Wait()
		return []result{{ID: "capabilities", Error: "stdio adapter exited before capability response"}}
	}
	var cap capabilities
	if err := json.Unmarshal(capabilityLine, &cap); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return []result{{ID: "capabilities", Error: fmt.Sprintf("invalid capabilities: %v", err)}}
	}
	vs, err = selectVectors(cap, vs)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return []result{{ID: "capabilities", Error: err.Error()}}
	}
	rs := make([]result, 0, len(vs))
	for i, v := range vs {
		if err := enc.Encode(requestBody(v)); err != nil {
			rs = append(rs, failedResult(v, err.Error()))
			continue
		}
		timer := time.NewTimer(timeout)
		var line []byte
		select {
		case line = <-lines:
		case <-timer.C:
			_ = cmd.Process.Kill()
			r := failedResult(v, "timeout")
			rs = append(rs, r)
			for _, remaining := range vs[i+1:] {
				rs = append(rs, failedResult(remaining, "process terminated after timeout"))
			}
			_ = in.Close()
			_ = cmd.Wait()
			sort.Slice(rs, func(i, j int) bool { return rs[i].ID < rs[j].ID })
			return rs
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		if line == nil {
			rs = append(rs, failedResult(v, "stdio adapter exited before response"))
			break
		}
		var actual map[string]any
		if err := json.Unmarshal(line, &actual); err != nil {
			rs = append(rs, failedResult(v, err.Error()))
			continue
		}
		rs = append(rs, evaluate(v, actual))
	}
	_ = in.Close()
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	waitTimer := time.NewTimer(timeout)
	select {
	case err := <-wait:
		if err != nil && len(rs) < len(vs) {
			rs = append(rs, result{ID: "process", Error: err.Error()})
		}
	case <-waitTimer.C:
		_ = cmd.Process.Kill()
		<-wait
		rs = append(rs, result{ID: "process", Error: "timeout waiting for process exit"})
	}
	if !waitTimer.Stop() {
		select {
		case <-waitTimer.C:
		default:
		}
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].ID < rs[j].ID })
	return rs
}
func failedResult(v vector, message string) result {
	return result{ID: v.ID, Error: message, Request: requestBody(v), Expected: expected(v)}
}
func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	var ax, bx any
	if json.Unmarshal(x, &ax) != nil || json.Unmarshal(y, &bx) != nil {
		return false
	}
	return fmt.Sprintf("%v", ax) == fmt.Sprintf("%v", bx)
}
func write(path string, v any) {
	b, e := json.MarshalIndent(v, "", "  ")
	if e == nil {
		e = os.WriteFile(path, append(b, '\n'), 0644)
	}
	if e != nil {
		fatal(e)
	}
}
func writeJunit(path string, rs []result) {
	s := junitSuite{Name: "xapi-conformance", Tests: len(rs)}
	for _, r := range rs {
		c := junitCase{Name: r.ID}
		if !r.Pass {
			s.Failures++
			c.Failure = &junitFailure{Message: r.Error, Text: r.Diff}
		}
		s.Cases = append(s.Cases, c)
	}
	b, e := xml.MarshalIndent(s, "", "  ")
	if e == nil {
		e = os.WriteFile(path, append(b, '\n'), 0644)
	}
	if e != nil {
		fatal(e)
	}
}
func fatal(e error) { fmt.Fprintln(os.Stderr, e); os.Exit(2) }
