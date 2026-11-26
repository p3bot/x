package catalog_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/p3bot/x/skill/catalog"
)

var testAgent = catalog.Agent{
	ID:     "tester",
	Name:   "Tester",
	Bin:    "tester",
	Config: catalog.Config{Global: "~/.tester", Local: ".tester"},
	Skills: catalog.Skills{
		Global: catalog.Scope{Native: "~/.tester/skills", Alternatives: []string{"~/.agents/skills"}},
		Local:  catalog.Scope{Alternatives: []string{".claude/skills", ".tester/skills"}},
	},
	Provider: []string{"p"},
}

func TestDetectResolvesPaths(t *testing.T) {
	home, wd := t.TempDir(), t.TempDir()
	mkdir(t, home, ".tester")
	mkdir(t, home, ".agents/skills")
	mkdir(t, wd, ".tester/skills")

	got := catalog.Detect(catalog.Env{Home: home, WorkingDir: wd}, testAgent)

	wantConfig := catalog.ResolvedConfig{
		Global: catalog.PathEntry{Path: filepath.Join(home, ".tester"), Exists: true},
		Local:  catalog.PathEntry{Path: filepath.Join(wd, ".tester"), Exists: true},
	}
	if got.Config != wantConfig {
		t.Errorf("Config = %+v, want %+v", got.Config, wantConfig)
	}
	wantSkills := catalog.ResolvedSkills{
		Global: catalog.ResolvedScope{
			Native:       catalog.PathEntry{Path: filepath.Join(home, ".tester/skills")},
			Alternatives: []catalog.PathEntry{{Path: filepath.Join(home, ".agents/skills"), Exists: true}},
			Primary:      catalog.PathEntry{Path: filepath.Join(home, ".tester/skills")},
		},
		Local: catalog.ResolvedScope{
			Alternatives: []catalog.PathEntry{
				{Path: filepath.Join(wd, ".claude/skills")},
				{Path: filepath.Join(wd, ".tester/skills"), Exists: true},
			},
			Primary: catalog.PathEntry{Path: filepath.Join(wd, ".claude/skills")},
		},
	}
	if !reflect.DeepEqual(got.Skills, wantSkills) {
		t.Errorf("Skills = %+v, want %+v", got.Skills, wantSkills)
	}
	if !reflect.DeepEqual(got.Agent, testAgent) {
		t.Errorf("Agent = %+v, want %+v", got.Agent, testAgent)
	}
}

