// Package xdg locates a CLI's per-user config, data, state, and cache
// directories and its system config file, following the XDG Base Directory
// Specification with native defaults on macOS.
//
// An XDG variable counts only when it holds an absolute path; unset, empty,
// and relative values fall back to the platform default, as the
// specification requires. The defaults are:
//
//	tier    macOS                                         every other OS, Windows included
//	config  ~/Library/Application Support/<cli>/config    ~/.config/<cli>
//	data    ~/Library/Application Support/<cli>/data      ~/.local/share/<cli>
//	state   ~/Library/Application Support/<cli>/state     ~/.local/state/<cli>
//	cache   ~/Library/Caches/<cli>                        ~/.cache/<cli>
//
// os.UserConfigDir and os.UserCacheDir are not used because they ignore XDG
// variables on macOS, fail on a relative value instead of ignoring it, and
// have no data or state tier.
package xdg

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/p3bot/x/clikit/fault"
)

// Env is the process state that directory resolution reads. The zero Env has
// no variables set, no home directory, and a non-macOS OS, so a test states
// exactly what it needs and a forgotten field fails instead of reaching the
// real home directory.
type Env struct {
	Getenv  func(key string) string // nil: every variable is unset
	HomeDir func() (string, error)  // nil: the home directory is unknown
	GOOS    string                  // a runtime.GOOS value; "" uses the non-macOS defaults
}

// ProcessEnv returns the Env of the running process.
func ProcessEnv() Env {
	return Env{Getenv: os.Getenv, HomeDir: os.UserHomeDir, GOOS: runtime.GOOS}
}

var errNoHome = errors.New("home directory is unknown")

func (e Env) getenv(key string) string {
	if e.Getenv == nil {
		return ""
	}
	return e.Getenv(key)
}

func (e Env) homeDir() (string, error) {
	if e.HomeDir == nil {
		return "", errNoHome
	}
	return e.HomeDir()
}

type tier struct {
	name      string
	xdgEnv    string
	unixRel   string
	darwinRel string
	// darwinSub keeps tiers apart where macOS defaults share a base.
	darwinSub string
}

var (
	configTier = tier{"config", "XDG_CONFIG_HOME", ".config", "Library/Application Support", "config"}
	dataTier   = tier{"data", "XDG_DATA_HOME", ".local/share", "Library/Application Support", "data"}
	stateTier  = tier{"state", "XDG_STATE_HOME", ".local/state", "Library/Application Support", "state"}
	cacheTier  = tier{"cache", "XDG_CACHE_HOME", ".cache", "Library/Caches", ""}
)

// Dirs resolves the directories of one named CLI. Build it with New; the
// zero Dirs names no CLI and every method returns an error.
//
// Resolution reads Env on each call and creates nothing; call Ensure before
// writing into a directory.
type Dirs struct {
	cli string
	env Env
}

// New returns the Dirs of the CLI named cli, which must be a single path
// element such as "tk".
func New(cli string, env Env) (Dirs, error) {
	if cli == "" || cli == "." || cli == ".." || strings.ContainsAny(cli, `/\`) {
		return Dirs{}, fmt.Errorf("xdg: CLI name %q is not a single path element", cli)
	}
	return Dirs{cli: cli, env: env}, nil
}

var errNoCLI = errors.New("xdg: Dirs was not created by New")

// Config returns the config directory, for user-edited settings.
func (d Dirs) Config() (string, error) { return d.dir(configTier) }

// Data returns the data directory, for user data that is not config.
func (d Dirs) Data() (string, error) { return d.dir(dataTier) }

// State returns the state directory, for runtime history such as logs.
func (d Dirs) State() (string, error) { return d.dir(stateTier) }

// Cache returns the cache directory, for data that can be regenerated.
func (d Dirs) Cache() (string, error) { return d.dir(cacheTier) }

func (d Dirs) dir(t tier) (string, error) {
	if d.cli == "" {
		return "", errNoCLI
	}
	if base := d.env.getenv(t.xdgEnv); filepath.IsAbs(base) {
		return filepath.Join(base, d.cli), nil
	}
	home, err := d.home(t)
	if err != nil {
		return "", err
	}
	if d.env.GOOS == "darwin" {
		return filepath.Join(home, filepath.FromSlash(t.darwinRel), d.cli, t.darwinSub), nil
	}
	return filepath.Join(home, filepath.FromSlash(t.unixRel), d.cli), nil
}

// home returns the user's home directory, which must be absolute so a
// default never resolves against the working directory.
func (d Dirs) home(t tier) (string, error) {
	home, err := d.env.homeDir()
	if err == nil && !filepath.IsAbs(home) {
		err = fmt.Errorf("home directory %q is not an absolute path", home)
	}
	if err == nil {
		return home, nil
	}
	homeVar := "HOME"
	if d.env.GOOS == "windows" {
		homeVar = "USERPROFILE"
	}
	return "", fault.New(fault.CodeConfigInvalid,
		fmt.Sprintf("cannot locate the %s directory: %v", t.name, err),
		fmt.Sprintf("set %s to your home directory, or set %s to an absolute path", homeVar, t.xdgEnv),
	).WithCause(err)
}

// SystemConfigFile returns the first <dir>/<cli>/<name> that can be opened
// across $XDG_CONFIG_DIRS, or "" when there is none; later directories are
// never merged. Relative entries are ignored. An unset or empty
// $XDG_CONFIG_DIRS means /etc/xdg, except on macOS, where only an explicit
// value is searched; on Windows /etc/xdg is not absolute, so only an explicit
// value is searched there too.
//
// A missing candidate is skipped silently. A candidate that exists but cannot
// be read, or is not a regular file, is skipped and reported to warn, which
// may be nil; the CLI decides how to show it.
//
// name must be a local path, as reported by filepath.IsLocal, so the candidate
// stays inside <dir>/<cli>. Anything else returns an error.
func (d Dirs) SystemConfigFile(name string, warn func(error)) (string, error) {
	if d.cli == "" {
		return "", errNoCLI
	}
	if !filepath.IsLocal(name) {
		return "", fmt.Errorf("xdg: config file name %q is not a relative path inside the CLI directory", name)
	}
	if warn == nil {
		warn = func(error) {}
	}
	for _, dir := range filepath.SplitList(d.env.configDirs()) {
		if !filepath.IsAbs(dir) {
			continue
		}
		p := filepath.Join(dir, d.cli, name)
		if err := readableFile(p); err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				warn(fmt.Errorf("skipping system config: %w", err))
			}
			continue
		}
		return p, nil
	}
	return "", nil
}

// configDirs returns the directory list SystemConfigFile searches:
// $XDG_CONFIG_DIRS, or the OS default when it is unset or empty, which is
// /etc/xdg everywhere but macOS, where there is none.
func (e Env) configDirs() string {
	dirs := e.getenv("XDG_CONFIG_DIRS")
	if dirs == "" && e.GOOS != "darwin" {
		return "/etc/xdg"
	}
	return dirs
}

func readableFile(path string) error {
	// Stat follows a symlink and does not block on a pipe, unlike Open.
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return f.Close()
}

// Ensure creates dir and any missing parents with mode 0700, and leaves the
// mode of a directory that already exists unchanged.
func Ensure(dir string) error {
	return os.MkdirAll(dir, 0o700)
}
