package xdg_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/p3bot/x/clikit/fault"
	"github.com/p3bot/x/clikit/xdg"
)

func envOf(goos, home string, vars map[string]string) xdg.Env {
	return xdg.Env{
		Getenv:  func(key string) string { return vars[key] },
		HomeDir: func() (string, error) { return home, nil },
		GOOS:    goos,
	}
}

func mustNew(t *testing.T, env xdg.Env) xdg.Dirs {
	t.Helper()
	d, err := xdg.New("tk", env)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return d
}

type tierCase struct {
	name   string
	xdgEnv string
	get    func(xdg.Dirs) (string, error)
}

var tiers = []tierCase{
	{"config", "XDG_CONFIG_HOME", xdg.Dirs.Config},
	{"data", "XDG_DATA_HOME", xdg.Dirs.Data},
	{"state", "XDG_STATE_HOME", xdg.Dirs.State},
	{"cache", "XDG_CACHE_HOME", xdg.Dirs.Cache},
}

func TestPlatformDefaults(t *testing.T) {
	home := t.TempDir()
	unix := map[string]string{
		"config": filepath.Join(home, ".config", "tk"),
		"data":   filepath.Join(home, ".local", "share", "tk"),
		"state":  filepath.Join(home, ".local", "state", "tk"),
		"cache":  filepath.Join(home, ".cache", "tk"),
	}
	support := filepath.Join(home, "Library", "Application Support", "tk")
	darwin := map[string]string{
		"config": filepath.Join(support, "config"),
		"data":   filepath.Join(support, "data"),
		"state":  filepath.Join(support, "state"),
		"cache":  filepath.Join(home, "Library", "Caches", "tk"),
	}
	want := map[string]map[string]string{
		"linux":   unix,
		"darwin":  darwin,
		"windows": unix,
		"freebsd": unix,
	}
	for goos, dirs := range want {
		for _, tc := range tiers {
			got, err := tc.get(mustNew(t, envOf(goos, home, nil)))
			if err != nil || got != dirs[tc.name] {
				t.Errorf("%s %s = %q, %v; want %q", goos, tc.name, got, err, dirs[tc.name])
			}
		}
	}
}

func TestXDGVariables(t *testing.T) {
	home := t.TempDir()
	abs := filepath.Join(home, "xdg")
	tests := []struct {
		name    string
		value   string
		useBase bool
	}{
		{"absolute value is used", abs, true},
		{"relative value is ignored", "relative/path", false},
		{"dot-relative value is ignored", "./relative", false},
		{"empty value is ignored", "", false},
	}
	for _, goos := range []string{"linux", "darwin", "windows"} {
		for _, tt := range tests {
			for _, tc := range tiers {
				env := envOf(goos, home, map[string]string{tc.xdgEnv: tt.value})
				got, err := tc.get(mustNew(t, env))
				if err != nil {
					t.Fatalf("%s %s %s: %v", goos, tt.name, tc.name, err)
				}
				fallback, _ := tc.get(mustNew(t, envOf(goos, home, nil)))
				want := fallback
				if tt.useBase {
					want = filepath.Join(abs, "tk")
				}
				if got != want {
					t.Errorf("%s %s %s = %q, want %q", goos, tt.name, tc.name, got, want)
				}
			}
		}
	}
}

func TestOtherTierVariableIgnored(t *testing.T) {
	home := t.TempDir()
	env := envOf("linux", home, map[string]string{"XDG_CACHE_HOME": filepath.Join(home, "c")})
	got, err := mustNew(t, env).Config()
	if want := filepath.Join(home, ".config", "tk"); err != nil || got != want {
		t.Errorf("Config = %q, %v; want %q", got, err, want)
	}
}

