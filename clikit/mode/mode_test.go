package mode_test

import (
	"strings"
	"testing"

	"github.com/fatih/color"

	"github.com/p3bot/x/clikit/fault"
	"github.com/p3bot/x/clikit/mode"
)

func envOf(vars map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		v, ok := vars[name]
		return v, ok
	}
}

func TestDecoration(t *testing.T) {
	tests := []struct {
		name       string
		color      string
		vars       map[string]string
		json       bool
		terminal   bool
		wantStdout bool
		wantStderr bool
	}{
		{name: "terminal decorates", terminal: true, wantStdout: true, wantStderr: true},
		{name: "pipe does not decorate", terminal: false},
		{name: "auto on terminal", color: "auto", terminal: true, wantStdout: true, wantStderr: true},
		{name: "always beats NO_COLOR", color: "always", vars: map[string]string{"NO_COLOR": "1"}, wantStdout: true, wantStderr: true},
		{name: "never beats FORCE_COLOR", color: "never", vars: map[string]string{"FORCE_COLOR": "1"}, terminal: true},
		{name: "NO_COLOR beats FORCE_COLOR", vars: map[string]string{"NO_COLOR": "1", "FORCE_COLOR": "1"}, terminal: true},
		{name: "empty NO_COLOR is unset", vars: map[string]string{"NO_COLOR": ""}, terminal: true, wantStdout: true, wantStderr: true},
		{name: "FORCE_COLOR=0 counts as set", vars: map[string]string{"FORCE_COLOR": "0"}, wantStdout: true, wantStderr: true},
		{name: "CLICOLOR_FORCE forces", vars: map[string]string{"CLICOLOR_FORCE": "1"}, wantStdout: true, wantStderr: true},
		{name: "empty FORCE_COLOR is unset", vars: map[string]string{"FORCE_COLOR": ""}},
		{name: "FORCE_COLOR beats TERM=dumb", vars: map[string]string{"FORCE_COLOR": "1", "TERM": "dumb"}, wantStdout: true, wantStderr: true},
		{name: "TERM=dumb on terminal", vars: map[string]string{"TERM": "dumb"}, terminal: true},
		{name: "JSON never decorates stdout", color: "always", json: true, wantStderr: true},
		{name: "invalid colour reads as auto", color: "sometimes", terminal: true, wantStdout: true, wantStderr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := mode.Env{LookupEnv: envOf(tt.vars), StdoutTerminal: tt.terminal, StderrTerminal: tt.terminal}
			m, _ := mode.Detect("demo", mode.Flags{Color: tt.color, JSON: tt.json}, env)
			if m.StdoutDecorated() != tt.wantStdout || m.StderrDecorated() != tt.wantStderr {
				t.Errorf("stdout %v stderr %v, want %v %v", m.StdoutDecorated(), m.StderrDecorated(), tt.wantStdout, tt.wantStderr)
			}
		})
	}
}

func TestStreamsDecideSeparately(t *testing.T) {
	m, err := mode.Detect("demo", mode.Flags{}, mode.Env{StderrTerminal: true})
	if err != nil {
		t.Fatal(err)
	}
	if m.StdoutDecorated() || !m.StderrDecorated() {
		t.Errorf("stdout %v stderr %v, want false true", m.StdoutDecorated(), m.StderrDecorated())
	}
}

func TestInteractive(t *testing.T) {
	tests := []struct {
		name  string
		flags mode.Flags
		vars  map[string]string
		stdin bool
		want  bool
	}{
		{name: "terminal stdin", stdin: true, want: true},
		{name: "piped stdin", stdin: false},
		{name: "--no-input", flags: mode.Flags{NoInput: true}, stdin: true},
		{name: "--json", flags: mode.Flags{JSON: true}, stdin: true},
		{name: "env no input", vars: map[string]string{"MY_CLI_NO_INPUT": "yes"}, stdin: true},
		{name: "env no input false", vars: map[string]string{"MY_CLI_NO_INPUT": "0"}, stdin: true, want: true},
		{name: "quiet still prompts", flags: mode.Flags{Quiet: true}, stdin: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := mode.Detect("my-cli", tt.flags, mode.Env{LookupEnv: envOf(tt.vars), StdinTerminal: tt.stdin})
			if err != nil {
				t.Fatal(err)
			}
			if m.Interactive() != tt.want {
				t.Errorf("Interactive = %v, want %v", m.Interactive(), tt.want)
			}
		})
	}
}

func TestAnimated(t *testing.T) {
	tests := []struct {
		name     string
		flags    mode.Flags
		vars     map[string]string
		terminal bool
		want     bool
	}{
		{name: "decorated terminal", terminal: true, want: true},
		{name: "quiet", flags: mode.Flags{Quiet: true}, terminal: true},
		{name: "colour off", flags: mode.Flags{Color: "never"}, terminal: true},
		{name: "TERM=dumb", vars: map[string]string{"TERM": "dumb"}, terminal: true},
		{name: "forced colour on a pipe", vars: map[string]string{"FORCE_COLOR": "1"}},
		{name: "always on a pipe", flags: mode.Flags{Color: "always"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := mode.Detect("demo", tt.flags, mode.Env{LookupEnv: envOf(tt.vars), StderrTerminal: tt.terminal})
			if err != nil {
				t.Fatal(err)
			}
			if m.Animated() != tt.want {
				t.Errorf("Animated = %v, want %v", m.Animated(), tt.want)
			}
		})
	}
}

