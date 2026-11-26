package skill_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/p3bot/x/skill"
)

func TestList(t *testing.T) {
	env := newEnv(t, "claude-code", "codex", "amp")
	s := newSkill(t)
	for _, rel := range []string{".agents/skills", ".claude/skills", ".config/amp/skills"} {
		writeFile(t, filepath.Join(home(env, rel), "tk", "SKILL.md"), testText, 0o644)
	}
	// Not a candidate root of any installed agent.
	writeFile(t, home(env, ".hermes/skills/tk/SKILL.md"), testText, 0o644)
	// A directory where SKILL.md should be is not a copy.
	if err := os.MkdirAll(home(env, ".codex/skills/tk/SKILL.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := s.List(env, skill.Global)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []skill.Copy{
		{Path: home(env, ".agents/skills/tk/SKILL.md"), Agents: []string{"amp", "codex"}},
		{Path: home(env, ".claude/skills/tk/SKILL.md"), Agents: []string{"claude-code"}},
		{Path: home(env, ".config/amp/skills/tk/SKILL.md"), Agents: []string{"amp"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("List = %+v, want %+v", got, want)
	}
}

func TestListEmpty(t *testing.T) {
	for name, installed := range map[string][]string{"no copies": {"claude-code"}, "no agents": nil} {
		t.Run(name, func(t *testing.T) {
			got, err := newSkill(t).List(newEnv(t, installed...), skill.Global)
			if err != nil || len(got) != 0 {
				t.Errorf("List = %+v, %v; want empty and nil", got, err)
			}
		})
	}
}
