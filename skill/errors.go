package skill

import (
	"errors"
	"fmt"
	"slices"

	"github.com/p3bot/x/skill/catalog"
)

// Sentinels for the coded errors this package returns, matched with
// errors.Is. Each coded error also carries a canonical payload code and a fix
// through the method set clikit's fault package reads.
var (
	// ErrUnknownAgent is a named agent the catalog does not know. Code
	// AGENT_UNKNOWN; the valid values are the catalog's agent ids.
	ErrUnknownAgent = errors.New("unknown agent")
	// ErrNoSkillsConcept is a named agent with no skills roots in the
	// catalog. Code NO_SKILLS_CONCEPT.
	ErrNoSkillsConcept = errors.New("agent has no skills concept")
	// ErrNoSkillsRoot is a target with no skills root at the location, or a
	// root that refuses writes. Code NO_SKILLS_ROOT.
	ErrNoSkillsRoot = errors.New("no writable skills root")
	// ErrNoAgents is an empty default set: no installed agent has a skills
	// concept. Code NO_AGENTS.
	ErrNoAgents = errors.New("no installed agents with a skills concept")
	// ErrForeignCopy is a copy at a skills root that the skill does not own,
	// which install refuses to replace. Code CONFLICT.
	ErrForeignCopy = errors.New("existing copy is not this skill")
)

// Canonical payload codes, from the p3bot CLI code table.
const (
	codeAgentUnknown    = "AGENT_UNKNOWN"
	codeNoSkillsConcept = "NO_SKILLS_CONCEPT"
	codeNoSkillsRoot    = "NO_SKILLS_ROOT"
	codeNoAgents        = "NO_AGENTS"
	codeConflict        = "CONFLICT"
)

const conflictFix = "inspect the copy named in the message, remove it if unwanted, and rerun"

// operation names the skill subcommand a call serves, for fixes that repeat it.
type operation string

const (
	opInstall   operation = "install"
	opUninstall operation = "uninstall"
)

// codedError implements the coded-error interfaces of clikit/fault
// structurally, so this package never imports fault. Exit codes come from the
// canonical table, so it has no ExitCode method.
type codedError struct {
	sentinel error
	code     string
	message  string
	fix      string
	valid    []string
	cause    error
}

func (e *codedError) Error() string         { return e.message }
func (e *codedError) Code() string          { return e.code }
func (e *codedError) Fix() string           { return e.fix }
func (e *codedError) ValidValues() []string { return slices.Clone(e.valid) }

func (e *codedError) Unwrap() []error {
	if e.cause == nil {
		return []error{e.sentinel}
	}
	return []error{e.sentinel, e.cause}
}

// manualFix installs by hand, which needs neither the catalog nor a root this
// package could resolve.
func (s Skill) manualFix() string {
	return s.id + " skill > <agent-skills-dir>/" + s.id + "/" + FileName
}

func (s Skill) errUnknownAgent(op operation, id string) error {
	agents := catalog.Agents()
	known := make([]string, len(agents))
	for i, a := range agents {
		known[i] = a.ID
	}
	return &codedError{
		sentinel: ErrUnknownAgent,
		code:     codeAgentUnknown,
		message:  fmt.Sprintf("unknown agent %q", id),
		fix:      fmt.Sprintf("%s skill %s <agent>", s.id, op),
		valid:    known,
	}
}

func (s Skill) errNoSkillsConcept(id string) error {
	return &codedError{
		sentinel: ErrNoSkillsConcept,
		code:     codeNoSkillsConcept,
		message:  fmt.Sprintf("agent %q has no skills concept", id),
		fix:      s.manualFix(),
	}
}

func (s Skill) errNoSkillsRoot(message string, cause error) error {
	return &codedError{
		sentinel: ErrNoSkillsRoot,
		code:     codeNoSkillsRoot,
		message:  message,
		fix:      s.manualFix(),
		cause:    cause,
	}
}

func (s Skill) errNoAgents() error {
	return &codedError{
		sentinel: ErrNoAgents,
		code:     codeNoAgents,
		message:  "no installed agents with a skills concept",
		fix:      s.manualFix(),
	}
}

func errForeignCopy(path, reason string) error {
	return &codedError{
		sentinel: ErrForeignCopy,
		code:     codeConflict,
		message:  fmt.Sprintf("foreign copy at %s: %s", path, reason),
		fix:      conflictFix,
	}
}
