package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/p3bot/x/clikit/fault"
)

// TextRenderer is implemented by every value Data renders in text mode other
// than a Page, a []string, or nil.
type TextRenderer interface {
	RenderText(w io.Writer) error
}

// Selection is a validated field selection for one command, built by
// SelectFields. It always names at least one field, and records whether the
// caller named the fields, which switches text output to headerless TSV. The
// zero Selection names none, so a Table or Record holding it fails to render.
type Selection struct {
	names    []string
	explicit bool
}

// SelectFields validates requested against a command's known field set. With
// nothing requested the selection is the whole known set, not explicit. Names
// are trimmed of spaces, and a repeated name keeps its first position. Any
// name outside known is an INVALID_FIELDS error that lists known as the
// accepted values. An empty known is the command's own bug, reported as an
// uncoded error rather than blamed on requested.
func SelectFields(requested, known []string) (Selection, error) {
	if len(known) == 0 {
		return Selection{}, errors.New("output: command declares no fields")
	}
	if len(requested) == 0 {
		return Selection{names: slices.Clone(known)}, nil
	}
	names := make([]string, 0, len(requested))
	var unknown []string
	for _, r := range requested {
		name := strings.TrimSpace(r)
		quoted := fmt.Sprintf("%q", name)
		switch {
		case !slices.Contains(known, name):
			if !slices.Contains(unknown, quoted) {
				unknown = append(unknown, quoted)
			}
		case !slices.Contains(names, name):
			names = append(names, name)
		}
	}
	if len(unknown) > 0 {
		label := "unknown field: "
		if len(unknown) > 1 {
			label = "unknown fields: "
		}
		return Selection{}, fault.New("INVALID_FIELDS",
			label+strings.Join(unknown, ", "),
			"rerun with --fields set to a comma-separated list of the accepted fields",
		).WithValidValues(known...)
	}
	return Selection{names: names, explicit: true}, nil
}

// Names returns the selected field names in output order.
func (s Selection) Names() []string { return slices.Clone(s.names) }

// Explicit reports whether the caller named the fields.
func (s Selection) Explicit() bool { return s.explicit }

func (s Selection) validate() error {
	if len(s.names) == 0 {
		return errors.New("output: field selection is empty; build it with SelectFields")
	}
	return nil
}

// Table is a list result. Both text and JSON output show only the selected
// fields of each row, whatever else a row holds. Text output is an aligned
// table under a header, or headerless TSV when the fields were named; each
// value renders as one line of text with no control characters (see
// cleanText), so multi-line prose belongs in a TextRenderer of the CLI's own.
type Table struct {
	Fields Selection
	Rows   []map[string]any
}

// NewTable builds a Table with one row of values per item.
func NewTable[T any](fields Selection, items []T, values func(T) map[string]any) Table {
	rows := make([]map[string]any, len(items))
	for i, it := range items {
		rows[i] = values(it)
	}
	return Table{Fields: fields, Rows: rows}
}

// RenderText writes the rows as an aligned table, or as TSV when the fields
// were named. It fails when Fields was not built by SelectFields.
func (t Table) RenderText(w io.Writer) error {
	if err := t.Fields.validate(); err != nil {
		return err
	}
	cells := make([][]string, len(t.Rows))
	for i, row := range t.Rows {
		cells[i] = rowCells(row, t.Fields.names)
	}
	if t.Fields.explicit {
		return writeTSV(w, cells)
	}
	return writeAligned(w, slices.Insert(cells, 0, t.Fields.names), 2)
}

// MarshalJSON emits the rows alone, each with only the selected fields. It
// fails when Fields was not built by SelectFields.
func (t Table) MarshalJSON() ([]byte, error) {
	if err := t.Fields.validate(); err != nil {
		return nil, err
	}
	rows := make([]map[string]any, len(t.Rows))
	for i, row := range t.Rows {
		rows[i] = pick(row, t.Fields.names)
	}
	return json.Marshal(rows)
}

// Record is a single resource. Both text and JSON output show only its
// selected fields. Text output is aligned "label: value" lines, or one TSV
// line when the fields were named; values render as Table's do.
type Record struct {
	Fields Selection
	Values map[string]any
}

// NewRecord builds a Record from item's values.
func NewRecord[T any](fields Selection, item T, values func(T) map[string]any) Record {
	return Record{Fields: fields, Values: values(item)}
}

// RenderText writes one aligned "label: value" line per field, or one TSV line
// when the fields were named. It fails when Fields was not built by
// SelectFields.
func (r Record) RenderText(w io.Writer) error {
	if err := r.Fields.validate(); err != nil {
		return err
	}
	values := rowCells(r.Values, r.Fields.names)
	if r.Fields.explicit {
		return writeTSV(w, [][]string{values})
	}
	rows := make([][]string, len(values))
	for i, f := range r.Fields.names {
		rows[i] = []string{f + ":", values[i]}
	}
	return writeAligned(w, rows, 1)
}

