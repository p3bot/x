// Package output writes a CLI invocation's results, errors, warnings, notices,
// hints, and progress, honouring its runtime modes: text by default, the JSON
// envelope when JSON is on.
//
// An Output is built once per invocation and passed to commands; the package
// keeps no state of its own.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"

	"github.com/p3bot/x/clikit/fault"
	"github.com/p3bot/x/clikit/mode"
)

// Output writes one invocation's output to its stdout and stderr.
type Output struct {
	stdout    io.Writer
	stderr    io.Writer
	modes     mode.Modes
	errStyle  *color.Color
	warnStyle *color.Color
	dimStyle  *color.Color
}

// New returns an Output writing to stdout and stderr under m. Its styles are
// its own and stamped from m, so it never reads or sets fatih/color's
// package-wide switch.
func New(stdout, stderr io.Writer, m mode.Modes) *Output {
	o := &Output{
		stdout:    stdout,
		stderr:    stderr,
		modes:     m,
		errStyle:  color.New(color.FgRed),
		warnStyle: color.New(color.FgYellow),
		dimStyle:  color.New(color.Faint),
	}
	m.StampStyles(o.errStyle, o.warnStyle, o.dimStyle)
	return o
}

// Data writes a successful result. In text mode v is a Page or *Page, a
// TextRenderer, a []string written one per line, or nil for no output, and
// each warning follows on stderr unless quiet. With JSON on, v and the
// warnings form one envelope on stdout. It returns rendering and write
// failures, so a lost result never exits 0.
func (o *Output) Data(v any, warnings ...fault.WarningPayload) error {
	if p, ok := v.(*Page); ok {
		v = nil
		if p != nil {
			v = *p
		}
	}
	if o.modes.JSON() {
		return o.writeEnvelope(okEnvelope(v, warnings))
	}
	if err := o.renderText(v); err != nil {
		return fmt.Errorf("rendering result: %w", err)
	}
	for _, w := range warnings {
		o.Warning(w)
	}
	return nil
}

func (o *Output) renderText(v any) error {
	switch t := v.(type) {
	case nil:
		return nil
	case Page:
		return o.renderPage(t)
	case TextRenderer:
		return t.RenderText(o.stdout)
	case []string:
		for _, s := range t {
			if _, err := fmt.Fprintln(o.stdout, s); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%T does not implement output.TextRenderer", v)
	}
}

// renderPage writes the indicator after the table, or to stderr when the
// fields were named so the TSV stays pure. Either way it is part of the
// result: it ignores quiet and its write error is returned, because a lost
// cursor cannot be recovered.
func (o *Output) renderPage(p Page) error {
	if err := p.validate(); err != nil {
		return err
	}
	if err := p.Table.RenderText(o.stdout); err != nil {
		return err
	}
	w := o.stdout
	if p.Table.Fields.Explicit() {
		w = o.stderr
	}
	_, err := fmt.Fprintln(w, p.indicator())
	return err
}

// Err writes err's payload and returns an error that wraps err, fixes the
// exit code at the payload's, and is marked printed. In text mode the payload
// goes to stderr as "Error: <message>" with Code, Accepted, Retry after, and
// Fix lines; with JSON on it is one envelope on stdout, falling back to the
// text form when stdout fails. An error already printed is returned
// unchanged, and nil returns nil.
func (o *Output) Err(err error) error {
	if err == nil || fault.IsPrinted(err) {
		return err
	}
	p := fault.Payload(err)
	if !o.modes.JSON() || o.writeEnvelope(envelope{Status: statusError, Error: p}) != nil {
		labels, values := []string{"Code"}, []string{p.Code}
		if len(p.ValidValues) > 0 {
			labels, values = append(labels, "Accepted"), append(values, strings.Join(p.ValidValues, ", "))
		}
		if p.RetryAfter != "" {
			labels, values = append(labels, "Retry after"), append(values, p.RetryAfter)
		}
		if p.Fix != "" {
			labels, values = append(labels, "Fix"), append(values, p.Fix)
		}
		writeDiagnostic(o.stderr, o.errStyle.Sprint("Error: "+cleanText(p.Message))+"\n")
		o.writeDetail(labels, values)
	}
	return fault.MarkPrinted(fault.WithFinalExit(p.Exit(), err))
}

// Warning writes w to stderr unless quiet, with its Retry after and Fix when
// set. A warning that belongs to a result rides in Data instead, which puts it
// in the envelope when JSON is on.
func (o *Output) Warning(w fault.WarningPayload) {
	if o.modes.Quiet() {
		return
	}
	var labels, values []string
	if w.RetryAfter != "" {
		labels, values = append(labels, "Retry after"), append(values, w.RetryAfter)
	}
	if w.Fix != "" {
		labels, values = append(labels, "Fix"), append(values, w.Fix)
	}
	writeDiagnostic(o.stderr, o.warnStyle.Sprint("Warning: "+cleanText(w.Message))+"\n")
	o.writeDetail(labels, values)
}

// Notice writes msg to stderr as one line for a human, unless quiet or JSON is
// on. A notice of several lines is several calls.
func (o *Output) Notice(msg string) {
	if o.modes.Quiet() || o.modes.JSON() {
		return
	}
	writeDiagnostic(o.stderr, o.warnStyle.Sprint(cleanText(msg))+"\n")
}

// Hint writes "Hint: <msg>" to stderr as one line for a human, unless quiet or
// JSON is on.
func (o *Output) Hint(msg string) {
	if o.modes.Quiet() || o.modes.JSON() {
		return
	}
	writeDiagnostic(o.stderr, o.dimStyle.Sprint("Hint: "+cleanText(msg))+"\n")
}

// writeDetail writes indented, aligned "label: value" lines to stderr.
func (o *Output) writeDetail(labels, values []string) {
	width := 0
	for _, l := range labels {
		width = max(width, len(l))
	}
	for i, l := range labels {
		writeDiagnostic(o.stderr, o.dimStyle.Sprint(fmt.Sprintf("  %-*s %s", width+1, l+":", cleanText(values[i])))+"\n")
	}
}

// writeDiagnostic writes s to stderr and drops a write error, because a failed
// diagnostic has nowhere left to be reported. Results never go through it.
func writeDiagnostic(w io.Writer, s string) {
	_, _ = io.WriteString(w, s)
}

const (
	statusOK    = "ok"
	statusError = "error"
)

// envelope is the single stdout document when JSON is on.
type envelope struct {
	Status   string                 `json:"status"`
	Data     any                    `json:"data,omitempty"`
	Error    *fault.ErrorPayload    `json:"error,omitempty"`
	Warnings []fault.WarningPayload `json:"warnings,omitempty"`
	Next     string                 `json:"next,omitempty"`
}

func okEnvelope(v any, warnings []fault.WarningPayload) envelope {
	env := envelope{Status: statusOK, Data: v, Warnings: warnings}
	if v == nil {
		// omitempty would drop data, and a success always carries it.
		env.Data = json.RawMessage("null")
	}
	if p, ok := v.(interface{ NextCursor() string }); ok {
		env.Next = p.NextCursor()
	}
	return env
}

func (o *Output) writeEnvelope(env envelope) error {
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding envelope: %w", err)
	}
	_, err = fmt.Fprintln(o.stdout, string(data))
	return err
}