func TestDetectZeroEnvIsHermetic(t *testing.T) {
	got := catalog.Detect(catalog.Env{}, testAgent)
	want := catalog.Detection{
		Agent: testAgent,
		Skills: catalog.ResolvedSkills{
			Global: catalog.ResolvedScope{Alternatives: []catalog.PathEntry{{}}},
			Local:  catalog.ResolvedScope{Alternatives: []catalog.PathEntry{{}, {}}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Detect(Env{}) = %+v, want %+v", got, want)
	}
}

func TestDetectRelativeDirsCountAsUnset(t *testing.T) {
	got := catalog.Detect(catalog.Env{Home: "home", WorkingDir: "wd"}, testAgent)
	if got.Config != (catalog.ResolvedConfig{}) {
		t.Errorf("Config = %+v, want zero for relative Home and WorkingDir", got.Config)
	}
}

func TestDetectBinary(t *testing.T) {
	wd := t.TempDir()
	search := mkdir(t, wd, "search")
	other := mkdir(t, wd, "other")
	searchBin := writeFile(t, search, "tester", 0o755)
	nonExec := writeFile(t, other, "tester", 0o644)
	lookupBin := writeFile(t, mkdir(t, wd, "lookup"), "tester", 0o755)
	mkdir(t, wd, "dir/tester")
	errNotFound := errors.New("not found")

	tests := []struct {
		name string
		env  catalog.Env
		want catalog.Binary
	}{
		{"zero env finds nothing", catalog.Env{}, catalog.Binary{}},
		{"lookup hit", catalog.Env{LookPath: returns(lookupBin, nil), SearchDirs: []string{search}},
			catalog.Binary{Path: lookupBin, Found: true}},
		{"found via a search directory", catalog.Env{SearchDirs: []string{search}},
			catalog.Binary{Path: searchBin, Found: true}},
		{"search directories in order", catalog.Env{SearchDirs: []string{other, filepath.Join(wd, "dir"), search}},
			catalog.Binary{Path: searchBin, Found: true}},
		{"relative search directory joins the working directory", catalog.Env{WorkingDir: wd, SearchDirs: []string{"search"}},
			catalog.Binary{Path: searchBin, Found: true}},
		{"relative search directory without a working directory", catalog.Env{SearchDirs: []string{"search"}},
			catalog.Binary{}},
		{"lookup error falls through", catalog.Env{LookPath: returns("", errNotFound), SearchDirs: []string{search}},
			catalog.Binary{Path: searchBin, Found: true}},
		{"non-executable lookup hit falls through", catalog.Env{LookPath: returns(nonExec, nil), SearchDirs: []string{search}},
			catalog.Binary{Path: searchBin, Found: true}},
		{"missing lookup hit falls through", catalog.Env{LookPath: returns(filepath.Join(wd, "gone"), nil), SearchDirs: []string{search}},
			catalog.Binary{Path: searchBin, Found: true}},
		{"relative lookup hit joins the working directory", catalog.Env{WorkingDir: wd, LookPath: returns("lookup/tester", nil)},
			catalog.Binary{Path: lookupBin, Found: true}},
		{"empty search directory is ignored", catalog.Env{WorkingDir: search, SearchDirs: []string{""}}, catalog.Binary{}},
		{"dot searches the working directory", catalog.Env{WorkingDir: search, SearchDirs: []string{"."}},
			catalog.Binary{Path: searchBin, Found: true}},
		{"directory is not a binary", catalog.Env{SearchDirs: []string{filepath.Join(wd, "dir")}}, catalog.Binary{}},
		{"override found", catalog.Env{WorkingDir: wd, BinPaths: map[string]string{"tester": "lookup/tester"}},
			catalog.Binary{Path: lookupBin, Found: true}},
		{"override not found does not fall back", catalog.Env{
			LookPath:   returns(lookupBin, nil),
			SearchDirs: []string{search},
			BinPaths:   map[string]string{"tester": filepath.Join(wd, "gone")},
		}, catalog.Binary{}},
		{"non-executable override does not fall back", catalog.Env{
			SearchDirs: []string{search},
			BinPaths:   map[string]string{"tester": nonExec},
		}, catalog.Binary{}},
		{"override for another agent is ignored", catalog.Env{
			SearchDirs: []string{search},
			BinPaths:   map[string]string{"other": filepath.Join(wd, "gone")},
		}, catalog.Binary{Path: searchBin, Found: true}},
		{"empty override is no override", catalog.Env{
			SearchDirs: []string{search},
			BinPaths:   map[string]string{"tester": ""},
		}, catalog.Binary{Path: searchBin, Found: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := catalog.Detect(tt.env, testAgent).Binary; got != tt.want {
				t.Errorf("Binary = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDetectBinaryWindowsRules(t *testing.T) {
	dir := t.TempDir()
	exe := writeFile(t, dir, "claude.exe", 0o644)
	both := mkdir(t, dir, "both")
	com := writeFile(t, both, "claude.com", 0o644)
	writeFile(t, both, "claude.exe", 0o644)
	batOnly := mkdir(t, dir, "bat")
	bat := writeFile(t, batOnly, "claude.bat", 0o644)
	cmdOnly := mkdir(t, dir, "cmd")
	cmd := writeFile(t, cmdOnly, "claude.cmd", 0o644)
	exeBat := mkdir(t, dir, "exe-bat")
	exeOverBat := writeFile(t, exeBat, "claude.exe", 0o644)
	writeFile(t, exeBat, "claude.bat", 0o644)
	batCmd := mkdir(t, dir, "bat-cmd")
	batOverCmd := writeFile(t, batCmd, "claude.bat", 0o644)
	writeFile(t, batCmd, "claude.cmd", 0o644)

	tests := []struct {
		name string
		goos string
		bin  string
		dirs []string
		want catalog.Binary
	}{
		{"suffix added to a bare name", "windows", "claude", []string{dir}, catalog.Binary{Path: exe, Found: true}},
		{"name with an extension tried as given", "windows", "claude.exe", []string{dir}, catalog.Binary{Path: exe, Found: true}},
		{".com before .exe", "windows", "claude", []string{both}, catalog.Binary{Path: com, Found: true}},
		{".exe before .bat", "windows", "claude", []string{exeBat}, catalog.Binary{Path: exeOverBat, Found: true}},
		{".bat tried", "windows", "claude", []string{batOnly}, catalog.Binary{Path: bat, Found: true}},
		{".bat before .cmd", "windows", "claude", []string{batCmd}, catalog.Binary{Path: batOverCmd, Found: true}},
		{".cmd tried", "windows", "claude", []string{cmdOnly}, catalog.Binary{Path: cmd, Found: true}},
		{"no suffixes off windows", "linux", "claude", []string{dir}, catalog.Binary{}},
		{"execute bit required off windows", "", "claude.exe", []string{dir}, catalog.Binary{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := testAgent
			a.Bin = tt.bin
			got := catalog.Detect(catalog.Env{GOOS: tt.goos, SearchDirs: tt.dirs}, a).Binary
			if got != tt.want {
				t.Errorf("Binary = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDetectAll(t *testing.T) {
	got := catalog.DetectAll(catalog.Env{})
	if len(got) != len(wantIDs) {
		t.Fatalf("DetectAll returned %d agents, want %d", len(got), len(wantIDs))
	}
	for i, d := range got {
		if d.Agent.ID != wantIDs[i] {
			t.Errorf("DetectAll()[%d].Agent.ID = %q, want %q", i, d.Agent.ID, wantIDs[i])
		}
	}
}

func TestProcessEnv(t *testing.T) {
	env, err := catalog.ProcessEnv()
	if err != nil {
		t.Fatalf("ProcessEnv: %v", err)
	}
	home, _ := os.UserHomeDir()
	wd, _ := os.Getwd()
	if env.Home != home || env.WorkingDir != wd || env.GOOS != runtime.GOOS || env.LookPath == nil {
		t.Errorf("ProcessEnv() = %+v, want home %q, working directory %q, GOOS %q, and a lookup", env, home, wd, runtime.GOOS)
	}
	if _, err := exec.LookPath("go"); err == nil {
		if _, err := env.LookPath("go"); err != nil {
			t.Errorf("ProcessEnv().LookPath(\"go\"): %v", err)
		}
	}
}

func TestProcessEnvUnreadableHome(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
		t.Skip("os.UserHomeDir does not read HOME")
	}
	t.Setenv("HOME", "")
	env, err := catalog.ProcessEnv()
	if err == nil || !strings.Contains(err.Error(), "home directory") {
		t.Errorf("ProcessEnv error = %v, want one naming the home directory", err)
	}
	wd, _ := os.Getwd()
	if env.Home != "" || env.WorkingDir != wd || env.GOOS != runtime.GOOS || env.LookPath == nil {
		t.Errorf("ProcessEnv() = %+v, want no home, working directory %q, GOOS %q, and a lookup", env, wd, runtime.GOOS)
	}
}

func returns(p string, err error) func(string) (string, error) {
	return func(string) (string, error) { return p, err }
}

func mkdir(t *testing.T, base, rel string) string {
	t.Helper()
	p := filepath.Join(base, filepath.FromSlash(rel))
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeFile(t *testing.T, dir, name string, perm os.FileMode) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, nil, perm); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, perm); err != nil { // WriteFile's mode is masked by umask
		t.Fatal(err)
	}
	return p
}