func TestDebugAndQuiet(t *testing.T) {
	tests := []struct {
		name  string
		flags mode.Flags
		vars  map[string]string
		want  bool
	}{
		{name: "off"},
		{name: "flag", flags: mode.Flags{Debug: true}, want: true},
		{name: "env", vars: map[string]string{"DEMO_DEBUG": "TRUE"}, want: true},
		{name: "flag wins over a false env", flags: mode.Flags{Debug: true}, vars: map[string]string{"DEMO_DEBUG": "no"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := mode.Detect("demo", tt.flags, mode.Env{LookupEnv: envOf(tt.vars)})
			if err != nil {
				t.Fatal(err)
			}
			if m.Debug() != tt.want {
				t.Errorf("Debug = %v, want %v", m.Debug(), tt.want)
			}
		})
	}
	m, err := mode.Detect("demo", mode.Flags{Quiet: true, JSON: true}, mode.Env{})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Quiet() || !m.JSON() {
		t.Errorf("Quiet %v JSON %v, want both true", m.Quiet(), m.JSON())
	}
}

func TestDetectErrors(t *testing.T) {
	tests := []struct {
		name         string
		flags        mode.Flags
		vars         map[string]string
		code         string
		exit         int
		wantDecorate bool
	}{
		{name: "invalid colour", flags: mode.Flags{Color: "yes"}, code: "INVALID_COLOR", exit: 2, wantDecorate: true},
		{name: "invalid no-input env", vars: map[string]string{"DEMO_NO_INPUT": "maybe"}, code: "CONFIG_INVALID", exit: 78, wantDecorate: true},
		{name: "invalid debug env", vars: map[string]string{"DEMO_DEBUG": "2"}, code: "CONFIG_INVALID", exit: 78, wantDecorate: true},
		{name: "invalid debug env ignored under --debug", flags: mode.Flags{Debug: true}, vars: map[string]string{"DEMO_DEBUG": "2"}, wantDecorate: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := mode.Detect("demo", tt.flags, mode.Env{LookupEnv: envOf(tt.vars), StderrTerminal: true})
			if m.StderrDecorated() != tt.wantDecorate {
				t.Errorf("StderrDecorated = %v, want %v", m.StderrDecorated(), tt.wantDecorate)
			}
			if tt.code == "" {
				if err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				return
			}
			p := fault.Payload(err)
			if p == nil || p.Code != tt.code || p.Exit() != tt.exit || len(p.ValidValues) == 0 || p.Fix == "" {
				t.Errorf("payload = %+v, want code %s exit %d with fix and valid values", p, tt.code, tt.exit)
			}
		})
	}
	if _, err := mode.Detect("", mode.Flags{}, mode.Env{}); err == nil {
		t.Error("empty CLI name: want an error")
	}
}

func TestEnvBool(t *testing.T) {
	tests := []struct {
		raw     string
		set     bool
		want    bool
		wantErr bool
	}{
		{set: false, want: false},
		{raw: "", set: true, want: false},
		{raw: "1", set: true, want: true},
		{raw: "true", set: true, want: true},
		{raw: "Yes", set: true, want: true},
		{raw: " TRUE ", set: true, want: true},
		{raw: "0", set: true, want: false},
		{raw: "False", set: true, want: false},
		{raw: "NO", set: true, want: false},
		{raw: "on", set: true, wantErr: true},
		{raw: "2", set: true, wantErr: true},
		{raw: "ture", set: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			vars := map[string]string{}
			if tt.set {
				vars["X_FLAG"] = tt.raw
			}
			got, err := mode.Env{LookupEnv: envOf(vars)}.Bool("X_FLAG")
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("Bool(%q) = %v, %v; want %v, err %v", tt.raw, got, err, tt.want, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "X_FLAG") {
				t.Errorf("error %q does not name the variable", err)
			}
		})
	}
}

// TestColorAlwaysBeatsNoColorAtConstruction covers fatih/color stamping a
// style colourless when NO_COLOR is set as it is built.
func TestColorAlwaysBeatsNoColorAtConstruction(t *testing.T) {
	saved := color.NoColor
	t.Cleanup(func() { color.NoColor = saved })
	t.Setenv("NO_COLOR", "1")
	style := color.New(color.FgRed)
	if got := style.Sprint("x"); got != "x" {
		t.Fatalf("precondition: style built under NO_COLOR printed %q", got)
	}

	m, err := mode.Detect("demo", mode.Flags{Color: "always"}, mode.ProcessEnv())
	if err != nil {
		t.Fatal(err)
	}
	m.ApplyColor(style)
	if got := style.Sprint("x"); !strings.Contains(got, "\x1b[31m") {
		t.Errorf("--color=always printed %q, want red escape", got)
	}
	if color.NoColor {
		t.Error("ApplyColor left the package-wide switch off")
	}

	mode.Modes{}.ApplyColor(style)
	if got := style.Sprint("x"); got != "x" || !color.NoColor {
		t.Errorf("undecorated modes printed %q, NoColor %v", got, color.NoColor)
	}
}

func TestStampStylesLeavesGlobal(t *testing.T) {
	saved := color.NoColor
	t.Cleanup(func() { color.NoColor = saved })
	color.NoColor = true
	style := color.New(color.Bold)
	m, err := mode.Detect("demo", mode.Flags{Color: "always"}, mode.Env{})
	if err != nil {
		t.Fatal(err)
	}
	m.StampStyles(style)
	if !color.NoColor {
		t.Error("StampStyles changed the package-wide switch")
	}
	if got := style.Sprint("x"); !strings.Contains(got, "\x1b[1m") {
		t.Errorf("stamped style printed %q", got)
	}
}
