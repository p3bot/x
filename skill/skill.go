// Package skill installs, lists, and uninstalls a CLI's agent skill in the
// skills roots of the agents in the catalog.
//
// A CLI builds one Skill from its embedded SKILL.md with New and calls
// Install, List, and Uninstall from its skill commands. The package touches
// only copies it owns: a copy is owned when <root>/<id> is a real directory,
// not a symbolic link, and <root>/<id>/SKILL.md is a regular file whose
// frontmatter name is the skill id. Failures in agent selection,
// a root that refuses writes, and a foreign copy are coded errors that
// clikit's fault package turns into error payloads; every other error is
// returned unchanged.
package skill

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// FileName is the skill's entry file inside <root>/<id>/.
const FileName = "SKILL.md"

// idPattern and maxIDLen are the Agent Skills name rules, so the id is always
// one safe path element.
var idPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const maxIDLen = 64

// Skill is one CLI's agent skill: its id, which is the CLI name, the
// directory name under a skills root, and the frontmatter name, and its
// SKILL.md text. The zero Skill is not usable; build one with New.
type Skill struct {
	id   string
	text string
}

// New returns the skill with the given id and SKILL.md text, adding a final
// newline when text lacks one. It fails when id is not a valid Agent Skills
// name or the frontmatter name of text is not id, so a Skill always passes
// its own ownership check.
func New(id, text string) (Skill, error) {
	if len(id) > maxIDLen || !idPattern.MatchString(id) {
		return Skill{}, fmt.Errorf("skill id %q is not 1 to %d lowercase letters, digits, and single inner hyphens", id, maxIDLen)
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	name, err := frontmatterName([]byte(text))
	if err != nil {
		return Skill{}, fmt.Errorf("skill %q: %w", id, err)
	}
	if name != id {
		return Skill{}, fmt.Errorf("skill %q: frontmatter name is %q", id, name)
	}
	return Skill{id: id, text: text}, nil
}

// ID returns the skill id.
func (s Skill) ID() string { return s.id }

// Text returns the SKILL.md text, ending in a newline.
func (s Skill) Text() string { return s.text }

// Location selects global or project-local skills roots.
type Location int

// Skills root locations.
const (
	Global Location = iota
	Local
)

// String returns "global" or "local".
func (l Location) String() string {
	switch l {
	case Global:
		return "global"
	case Local:
		return "local"
	}
	return fmt.Sprintf("Location(%d)", int(l))
}

// check rejects a zero Skill and an unknown Location before an operation
// builds a path from them.
func (s Skill) check(loc Location) error {
	if s.id == "" {
		return errors.New("skill: zero Skill; build one with New")
	}
	if loc != Global && loc != Local {
		return fmt.Errorf("skill: unknown %v", loc)
	}
	return nil
}
