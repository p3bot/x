package skill_test

import (
	"errors"
	"os"
	"runtime"
	"slices"
	"testing"

	"github.com/p3bot/x/clikit/fault"
	"github.com/p3bot/x/skill"
	"github.com/p3bot/x/skill/catalog"
)

// assertPayload checks err through fault's payload derivation, as a CLI's
// error boundary reads it, so a drift in the coded-error contract fails here.
func assertPayload(t *testing.T, err, sentinel error, code string, exit int, fix string, valid []string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", code)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(%v, %v) = false", err, sentinel)
	}
	p := fault.Payload(err)
	if p.Code != code {
		t.Errorf("code = %q, want %q (error %v)", p.Code, code, err)
	}
	if got := fault.ExitCode(err); got != exit {
		t.Errorf("exit code = %d, want %d", got, exit)
	}
	if p.Fix != fix {
		t.Errorf("fix = %q, want %q", p.Fix, fix)
	}
	if !slices.Equal(p.ValidValues, valid) {
		t.Errorf("valid values = %q, want %q", p.ValidValues, valid)
	}
	if p.Message != err.Error() || p.Message == "" {
		t.Errorf("message = %q, want the error text %q", p.Message, err.Error())
	}
}

func catalogIDs() []string {
	var ids []string
	for _, a := range catalog.Agents() {
		ids = append(ids, a.ID)
	}
	return ids
}

func TestAgentSelectionErrors(t *testing.T) {
	const manual = "tk skill > <agent-skills-dir>/tk/SKILL.md"
	install := func(env catalog.Env, agents ...string) error {
		_, err := newSkill(t).Install(env, skill.Global, agents)
		return err
	}
	uninstall := func(env catalog.Env, agents ...string) error {
		_, err := newSkill(t).Uninstall(env, skill.Global, agents)
		return err
	}
	tests := []struct {
		name     string
		err      error
		sentinel error
		code     string
		fix      string
		valid    []string
	}{
		{"install unknown agent", install(newEnv(t), "codex", "nope"), skill.ErrUnknownAgent,
			"AGENT_UNKNOWN", "tk skill install <agent>", catalogIDs()},
		{"uninstall unknown agent", uninstall(newEnv(t), "nope"), skill.ErrUnknownAgent,
			"AGENT_UNKNOWN", "tk skill uninstall <agent>", catalogIDs()},
		{"install no skills concept", install(newEnv(t), "aider"), skill.ErrNoSkillsConcept,
			"NO_SKILLS_CONCEPT", manual, nil},
		{"uninstall no skills concept", uninstall(newEnv(t), "aider"), skill.ErrNoSkillsConcept,
			"NO_SKILLS_CONCEPT", manual, nil},
		// aider is installed but has no skills concept, so the set is empty.
		{"install no agents", install(newEnv(t, "aider")), skill.ErrNoAgents, "NO_AGENTS", manual, nil},
		{"uninstall no agents", uninstall(newEnv(t)), skill.ErrNoAgents, "NO_AGENTS", manual, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertPayload(t, tt.err, tt.sentinel, tt.code, fault.ExitUsage, tt.fix, tt.valid)
		})
	}
}

func TestOtherErrorsPassThrough(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory modes do not refuse reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	env := newEnv(t, "claude-code")
	dir := home(env, ".claude/skills/tk")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	_, err := newSkill(t).Uninstall(env, skill.Global, nil)
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || pathErr.Path != dir {
		t.Fatalf("Uninstall error = %v, want the *os.PathError for %s", err, dir)
	}
	if code := fault.Payload(err).Code; code != fault.CodeUnknown {
		t.Errorf("code = %q, want %q", code, fault.CodeUnknown)
	}
}
