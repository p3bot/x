package catalog

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Env is the process state detection reads. The zero Env is hermetic: no
// lookup finds a binary, and with no home or working directory every path
// that depends on one resolves empty. Detection reads the filesystem only at
// the paths Env yields.
type Env struct {
	// Home and WorkingDir are absolute; any other value counts as unset.
	// Home replaces a leading ~ and WorkingDir roots relative paths.
	Home       string
	WorkingDir string
	// LookPath finds a binary by name, as exec.LookPath does. Nil finds
	// nothing.
	LookPath func(file string) (string, error)
	// SearchDirs are tried in order after LookPath. Empty entries are
	// ignored; "." searches the working directory.
	SearchDirs []string
	// BinPaths maps an agent ID to its binary. An override is the only
	// candidate for that agent: when it does not qualify the binary is not
	// found. An empty value is no override.
	BinPaths map[string]string
	// GOOS is a runtime.GOOS value selecting the binary rules. On windows no
	// execute bit is needed, and a search-directory name without an extension
	// is tried with .com, .exe, .bat, and .cmd. Any other value, empty
	// included, requires an execute bit and tries the name as given.
	GOOS string
}

// ProcessEnv returns the Env of the running process, without search
// directories or overrides. When the home or working directory cannot be
// read the error says which, and that field is empty in the returned Env,
// which remains usable.
func ProcessEnv() (Env, error) {
	env := Env{LookPath: exec.LookPath, GOOS: runtime.GOOS}
	var errs []error
	home, err := os.UserHomeDir()
	if err != nil {
		errs = append(errs, fmt.Errorf("home directory: %w", err))
	}
	env.Home = home
	wd, err := os.Getwd()
	if err != nil {
		errs = append(errs, fmt.Errorf("working directory: %w", err))
	}
	env.WorkingDir = wd
	return env, errors.Join(errs...)
}

// Detection is what detecting one agent found. Paths resolve the same
// whether or not the binary is found. Detection never runs the binary.
type Detection struct {
	Agent  Agent
	Binary Binary
	Config ResolvedConfig
	Skills ResolvedSkills
}

// Binary is where an agent's binary was found. Path is absolute when Found
// and empty otherwise.
type Binary struct {
	Path  string
	Found bool
}

// PathEntry is one catalog path resolved to an absolute path, and whether
// anything exists there. Path is empty when the catalog has no such path or
// the Env lacks the directory it depends on.
type PathEntry struct {
	Path   string
	Exists bool
}

// ResolvedConfig is Config resolved against an Env.
type ResolvedConfig struct {
	Global PathEntry
	Local  PathEntry
}

// ResolvedSkills is Skills resolved against an Env.
type ResolvedSkills struct {
	Global ResolvedScope
	Local  ResolvedScope
}

// ResolvedScope is a Scope resolved against an Env, with its primary root.
type ResolvedScope struct {
	Agents       PathEntry
	Native       PathEntry
	Alternatives []PathEntry
	Primary      PathEntry
}

// DetectAll detects every catalog agent, sorted by ID.
func DetectAll(env Env) []Detection {
	agents := Agents()
	out := make([]Detection, len(agents))
	for i, a := range agents {
		out[i] = Detect(env, a)
	}
	return out
}

// Detect detects a in env. The binary is the BinPaths override when one is
// set; otherwise the LookPath result, then the first qualifying name in
// SearchDirs. A candidate qualifies when it is a file that is not a
// directory and, unless GOOS is windows, has an execute bit.
func Detect(env Env, a Agent) Detection {
	return Detection{
		Agent:  a,
		Binary: env.binary(a),
		Config: ResolvedConfig{
			Global: env.entry(a.Config.Global),
			Local:  env.entry(a.Config.Local),
		},
		Skills: ResolvedSkills{
			Global: env.scope(a.Skills.Global),
			Local:  env.scope(a.Skills.Local),
		},
	}
}

func (e Env) binary(a Agent) Binary {
	if override := e.BinPaths[a.ID]; override != "" {
		return e.candidate(override)
	}
	if e.LookPath != nil {
		// A lookup error means not found; the search directories still apply.
		if p, err := e.LookPath(a.Bin); err == nil {
			if b := e.candidate(p); b.Found {
				return b
			}
		}
	}
	for _, dir := range e.SearchDirs {
		// Joining an empty entry would search the working directory.
		if dir == "" {
			continue
		}
		for _, name := range e.binNames(a.Bin) {
			if b := e.candidate(filepath.Join(dir, name)); b.Found {
				return b
			}
		}
	}
	return Binary{}
}

// binNames follows Go's exec.LookPath default when PATHEXT is unset.
func (e Env) binNames(bin string) []string {
	if e.GOOS != "windows" || filepath.Ext(bin) != "" {
		return []string{bin}
	}
	return []string{bin + ".com", bin + ".exe", bin + ".bat", bin + ".cmd"}
}

func (e Env) candidate(p string) Binary {
	p = e.rooted(p)
	if p == "" {
		return Binary{}
	}
	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		return Binary{}
	}
	if e.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return Binary{}
	}
	return Binary{Path: p, Found: true}
}

func (e Env) scope(s Scope) ResolvedScope {
	r := ResolvedScope{
		Agents:  e.entry(s.Agents),
		Native:  e.entry(s.Native),
		Primary: e.entry(s.Primary()),
	}
	for _, alt := range s.Alternatives {
		r.Alternatives = append(r.Alternatives, e.entry(alt))
	}
	return r
}

func (e Env) entry(raw string) PathEntry {
	p := e.resolve(raw)
	if p == "" {
		return PathEntry{}
	}
	_, err := os.Stat(p)
	return PathEntry{Path: p, Exists: err == nil}
}

// resolve expands a catalog path. Catalog paths are slash-separated, so ~ is
// recognised only as "~" or a "~/" prefix.
func (e Env) resolve(raw string) string {
	switch {
	case raw == "":
		return ""
	case raw == "~":
		return absOrEmpty(e.Home)
	case strings.HasPrefix(raw, "~/"):
		home := absOrEmpty(e.Home)
		if home == "" {
			return ""
		}
		return filepath.Join(home, filepath.FromSlash(raw[len("~/"):]))
	}
	return e.rooted(filepath.FromSlash(raw))
}

// rooted returns p cleaned when absolute, joined to WorkingDir when relative,
// and empty when relative with no usable WorkingDir.
func (e Env) rooted(p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	wd := absOrEmpty(e.WorkingDir)
	if wd == "" {
		return ""
	}
	return filepath.Join(wd, p)
}

func absOrEmpty(dir string) string {
	if !filepath.IsAbs(dir) {
		return ""
	}
	return filepath.Clean(dir)
}
