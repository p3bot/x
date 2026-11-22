package output_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/p3bot/x/clikit/fault"
	"github.com/p3bot/x/clikit/mode"
	"github.com/p3bot/x/clikit/output"
)

type item struct {
	ID     string
	Title  string
	Status string
	Owner  *string
}

var itemFields = []string{"id", "title", "status", "owner"}

func itemValues(it item) map[string]any {
	v := map[string]any{"id": it.ID, "title": it.Title, "status": it.Status}
	if it.Owner != nil {
		v["owner"] = *it.Owner
	}
	return v
}

func ptr(s string) *string { return &s }

var items = []item{
	{ID: "a1", Title: "First", Status: "open", Owner: ptr("ann")},
	{ID: "b22", Title: "Second", Status: "closed"},
	{ID: "c3", Title: "Third\tpart\r\nline\nmore", Status: "open"},
}

type harness struct {
	out            *output.Output
	stdout, stderr *bytes.Buffer
}

// modesFor builds modes the only way a CLI can, through mode.Detect.
func modesFor(t *testing.T, flags mode.Flags, env mode.Env) mode.Modes {
	t.Helper()
	m, err := mode.Detect("test", flags, env)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func newHarness(m mode.Modes) harness {
	var stdout, stderr bytes.Buffer
	return harness{out: output.New(&stdout, &stderr, m), stdout: &stdout, stderr: &stderr}
}

func mustSelect(t *testing.T, requested ...string) output.Selection {
	t.Helper()
	sel, err := output.SelectFields(requested, itemFields)
	if err != nil {
		t.Fatalf("SelectFields(%v): %v", requested, err)
	}
	return sel
}

func TestSelectFields(t *testing.T) {
	tests := []struct {
		name      string
		requested []string
		want      []string
		explicit  bool
		wantMsg   string
	}{
		{name: "default is the known set", want: itemFields},
		{name: "named fields keep their order", requested: []string{"status", "id"}, want: []string{"status", "id"}, explicit: true},
		{name: "naming the default set is still explicit", requested: itemFields, want: itemFields, explicit: true},
		{name: "a repeated name keeps its first position", requested: []string{"title", "id", "title", " id"}, want: []string{"title", "id"}, explicit: true},
		{name: "spaces are trimmed", requested: []string{" id", "title "}, want: []string{"id", "title"}, explicit: true},
		{name: "unknown name", requested: []string{"id", "colour"}, wantMsg: `unknown field: "colour"`},
		{name: "several unknown names", requested: []string{"x", "id", "y"}, wantMsg: `unknown fields: "x", "y"`},
		{name: "a repeated unknown name is reported once", requested: []string{"x", "x"}, wantMsg: `unknown field: "x"`},
		{name: "empty name", requested: []string{""}, wantMsg: `unknown field: ""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel, err := output.SelectFields(tt.requested, itemFields)
			if tt.wantMsg != "" {
				p := fault.Payload(err)
				if p == nil || p.Code != "INVALID_FIELDS" || p.Message != tt.wantMsg ||
					!reflect.DeepEqual(p.ValidValues, itemFields) || p.Fix == "" || p.Exit() != fault.ExitUsage {
					t.Errorf("payload = %+v", p)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(sel.Names(), tt.want) || sel.Explicit() != tt.explicit {
				t.Errorf("Names %v Explicit %v, want %v %v", sel.Names(), sel.Explicit(), tt.want, tt.explicit)
			}
		})
	}
}

func TestDataText(t *testing.T) {
	tests := []struct {
		name       string
		v          func(t *testing.T) any
		quiet      bool
		wantStdout string
		wantStderr string
	}{
		{
			name:       "nil writes nothing",
			v:          func(*testing.T) any { return nil },
			wantStdout: "",
		},
		{
			name:       "lines",
			v:          func(*testing.T) any { return []string{"one", "two"} },
			wantStdout: "one\ntwo\n",
		},
		{
			name: "table aligns under a header and leaves empty cells blank",
			v: func(t *testing.T) any {
				return output.NewTable(mustSelect(t), items[:2], itemValues)
			},
			wantStdout: "" +
				"id   title   status  owner\n" +
				"a1   First   open    ann\n" +
				"b22  Second  closed\n",
		},
		{
			name: "table with named fields is headerless TSV in requested order",
			v: func(t *testing.T) any {
				return output.NewTable(mustSelect(t, "status", "title"), items, itemValues)
			},
			wantStdout: "open\tFirst\nclosed\tSecond\nopen\tThird part line more\n",
		},
		{
			name: "table naming the default set is still TSV",
			v: func(t *testing.T) any {
				return output.NewTable(mustSelect(t, itemFields...), items[:1], itemValues)
			},
			wantStdout: "a1\tFirst\topen\tann\n",
		},
		{
			name: "record aligns label and value",
			v: func(t *testing.T) any {
				return output.NewRecord(mustSelect(t), items[1], itemValues)
			},
			wantStdout: "id:     b22\ntitle:  Second\nstatus: closed\nowner:\n",
		},
		{
			name: "record with named fields is one TSV line with breaks and tabs as spaces",
			v: func(t *testing.T) any {
				return output.NewRecord(mustSelect(t, "title", "id"), items[2], itemValues)
			},
			wantStdout: "Third part line more\tc3\n",
		},
		{
			name: "empty page with named fields writes only the indicator, to stderr",
			v: func(t *testing.T) any {
				return output.Page{Table: output.NewTable(mustSelect(t, "id"), nil, itemValues), Start: 1, Total: 0}
			},
			wantStdout: "",
			wantStderr: "Showing 0 results\n",
		},
		{
			name: "aligned page indicator on stdout with total and cursor",
			v: func(t *testing.T) any {
				sel, err := output.SelectFields(nil, []string{"id"})
				if err != nil {
					t.Fatal(err)
				}
				return output.Page{Table: output.NewTable(sel, items[:2], itemValues), Start: 26, Total: 3352, Next: "eyJvIjo1MH0"}
			},
			wantStdout: "id\na1\nb22\nShowing 26-27 of 3352 results  (--next eyJvIjo1MH0)\n",
		},
		{
			name: "aligned page indicator with total left unknown and no cursor",
			v: func(t *testing.T) any {
				sel, err := output.SelectFields(nil, []string{"id"})
				if err != nil {
					t.Fatal(err)
				}
				return output.Page{Table: output.NewTable(sel, items[:2], itemValues), Start: 1}
			},
			wantStdout: "id\na1\nb22\nShowing 1-2 results\n",
		},
		{
			name: "negative total also reads as unknown",
			v: func(t *testing.T) any {
				return output.Page{Table: output.NewTable(mustSelect(t, "id"), items[:1], itemValues), Start: 1, Total: -1}
			},
			wantStdout: "a1\n",
			wantStderr: "Showing 1-1 results\n",
		},
		{
			name: "page with named fields puts TSV on stdout and the indicator on stderr even under quiet",
			v: func(t *testing.T) any {
				return output.Page{Table: output.NewTable(mustSelect(t, "status", "id"), items, itemValues), Start: 1, Total: 12, Next: "tok"}
			},
			quiet:      true,
			wantStdout: "open\ta1\nclosed\tb22\nopen\tc3\n",
			wantStderr: "Showing 1-3 of 12 results  (--next tok)\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(modesFor(t, mode.Flags{Quiet: tt.quiet}, mode.Env{}))
			if err := h.out.Data(tt.v(t)); err != nil {
				t.Fatal(err)
			}
			if got := h.stdout.String(); got != tt.wantStdout {
				t.Errorf("stdout\n got: %q\nwant: %q", got, tt.wantStdout)
			}
			if got := h.stderr.String(); got != tt.wantStderr {
				t.Errorf("stderr\n got: %q\nwant: %q", got, tt.wantStderr)
			}
		})
	}
}

func TestDataTextFailures(t *testing.T) {
	h := newHarness(mode.Modes{})
	if err := h.out.Data(struct{}{}); err == nil || !strings.Contains(err.Error(), "TextRenderer") {
		t.Errorf("unsupported value: err = %v", err)
	}
	failing := output.New(failWriter{}, io.Discard, mode.Modes{})
	sel, err := output.SelectFields(nil, itemFields)
	if err != nil {
		t.Fatal(err)
	}
	values := []any{
		[]string{"x"},
		output.NewTable(sel, items, itemValues),
		output.NewRecord(sel, items[0], itemValues),
		output.Page{Table: output.NewTable(sel, items, itemValues), Start: 1, Total: 3},
	}
	for _, v := range values {
		if err := failing.Data(v); !errors.Is(err, errWrite) {
			t.Errorf("Data(%T) on a failing stdout = %v, want the write error", v, err)
		}
	}
	named := output.Page{Table: output.NewTable(mustSelect(t, "id"), items, itemValues), Start: 1, Next: "tok"}
	if err := output.New(io.Discard, failWriter{}, mode.Modes{}).Data(named); !errors.Is(err, errWrite) {
		t.Errorf("page indicator on a failing stderr = %v, want the write error", err)
	}
	jsonFailing := output.New(failWriter{}, io.Discard, modesFor(t, mode.Flags{JSON: true}, mode.Env{}))
	if err := jsonFailing.Data([]string{"x"}); !errors.Is(err, errWrite) {
		t.Errorf("JSON Data on a failing stdout = %v", err)
	}
}

func TestSelectFieldsNoKnownFields(t *testing.T) {
	for _, requested := range [][]string{nil, {"id"}} {
		_, err := output.SelectFields(requested, nil)
		if err == nil || !strings.Contains(err.Error(), "declares no fields") {
			t.Errorf("SelectFields(%v, nil) err = %v, want the no-fields error", requested, err)
		}
		if p := fault.Payload(err); p.Code != fault.CodeUnknown {
			t.Errorf("SelectFields(%v, nil) code = %s, want %s", requested, p.Code, fault.CodeUnknown)
		}
	}
}

func TestDataEmptySelection(t *testing.T) {
	rows := []map[string]any{{"id": "x"}}
	values := []any{
		output.Table{Rows: rows},
		output.Record{Values: rows[0]},
		output.Page{Table: output.Table{Rows: rows}, Start: 1, Total: 1},
		[]any{output.Table{Rows: rows}},
	}
	for _, jsonOn := range []bool{false, true} {
		for _, v := range values {
			if _, nested := v.([]any); nested && !jsonOn {
				continue // text mode has no nesting
			}
			h := newHarness(modesFor(t, mode.Flags{JSON: jsonOn}, mode.Env{}))
			err := h.out.Data(v)
			if err == nil || !strings.Contains(err.Error(), "field selection is empty") {
				t.Errorf("json=%v Data(%T) err = %v, want the empty-selection error", jsonOn, v, err)
			}
			if h.stdout.Len() != 0 {
				t.Errorf("json=%v Data(%T) wrote %q", jsonOn, v, h.stdout.String())
			}
		}
	}
}

func TestDataPageWithoutStart(t *testing.T) {
	table := output.NewTable(mustSelect(t), items[:2], itemValues)
	for _, jsonOn := range []bool{false, true} {
		h := newHarness(modesFor(t, mode.Flags{JSON: jsonOn}, mode.Env{}))
		err := h.out.Data(output.Page{Table: table, Total: 2})
		if err == nil || !strings.Contains(err.Error(), "page Start must be 1 or more, got 0") {
			t.Errorf("json=%v err = %v, want the missing-Start error", jsonOn, err)
		}
		if h.stdout.Len() != 0 || h.stderr.Len() != 0 {
			t.Errorf("json=%v wrote stdout %q stderr %q", jsonOn, h.stdout.String(), h.stderr.String())
		}
	}
	h := newHarness(mode.Modes{})
	empty := output.Page{Table: output.NewTable(mustSelect(t), []item(nil), itemValues)}
	if err := h.out.Data(empty); err != nil {
		t.Errorf("empty page without Start: %v", err)
	}
}

var errWrite = errors.New("write failed")

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errWrite }

func TestWarningsNoticesHints(t *testing.T) {
	w := fault.WarningPayload{Code: "FLAG_DEPRECATED", Message: "--max is deprecated", Fix: "use --limit"}
	tests := []struct {
		name       string
		modes      mode.Modes
		wantStderr string
	}{
		{
			name: "text shows all",
			wantStderr: "Warning: --max is deprecated\n  Fix: use --limit\n" +
				"Warning: slow\n  Retry after: 5s\n" +
				"heads up\nHint: try --help\n",
		},
		{name: "quiet drops all", modes: modesFor(t, mode.Flags{Quiet: true}, mode.Env{})},
		{
			name:       "JSON keeps stderr warnings and drops notices and hints",
			modes:      modesFor(t, mode.Flags{JSON: true}, mode.Env{}),
			wantStderr: "Warning: --max is deprecated\n  Fix: use --limit\nWarning: slow\n  Retry after: 5s\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(tt.modes)
			h.out.Warning(w)
			h.out.Warning(fault.WarningPayload{Message: "slow", RetryAfter: "5s"})
			h.out.Notice("heads up")
			h.out.Hint("try --help")
			if got := h.stderr.String(); got != tt.wantStderr {
				t.Errorf("stderr\n got: %q\nwant: %q", got, tt.wantStderr)
			}
			if h.stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", h.stdout.String())
			}
		})
	}

	h := newHarness(mode.Modes{})
	if err := h.out.Data([]string{"done"}, w); err != nil {
		t.Fatal(err)
	}
	if h.stdout.String() != "done\n" || !strings.HasPrefix(h.stderr.String(), "Warning: --max is deprecated\n") {
		t.Errorf("Data warnings: stdout %q stderr %q", h.stdout.String(), h.stderr.String())
	}
}

type domainErr struct{}

func (domainErr) Error() string { return "branch is locked" }
func (domainErr) Code() string  { return "BRANCH_LOCKED" }
func (domainErr) Fix() string   { return "demo unlock main" }
func (domainErr) ExitCode() int { return fault.ExitConflict }

func TestErrText(t *testing.T) {
	invalidFields := func(*testing.T) error {
		_, err := output.SelectFields([]string{"colour"}, itemFields)
		return err
	}
	tests := []struct {
		name       string
		err        func(t *testing.T) error
		wantStderr string
		wantExit   int
	}{
		{
			name: "invalid fields lists the accepted set",
			err:  invalidFields,
			wantStderr: "Error: unknown field: \"colour\"\n" +
				"  Code:     INVALID_FIELDS\n" +
				"  Accepted: id, title, status, owner\n" +
				"  Fix:      rerun with --fields set to a comma-separated list of the accepted fields\n",
			wantExit: fault.ExitUsage,
		},
		{
			name: "domain code with its own exit code, wrapped for context",
			err:  func(*testing.T) error { return fmt.Errorf("pushing main: %w", domainErr{}) },
			wantStderr: "Error: pushing main: branch is locked\n" +
				"  Code: BRANCH_LOCKED\n" +
				"  Fix:  demo unlock main\n",
			wantExit: fault.ExitConflict,
		},
		{
			name: "retry hint",
			err: func(*testing.T) error {
				return fault.New("RATE_LIMITED", "API rate limit exceeded", "wait 30s and retry").WithRetryAfter("30s")
			},
			wantStderr: "Error: API rate limit exceeded\n" +
				"  Code:        RATE_LIMITED\n" +
				"  Retry after: 30s\n" +
				"  Fix:         wait 30s and retry\n",
			wantExit: fault.ExitTransient,
		},
		{
			name:       "unclassified error without a fix",
			err:        func(*testing.T) error { return errors.New("disk on fire") },
			wantStderr: "Error: disk on fire\n  Code: UNKNOWN\n",
			wantExit:   fault.ExitFailure,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, quiet := range []bool{false, true} {
				h := newHarness(modesFor(t, mode.Flags{Quiet: quiet}, mode.Env{}))
				src := tt.err(t)
				got := h.out.Err(src)
				if s := h.stderr.String(); s != tt.wantStderr {
					t.Errorf("quiet=%v stderr\n got: %q\nwant: %q", quiet, s, tt.wantStderr)
				}
				if h.stdout.Len() != 0 {
					t.Errorf("stdout = %q, want empty", h.stdout.String())
				}
				if code := fault.ExitCode(got); code != tt.wantExit {
					t.Errorf("ExitCode = %d, want %d", code, tt.wantExit)
				}
				if !fault.IsPrinted(got) || !errors.Is(got, src) {
					t.Errorf("returned error not marked printed or lost the source: %v", got)
				}
			}
		})
	}
}

func TestErrOnce(t *testing.T) {
	h := newHarness(mode.Modes{})
	if h.out.Err(nil) != nil {
		t.Error("Err(nil) != nil")
	}
	first := h.out.Err(fault.New("NOT_FOUND", "missing", "list"))
	h.stderr.Reset()
	again := h.out.Err(fmt.Errorf("outer: %w", first))
	if h.stderr.Len() != 0 {
		t.Errorf("printed twice: %q", h.stderr.String())
	}
	if fault.ExitCode(again) != fault.ExitNotFound {
		t.Errorf("ExitCode = %d", fault.ExitCode(again))
	}
}

func TestDecoration(t *testing.T) {
	err := fault.New("NOT_FOUND", "missing", "list")
	plain := newHarness(mode.Modes{})
	_ = plain.out.Err(err)
	plain.out.Hint("h")
	if strings.Contains(plain.stderr.String(), "\x1b[") {
		t.Errorf("undecorated stderr carries escapes: %q", plain.stderr.String())
	}

	decorated := newHarness(modesFor(t, mode.Flags{Color: "always"}, mode.Env{}))
	_ = decorated.out.Err(err)
	decorated.out.Warning(fault.WarningPayload{Message: "w"})
	s := decorated.stderr.String()
	for _, want := range []string{"\x1b[31mError: missing", "\x1b[2m  Code: NOT_FOUND", "\x1b[33mWarning: w"} {
		if !strings.Contains(s, want) {
			t.Errorf("decorated stderr %q lacks %q", s, want)
		}
	}
}

func decodeEnvelope(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, b)
	}
	return env
}

func TestJSON(t *testing.T) {
	warning := fault.WarningPayload{Code: "FLAG_DEPRECATED", Message: "old", Fix: "new"}
	tests := []struct {
		name string
		v    func(t *testing.T) any
		warn []fault.WarningPayload
		want string
	}{
		{
			name: "nil data is null",
			v:    func(*testing.T) any { return nil },
			want: `{"data":null,"status":"ok"}`,
		},
		{
			name: "named fields keep only those keys and the cursor rides the envelope",
			v: func(t *testing.T) any {
				return output.Page{Table: output.NewTable(mustSelect(t, "status", "id"), items[:2], itemValues), Start: 1, Total: 9, Next: "tok"}
			},
			want: `{"data":[{"id":"a1","status":"open"},{"id":"b22","status":"closed"}],"next":"tok","status":"ok"}`,
		},
		{
			name: "empty table is an empty list and the last page has no cursor",
			v: func(t *testing.T) any {
				return output.Page{Table: output.Table{Fields: mustSelect(t)}, Start: 1, Total: 0}
			},
			want: `{"data":[],"status":"ok"}`,
		},
		{
			name: "hand-built table and record keep only selected keys",
			v: func(t *testing.T) any {
				sel := mustSelect(t, "id")
				return []any{
					output.Table{Fields: sel, Rows: []map[string]any{{"id": "a1", "secret": "x"}}},
					output.Record{Fields: sel, Values: map[string]any{"id": "b2", "secret": "y"}},
				}
			},
			want: `{"data":[[{"id":"a1"}],{"id":"b2"}],"status":"ok"}`,
		},
		{
			name: "a selected field missing from a row is null",
			v: func(t *testing.T) any {
				return output.NewTable(mustSelect(t, "id", "owner"), items[1:2], itemValues)
			},
			want: `{"data":[{"id":"b22","owner":null}],"status":"ok"}`,
		},
		{
			name: "record with warnings",
			v: func(t *testing.T) any {
				return output.NewRecord(mustSelect(t, "id"), items[0], itemValues)
			},
			warn: []fault.WarningPayload{warning},
			want: `{"data":{"id":"a1"},"status":"ok","warnings":[{"code":"FLAG_DEPRECATED","fix":"new","message":"old"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(modesFor(t, mode.Flags{JSON: true}, mode.Env{}))
			if err := h.out.Data(tt.v(t), tt.warn...); err != nil {
				t.Fatal(err)
			}
			got, err := json.Marshal(decodeEnvelope(t, h.stdout.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("envelope\n got: %s\nwant: %s", got, tt.want)
			}
			if h.stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", h.stderr.String())
			}
		})
	}
}

func TestJSONErr(t *testing.T) {
	h := newHarness(modesFor(t, mode.Flags{JSON: true, Color: "always"}, mode.Env{}))
	_, selErr := output.SelectFields([]string{"nope"}, itemFields)
	err := h.out.Err(selErr)
	env := decodeEnvelope(t, h.stdout.Bytes())
	want := map[string]any{
		"status": "error",
		"error": map[string]any{
			"code":         "INVALID_FIELDS",
			"message":      `unknown field: "nope"`,
			"fix":          "rerun with --fields set to a comma-separated list of the accepted fields",
			"valid_values": []any{"id", "title", "status", "owner"},
			"transient":    false,
		},
	}
	if !reflect.DeepEqual(env, want) {
		t.Errorf("envelope = %v", env)
	}
	if h.stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", h.stderr.String())
	}
	if fault.ExitCode(err) != fault.ExitUsage || !fault.IsPrinted(err) {
		t.Errorf("ExitCode %d printed %v", fault.ExitCode(err), fault.IsPrinted(err))
	}
	if strings.Contains(h.stdout.String(), "\x1b[") {
		t.Error("JSON stdout carries escapes")
	}
}

func TestJSONErrFallsBackToStderr(t *testing.T) {
	var stderr bytes.Buffer
	out := output.New(failWriter{}, &stderr, modesFor(t, mode.Flags{JSON: true}, mode.Env{}))
	err := out.Err(fault.New("CONFLICT", "busy", "retry later"))
	if !strings.HasPrefix(stderr.String(), "Error: busy\n") {
		t.Errorf("stderr = %q", stderr.String())
	}
	if fault.ExitCode(err) != fault.ExitConflict {
		t.Errorf("ExitCode = %d", fault.ExitCode(err))
	}
}

func TestProgress(t *testing.T) {
	tests := []struct {
		name string
		m    mode.Modes
		want string
	}{
		{name: "animated rewrites and clears the line", m: modesFor(t, mode.Flags{}, mode.Env{StderrTerminal: true}), want: "\r1/3\x1b[K\r2/3 a\\x1b[2Jb\x1b[K\r\x1b[K"},
		{name: "not animated is silent", m: modesFor(t, mode.Flags{Color: "always"}, mode.Env{})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(tt.m)
			p := h.out.Progress()
			p.Update("1/3")
			p.Update("2/3\na\x1b[2Jb")
			p.Done()
			if got := h.stderr.String(); got != tt.want {
				t.Errorf("stderr = %q, want %q", got, tt.want)
			}
			if h.stdout.Len() != 0 {
				t.Errorf("stdout = %q", h.stdout.String())
			}
		})
	}
}

func TestDataPagePointer(t *testing.T) {
	page := &output.Page{Table: output.NewTable(mustSelect(t, "id"), items[:1], itemValues), Start: 1, Total: 1, Next: "tok"}
	var nilPage *output.Page
	tests := []struct {
		name       string
		modes      mode.Modes
		v          any
		wantStdout string
		wantStderr string
	}{
		{name: "text", v: page, wantStdout: "a1\n", wantStderr: "Showing 1-1 of 1 results  (--next tok)\n"},
		{name: "JSON", modes: modesFor(t, mode.Flags{JSON: true}, mode.Env{}), v: page, wantStdout: "{\n  \"status\": \"ok\",\n  \"data\": [\n    {\n      \"id\": \"a1\"\n    }\n  ],\n  \"next\": \"tok\"\n}\n"},
		{name: "nil text", v: nilPage},
		{name: "nil JSON", modes: modesFor(t, mode.Flags{JSON: true}, mode.Env{}), v: nilPage, wantStdout: "{\n  \"status\": \"ok\",\n  \"data\": null\n}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(tt.modes)
			if err := h.out.Data(tt.v); err != nil {
				t.Fatal(err)
			}
			if h.stdout.String() != tt.wantStdout || h.stderr.String() != tt.wantStderr {
				t.Errorf("stdout %q stderr %q, want %q %q", h.stdout.String(), h.stderr.String(), tt.wantStdout, tt.wantStderr)
			}
		})
	}
}

