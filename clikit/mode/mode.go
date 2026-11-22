// Package mode detects a CLI invocation's runtime modes (interactive,
// decorated, animated, debug) from its global flags and the process
// environment, and applies the colour decision to fatih/color.
package mode

import (
	"errors"
	"os"
	"strings"

	"github.com/fatih/color"
	"golang.org/x/term"

	"github.com/p3bot/x/clikit/fault"
)

// Accepted --color values. An empty value means ColorAuto.
const (
	ColorAuto   = "auto"
	ColorAlways = "always"
	ColorNever  = "never"
)

// Flags holds the global flag values that detection reads.
type Flags struct {
	Color   string // --color: ColorAuto, ColorAlways, ColorNever, or "" for auto
	NoInput bool   // --no-input
	JSON    bool   // --json
	Quiet   bool   // --quiet
	Debug   bool   // --debug
}

// Env is the process state that detection reads. The zero Env has no
// variables set and no terminals, so tests state exactly what they need.
type Env struct {
	LookupEnv      func(name string) (string, bool)
	StdinTerminal  bool
	StdoutTerminal bool
	StderrTerminal bool
}

// ProcessEnv returns the Env of the running process.
func ProcessEnv() Env {
	return Env{
		LookupEnv:      os.LookupEnv,
		StdinTerminal:  isTerminal(os.Stdin),
		StdoutTerminal: isTerminal(os.Stdout),
		StderrTerminal: isTerminal(os.Stderr),
	}
}

func isTerminal(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

func (e Env) lookup(name string) (string, bool) {
	if e.LookupEnv == nil {
		return "", false
	}
	return e.LookupEnv(name)
}

func (e Env) getenv(name string) string {
	v, _ := e.lookup(name)
	return v
}

// Bool reads a CLI-owned boolean variable strictly: 1, true, and yes are
// true; 0, false, no, empty, and unset are false, all case-insensitive. Any
// other value is a CONFIG_INVALID error, so a typo fails instead of reading
// as false.
func (e Env) Bool(name string) (bool, error) {
	raw, ok := e.lookup(name)
	if !ok {
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "0", "false", "no":
		return false, nil
	case "1", "true", "yes":
		return true, nil
	default:
		return false, fault.New("CONFIG_INVALID",
			name+" has invalid value (expected one of: 1, 0, true, false, yes, no)",
			"unset "+name+" or set it to 1/true/yes/0/false/no",
		).WithValidValues("1", "0", "true", "false", "yes", "no")
	}
}

// Modes are the runtime modes of one invocation. Only Detect sets them, so
// they always obey the detection rules; the zero Modes is a plain,
// non-interactive invocation with every mode off.
type Modes struct {
	interactive     bool
	stdoutDecorated bool
	stderrDecorated bool
	animated        bool
	json            bool
	quiet           bool
	debug           bool
}

// Interactive reports whether prompts are allowed.
func (m Modes) Interactive() bool { return m.interactive }

// StdoutDecorated reports whether ANSI escapes may reach stdout; never under JSON.
func (m Modes) StdoutDecorated() bool { return m.stdoutDecorated }

// StderrDecorated reports whether ANSI escapes may reach stderr.
func (m Modes) StderrDecorated() bool { return m.stderrDecorated }

// Animated reports whether spinners and progress may rewrite stderr.
func (m Modes) Animated() bool { return m.animated }

// JSON reports whether stdout carries only the JSON envelope.
func (m Modes) JSON() bool { return m.json }

// Quiet reports whether warnings, notices, hints, and animation are dropped.
func (m Modes) Quiet() bool { return m.quiet }

// Debug reports whether diagnostic logging is on.
func (m Modes) Debug() bool { return m.debug }

// Detect resolves the modes for the CLI named cli, whose own variables are
// <CLI>_NO_INPUT and <CLI>_DEBUG with dashes in the name read as underscores.
//
// An invalid flags.Color is an INVALID_COLOR error and a malformed CLI-owned
// variable a CONFIG_INVALID error. On those errors the returned Modes still
// carry the decoration decision, with an invalid colour read as auto, so the
// caller can render the error itself as decorated as it should be.
func Detect(cli string, flags Flags, env Env) (Modes, error) {
	if cli == "" {
		return Modes{}, errors.New("mode: empty CLI name")
	}
	colorErr := validateColor(cli, flags.Color)
	m := Modes{
		json:            flags.JSON,
		quiet:           flags.Quiet,
		stdoutDecorated: decorates(flags.Color, env, env.StdoutTerminal) && !flags.JSON,
		stderrDecorated: decorates(flags.Color, env, env.StderrTerminal),
	}
	m.animated = m.stderrDecorated && env.StderrTerminal && !flags.Quiet
	if colorErr != nil {
		return m, colorErr
	}

	prefix := strings.ToUpper(strings.ReplaceAll(cli, "-", "_")) + "_"
	noInput, err := env.Bool(prefix + "NO_INPUT")
	if err != nil {
		return m, err
	}
	m.interactive = env.StdinTerminal && !flags.NoInput && !flags.JSON && !noInput

	m.debug = flags.Debug
	if !m.debug {
		if m.debug, err = env.Bool(prefix + "DEBUG"); err != nil {
			return m, err
		}
	}
	return m, nil
}

func validateColor(cli, value string) error {
	switch value {
	case "", ColorAuto, ColorAlways, ColorNever:
		return nil
	default:
		return fault.New("INVALID_COLOR",
			"--color must be one of: auto, always, never",
			cli+" --color=auto",
		).WithValidValues(ColorAuto, ColorAlways, ColorNever)
	}
}

// decorates applies the decoration rule for one stream, first match winning.
// The colour variables are not CLI-owned, so any non-empty value counts as
// set, FORCE_COLOR=0 included.
func decorates(colorFlag string, env Env, terminal bool) bool {
	switch {
	case colorFlag == ColorAlways:
		return true
	case colorFlag == ColorNever, env.getenv("NO_COLOR") != "":
		return false
	case env.getenv("FORCE_COLOR") != "", env.getenv("CLICOLOR_FORCE") != "":
		return true
	case env.getenv("TERM") == "dumb":
		return false
	default:
		return terminal
	}
}

// ApplyColor sets fatih/color's package-wide switch to m's stderr decoration
// and stamps every style handed in to match. Call it once per process after
// Detect, with every style the CLI owns.
//
// fatih/color stamps a style as colourless when NO_COLOR is set at its
// construction, and that stamp outranks the package-wide switch, so only the
// per-style stamp lets --color=always win for an existing style.
func (m Modes) ApplyColor(styles ...*color.Color) {
	color.NoColor = !m.stderrDecorated
	m.StampStyles(styles...)
}

// StampStyles stamps each style with m's stderr decoration without touching
// fatih/color's package-wide switch.
func (m Modes) StampStyles(styles ...*color.Color) {
	for _, s := range styles {
		if m.stderrDecorated {
			s.EnableColor()
		} else {
			s.DisableColor()
		}
	}
}
