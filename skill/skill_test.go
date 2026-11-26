package skill_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/p3bot/x/skill"
	"github.com/p3bot/x/skill/catalog"
)

const testText = "---\nname: tk\ndescription: Test skill.\n---\n\n# tk\n"

func newSkill(t *testing.T) skill.Skill {
	t.Helper()
	s, err := skill.New("tk", testText)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// newEnv returns a hermetic Env whose home and working directories are empty
// temp directories and in which exactly the named agents are installed.
func newEnv(t *testing.T, installed ...string) catalog.Env {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "agent")
	writeFile(t, bin, "#!/bin/sh\n", 0o755)
	paths := make(map[string]string, len(installed))
	for _, id := range installed {
		paths[id] = bin
	}
	return catalog.Env{Home: t.TempDir(), WorkingDir: t.TempDir(), BinPaths: paths, GOOS: runtime.GOOS}
}

func writeFile(t *testing.T, path, data string, perm os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), perm); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func home(env catalog.Env, rel string) string {
	return filepath.Join(env.Home, filepath.FromSlash(rel))
}

func TestNew(t *testing.T) {
	s := newSkill(t)
	if s.ID() != "tk" || s.Text() != testText {
		t.Errorf("New = (%q, %q), want (tk, %q)", s.ID(), s.Text(), testText)
	}
	s, err := skill.New("tk", strings.TrimSuffix(testText, "\n"))
	if err != nil || s.Text() != testText {
		t.Errorf("New without final newline: text %q, err %v; want %q", s.Text(), err, testText)
	}
	crlf, err := skill.New("tk", strings.ReplaceAll(testText, "\n", "\r\n"))
	if err != nil || crlf.ID() != "tk" {
		t.Errorf("New with CRLF lines: %v", err)
	}
}

func TestNewRejects(t *testing.T) {
	tests := []struct {
		name, id, text, want string
	}{
		{"name differs from id", "tk", "---\nname: webctl\n---\n", `frontmatter name is "webctl"`},
		{"no name", "tk", "---\ndescription: x\n---\n", `frontmatter name is ""`},
		{"no frontmatter", "tk", "# tk\n", "first line is not ---"},
		{"unterminated frontmatter", "tk", "---\nname: tk\n", "no closing ---"},
		{"invalid yaml", "tk", "---\nname: [tk\n---\n", "frontmatter:"},
		{"empty id", "", "---\nname: \"\"\n---\n", "not 1 to 64"},
		{"upper case id", "Tk", "---\nname: Tk\n---\n", "not 1 to 64"},
		{"path id", "../tk", "---\nname: ../tk\n---\n", "not 1 to 64"},
		{"double hyphen id", "t--k", "---\nname: t--k\n---\n", "not 1 to 64"},
		{"long id", strings.Repeat("a", 65), "---\nname: " + strings.Repeat("a", 65) + "\n---\n", "not 1 to 64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := skill.New(tt.id, tt.text)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("New(%q) error = %v, want one containing %q", tt.id, err, tt.want)
			}
		})
	}
}

func TestZeroSkillAndBadLocation(t *testing.T) {
	env := newEnv(t, "claude-code")
	var zero skill.Skill
	if _, err := zero.Install(env, skill.Global, nil); err == nil {
		t.Error("zero Skill Install succeeded")
	}
	if _, err := zero.List(env, skill.Global); err == nil {
		t.Error("zero Skill List succeeded")
	}
	if _, err := zero.Uninstall(env, skill.Global, nil); err == nil {
		t.Error("zero Skill Uninstall succeeded")
	}
	if _, err := newSkill(t).Install(env, skill.Location(7), nil); err == nil {
		t.Error("Install with an unknown Location succeeded")
	}
}

// TestNonTestImportsExcludeFault keeps codes travelling through fault's
// interfaces only: the package must not import fault outside its tests.
func TestNonTestImportsExcludeFault(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go command not found")
	}
	cmd := exec.Command(gobin, "list", "-deps", ".")
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=readonly", "GOPROXY=off")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	deps := strings.Fields(string(out))
	if slices.Contains(deps, "github.com/p3bot/x/clikit/fault") {
		t.Error("package skill imports github.com/p3bot/x/clikit/fault")
	}
	if !slices.Contains(deps, "github.com/p3bot/x/clikit/atomicfile") {
		t.Error("package skill does not import github.com/p3bot/x/clikit/atomicfile; is go list reading this package?")
	}
}