func TestHomeUnavailable(t *testing.T) {
	cause := errors.New("no home")
	tests := []struct {
		name    string
		goos    string
		home    func() (string, error)
		wantFix string
	}{
		{"lookup fails", "linux", func() (string, error) { return "", cause }, "set HOME"},
		{"relative home", "darwin", func() (string, error) { return "rel/home", nil }, "set HOME"},
		{"empty home", "linux", func() (string, error) { return "", nil }, "set HOME"},
		{"windows names USERPROFILE", "windows", func() (string, error) { return "", cause }, "set USERPROFILE"},
	}
	for _, tt := range tests {
		env := xdg.Env{Getenv: func(string) string { return "" }, HomeDir: tt.home, GOOS: tt.goos}
		_, err := mustNew(t, env).State()
		if err == nil {
			t.Fatalf("%s: State succeeded, want an error", tt.name)
		}
		p := fault.Payload(err)
		if p.Code != "CONFIG_INVALID" || fault.ExitCode(err) != fault.ExitConfig {
			t.Errorf("%s: code %s exit %d, want CONFIG_INVALID exit 78", tt.name, p.Code, fault.ExitCode(err))
		}
		if !strings.Contains(p.Fix, tt.wantFix) || !strings.Contains(p.Fix, "XDG_STATE_HOME") {
			t.Errorf("%s: fix %q, want it to name %s and XDG_STATE_HOME", tt.name, p.Fix, tt.wantFix)
		}
		if !strings.Contains(p.Message, "state directory") {
			t.Errorf("%s: message %q does not name the tier", tt.name, p.Message)
		}
	}

	env := xdg.Env{Getenv: func(string) string { return "" }, HomeDir: func() (string, error) { return "", cause }, GOOS: "linux"}
	if _, err := mustNew(t, env).Config(); !errors.Is(err, cause) {
		t.Errorf("Config error %v does not wrap the home lookup error", err)
	}
}

func TestAbsoluteVariableNeedsNoHome(t *testing.T) {
	base := t.TempDir()
	env := xdg.Env{
		Getenv:  func(string) string { return base },
		HomeDir: func() (string, error) { return "", errors.New("no home") },
		GOOS:    "linux",
	}
	got, err := mustNew(t, env).Cache()
	if want := filepath.Join(base, "tk"); err != nil || got != want {
		t.Errorf("Cache = %q, %v; want %q", got, err, want)
	}
}

func TestProcessEnv(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)
	got, err := mustNew(t, xdg.ProcessEnv()).Data()
	if want := filepath.Join(base, "tk"); err != nil || got != want {
		t.Errorf("Data = %q, %v; want %q", got, err, want)
	}
}

func TestZeroEnvIsEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := mustNew(t, xdg.Env{}).Config()
	if fault.Payload(err).Code != "CONFIG_INVALID" {
		t.Fatalf("zero Env Config = %v, want CONFIG_INVALID for the unknown home", err)
	}

	base := t.TempDir()
	env := xdg.Env{Getenv: func(string) string { return base }}
	got, err := mustNew(t, env).Config()
	if want := filepath.Join(base, "tk"); err != nil || got != want {
		t.Errorf("Env with only Getenv: Config = %q, %v; want %q", got, err, want)
	}

	home := t.TempDir()
	got, err = mustNew(t, xdg.Env{HomeDir: func() (string, error) { return home, nil }}).Cache()
	if want := filepath.Join(home, ".cache", "tk"); err != nil || got != want {
		t.Errorf("Env with only HomeDir: Cache = %q, %v; want %q", got, err, want)
	}
}

func TestNewRejectsBadNames(t *testing.T) {
	for _, cli := range []string{"", ".", "..", "a/b", `a\b`, "/tk"} {
		if _, err := xdg.New(cli, xdg.Env{}); err == nil {
			t.Errorf("New(%q) succeeded, want an error", cli)
		}
	}
}

func TestZeroDirsFails(t *testing.T) {
	var d xdg.Dirs
	for _, tc := range tiers {
		if got, err := tc.get(d); err == nil {
			t.Errorf("zero Dirs %s = %q, want an error", tc.name, got)
		}
	}
	if got, err := d.SystemConfigFile("config.cue", nil); err == nil {
		t.Errorf("zero Dirs SystemConfigFile = %q, want an error", got)
	}
}

func writeConfig(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "tk", "config.cue")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func systemConfig(t *testing.T, goos, dirs string) (string, []error) {
	t.Helper()
	env := envOf(goos, t.TempDir(), map[string]string{"XDG_CONFIG_DIRS": dirs})
	var warnings []error
	got, err := mustNew(t, env).SystemConfigFile("config.cue", func(w error) { warnings = append(warnings, w) })
	if err != nil {
		t.Fatalf("SystemConfigFile: %v", err)
	}
	return got, warnings
}

func TestSystemConfigFileName(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "shadow"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "tk", "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "tk", "config.cue"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "tk", "sub", "config.cue"), []byte("sub"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := envOf("linux", t.TempDir(), map[string]string{"XDG_CONFIG_DIRS": base})
	d := mustNew(t, env)

	for _, name := range []string{"../shadow", "", "/etc/passwd", "a/../../x"} {
		got, err := d.SystemConfigFile(name, nil)
		if err == nil || got != "" {
			t.Errorf("SystemConfigFile(%q) = %q, %v; want an error and an empty path", name, got, err)
		}
	}

	got, err := d.SystemConfigFile("config.cue", nil)
	if want := filepath.Join(base, "tk", "config.cue"); err != nil || got != want {
		t.Errorf("SystemConfigFile(config.cue) = %q, %v; want %q", got, err, want)
	}
	got, err = d.SystemConfigFile("sub/config.cue", nil)
	if want := filepath.Join(base, "tk", "sub", "config.cue"); err != nil || got != want {
		t.Errorf("SystemConfigFile(sub/config.cue) = %q, %v; want %q", got, err, want)
	}
}