// MarshalJSON emits the selected values alone. It fails when Fields was not
// built by SelectFields.
func (r Record) MarshalJSON() ([]byte, error) {
	if err := r.Fields.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(pick(r.Values, r.Fields.names))
}

// Page is one page of a list result. Output.Data renders it: the table, then
// the range and the cursor to advance. Page has no RenderText of its own,
// because with named fields its indicator goes to stderr, not to the table's
// writer. A Page with rows must set Start; leaving Total zero reads as unknown.
type Page struct {
	Table Table
	Start int    // 1-based position of the first row; required when there are rows
	Total int    // 0 when unknown, such as when counting is too expensive
	Next  string // cursor for the following page; "" on the last page
}

// MarshalJSON emits the rows alone; the cursor rides at the envelope root. It
// fails on a Page that would not render as text.
func (p Page) MarshalJSON() ([]byte, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	return p.Table.MarshalJSON()
}

// NextCursor returns the cursor for the following page, or "" on the last page.
func (p Page) NextCursor() string { return p.Next }

func (p Page) validate() error {
	if len(p.Table.Rows) > 0 && p.Start < 1 {
		return fmt.Errorf("output: page Start must be 1 or more, got %d", p.Start)
	}
	return nil
}

// indicator assumes validate passed. A page with rows is never 0 of a known
// total, which is what lets Total 0 mean unknown.
func (p Page) indicator() string {
	var b strings.Builder
	if n := len(p.Table.Rows); n == 0 {
		b.WriteString("Showing 0 results")
	} else {
		fmt.Fprintf(&b, "Showing %d-%d", p.Start, p.Start+n-1)
		if p.Total > 0 {
			fmt.Fprintf(&b, " of %d", p.Total)
		}
		b.WriteString(" results")
	}
	if p.Next != "" {
		fmt.Fprintf(&b, "  (--next %s)", cleanText(p.Next))
	}
	return b.String()
}

func pick(all map[string]any, fields []string) map[string]any {
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		out[f] = all[f]
	}
	return out
}

func rowCells(values map[string]any, fields []string) []string {
	cells := make([]string, len(fields))
	for i, f := range fields {
		cells[i] = cleanText(textValue(values[f]))
	}
	return cells
}

// textValue renders an empty value empty: no null, (none), or -.
func textValue(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// cleanText keeps a value on one line, in one column, and free of characters
// that can forge or reorder output, because values come from data the CLI does
// not control: tab, CR, LF, and the Unicode line and paragraph separators
// become one space (\r\n counts once), and any other control character, bidi
// control, or byte that is not valid UTF-8 becomes a visible Go escape such as
// \x1b or \u202e. A raw byte is escaped too, because 0x80-0x9f are 8-bit
// controls to some terminals.
func cleanText(s string) string {
	if utf8.ValidString(s) && !strings.ContainsFunc(s, isUnsafeRune) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, "\\x%02x", s[i])
		case r == '\r' && strings.HasPrefix(s[i+size:], "\n"):
			b.WriteByte(' ')
			size++
		case r == '\t', r == '\r', r == '\n', r == '\u2028', r == '\u2029':
			b.WriteByte(' ')
		case r < utf8.RuneSelf && unicode.IsControl(r):
			fmt.Fprintf(&b, "\\x%02x", r)
		case isUnsafeRune(r):
			fmt.Fprintf(&b, "\\u%04x", r)
		default:
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

func isUnsafeRune(r rune) bool {
	return unicode.IsControl(r) || r == '\u2028' || r == '\u2029' || unicode.Is(unicode.Bidi_Control, r)
}

func writeTSV(w io.Writer, rows [][]string) error {
	for _, r := range rows {
		if _, err := fmt.Fprintln(w, strings.Join(r, "\t")); err != nil {
			return err
		}
	}
	return nil
}

// writeAligned pads every column but the last to its widest cell plus gap.
// Lines never carry trailing spaces. Width is a rune count, which assumes
// every character is one terminal column.
func writeAligned(w io.Writer, rows [][]string, gap int) error {
	var widths []int
	for _, r := range rows {
		for i, c := range r {
			if i == len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], utf8.RuneCountInString(c))
		}
	}
	for _, r := range rows {
		var b strings.Builder
		for i, c := range r {
			b.WriteString(c)
			if i < len(r)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c)+gap))
			}
		}
		if _, err := fmt.Fprintln(w, strings.TrimRight(b.String(), " ")); err != nil {
			return err
		}
	}
	return nil
}