func TestHostileValues(t *testing.T) {
	sel, err := output.SelectFields(nil, []string{"id", "title"})
	if err != nil {
		t.Fatal(err)
	}
	named, err := output.SelectFields([]string{"title"}, []string{"id", "title"})
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(mode.Modes{})
	results := []any{
		output.Record{Fields: sel, Values: map[string]any{"id": "a1", "title": "Fix\nid:     forged"}},
		output.Table{Fields: sel, Rows: []map[string]any{{"id": "x", "title": "\x1b]52;c;cm0=\x07t"}, {"id": "yy", "title": "z"}}},
		output.Record{Fields: named, Values: map[string]any{"title": "a\x1b[2Jb"}},
		output.Page{Table: output.Table{Fields: sel}, Start: 1, Total: 0, Next: "tok\x1b[2J"},
	}
	for _, v := range results {
		if err := h.out.Data(v); err != nil {
			t.Fatal(err)
		}
	}
	wantStdout := "" +
		"id:    a1\n" +
		"title: Fix id:     forged\n" +
		"id  title\n" +
		"x   \\x1b]52;c;cm0=\\x07t\n" +
		"yy  z\n" +
		"a\\x1b[2Jb\n" +
		"id  title\n" +
		"Showing 0 results  (--next tok\\x1b[2J)\n"
	if got := h.stdout.String(); got != wantStdout {
		t.Errorf("stdout\n got: %q\nwant: %q", got, wantStdout)
	}

	_ = h.out.Err(fault.New("UPSTREAM_REJECTED", "server said: \x1b[31mno\nCode: FAKE", "retry\nFix: rm -rf").WithValidValues("a\tb"))
	h.out.Warning(fault.WarningPayload{Message: "w\r\nWarning: fake", Fix: "f\x07"})
	h.out.Notice("saved x\x1b[2J\nError: fake")
	h.out.Hint("open evil\u202etxt.exe")
	wantStderr := "" +
		"Error: server said: \\x1b[31mno Code: FAKE\n" +
		"  Code:     UPSTREAM_REJECTED\n" +
		"  Accepted: a b\n" +
		"  Fix:      retry Fix: rm -rf\n" +
		"Warning: w Warning: fake\n" +
		"  Fix: f\\x07\n" +
		"saved x\\x1b[2J Error: fake\n" +
		"Hint: open evil\\u202etxt.exe\n"
	if got := h.stderr.String(); got != wantStderr {
		t.Errorf("stderr\n got: %q\nwant: %q", got, wantStderr)
	}
}