func TestSystemConfigFile(t *testing.T) {
	empty, first, second := t.TempDir(), t.TempDir(), t.TempDir()
	firstFile := writeConfig(t, first)
	secondFile := writeConfig(t, second)
	list := func(dirs ...string) string { return strings.Join(dirs, string(os.PathListSeparator)) }

	tests := []struct {
		name string
		dirs string
		want string
	}{
		{"first match wins", list(first, second), firstFile},
		{"missing candidates are skipped", list(empty, second), secondFile},
		{"relative entries are ignored", list("relative", second), secondFile},
		{"empty entries are ignored", list("", second), secondFile},
		{"no match", list(empty), ""},
	}
	for _, goos := range []string{"linux", "darwin"} {
		for _, tt := range tests {
			got, warnings := systemConfig(t, goos, tt.dirs)
			if got != tt.want || len(warnings) != 0 {
				t.Errorf("%s %s = %q, warnings %v; want %q and none", goos, tt.name, got, warnings, tt.want)
			}
		}
	}
}

func TestSystemConfigFileDefaults(t *testing.T) {
	if got, warnings := systemConfig(t, "darwin", ""); got != "" || len(warnings) != 0 {
		t.Errorf("darwin with XDG_CONFIG_DIRS unset = %q, %v; want nothing searched", got, warnings)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if _, err := os.Stat("/etc/xdg/tk/config.cue"); err == nil {
		t.Skip("/etc/xdg/tk/config.cue exists on this host")
	}
	// Proving /etc/xdg is searched needs a file there; this pins only that the
	// default lookup is clean when it holds nothing for the CLI.
	if got, warnings := systemConfig(t, "linux", ""); got != "" || len(warnings) != 0 {
		t.Errorf("linux with XDG_CONFIG_DIRS unset = %q, %v; want nothing found", got, warnings)
	}
}

func TestSystemConfigFileWarnsAndSkips(t *testing.T) {
	isDir, second := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(isDir, "tk", "config.cue"), 0o700); err != nil {
		t.Fatal(err)
	}
	secondFile := writeConfig(t, second)
	got, warnings := systemConfig(t, "linux", isDir+string(os.PathListSeparator)+second)
	if got != secondFile || len(warnings) != 1 || !strings.Contains(warnings[0].Error(), "not a regular file") {
		t.Errorf("got %q, warnings %v; want %q after one non-regular-file warning", got, warnings, secondFile)
	}
}

func TestSystemConfigFileUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows modes cannot make a file unreadable")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads files regardless of mode")
	}
	locked, second := t.TempDir(), t.TempDir()
	if err := os.Chmod(writeConfig(t, locked), 0); err != nil {
		t.Fatal(err)
	}
	secondFile := writeConfig(t, second)
	got, warnings := systemConfig(t, "linux", locked+string(os.PathListSeparator)+second)
	if got != secondFile || len(warnings) != 1 || !errors.Is(warnings[0], fs.ErrPermission) {
		t.Errorf("got %q, warnings %v; want %q after one permission warning", got, warnings, secondFile)
	}
}

func TestSystemConfigFileNilWarn(t *testing.T) {
	isDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(isDir, "tk", "config.cue"), 0o700); err != nil {
		t.Fatal(err)
	}
	env := envOf("linux", t.TempDir(), map[string]string{"XDG_CONFIG_DIRS": isDir})
	if got, err := mustNew(t, env).SystemConfigFile("config.cue", nil); got != "" || err != nil {
		t.Errorf("SystemConfigFile with nil warn = %q, %v", got, err)
	}
}

func TestEnsure(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "a", "b")
	if err := xdg.Ensure(dir); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if err := xdg.Ensure(dir); err != nil {
		t.Fatalf("Ensure on an existing directory: %v", err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	for _, p := range []string{filepath.Join(root, "a"), dir} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o700 {
			t.Errorf("%s: mode %v, want 0700", p, fi.Mode().Perm())
		}
	}

	existing := filepath.Join(root, "open")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := xdg.Ensure(existing); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(existing)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("existing directory mode changed to %v", fi.Mode().Perm())
	}
}
