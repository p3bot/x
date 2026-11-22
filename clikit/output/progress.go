package output

import "io"

// Progress rewrites one stderr line in place. It is silent unless the modes
// it was obtained under were animated.
type Progress struct {
	w       io.Writer
	enabled bool
}

// Progress returns a progress indicator on o's stderr, enabled only when o's
// modes are animated.
func (o *Output) Progress() *Progress {
	return &Progress{w: o.stderr, enabled: o.modes.Animated()}
}

// Update replaces the current line with msg, kept to one line.
func (p *Progress) Update(msg string) {
	if !p.enabled {
		return
	}
	writeDiagnostic(p.w, "\r"+cleanText(msg)+"\x1b[K")
}

// Done clears the line.
func (p *Progress) Done() {
	if !p.enabled {
		return
	}
	writeDiagnostic(p.w, "\r\x1b[K")
}
