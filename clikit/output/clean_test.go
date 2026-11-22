package output

import "testing"

func TestCleanText(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain text is unchanged", "héllo wörld", "héllo wörld"},
		{"tab", "a\tb", "a b"},
		{"newline", "a\nb", "a b"},
		{"carriage return", "a\rb", "a b"},
		{"CRLF counts once", "a\r\nb", "a b"},
		{"CR CR LF", "a\r\r\nb", "a  b"},
		{"two tabs stay two spaces", "a\t\tb", "a  b"},
		{"ESC", "a\x1b[31mb", `a\x1b[31mb`},
		{"OSC 52 with BEL", "\x1b]52;c;cm0=\x07", `\x1b]52;c;cm0=\x07`},
		{"NUL", "a\x00b", `a\x00b`},
		{"DEL", "a\x7fb", `a\x7fb`},
		{"C1 CSI", "a\u009b2Jb", `a\u009b2Jb`},
		{"trailing CR", "a\r", "a "},
		{"raw 8-bit CSI byte", "a\x9b2Jb", `a\x9b2Jb`},
		{"other invalid bytes", "a\xff\xfeb", `a\xff\xfeb`},
		{"truncated multi-byte sequence", "a\xc3", `a\xc3`},
		{"valid replacement character is kept", "a\uFFFDb", "a\uFFFDb"},
		{"right-to-left override", "invoice\u202Efdp.exe", `invoice\u202efdp.exe`},
		{"bidi isolates and marks", "\u2066a\u2069\u200E\u200F\u061C", `\u2066a\u2069\u200e\u200f\u061c`},
		{"line separator", "a\u2028b", "a b"},
		{"paragraph separator", "a\u2029b", "a b"},
		{"emoji joiner is kept", "\U0001F469\u200D\U0001F4BB", "\U0001F469\u200D\U0001F4BB"},
		{"soft hyphen is kept", "co\u00ADop", "co\u00ADop"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanText(tt.in); got != tt.want {
				t.Errorf("cleanText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
