package skill_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/p3bot/x/skill"
)

func TestUninstallDefaultSet(t *testing.T) {
	env := newEnv(t, "claude-code", "codex")
	s := newSkill(t)
	if _, err := s.Install(env, skill.Global, []string{"codex"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Install(env, skill.Global, nil); err != nil {
		t.Fatal(err)
	}
	got, err := s.Uninstall(env, skill.Global, nil)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	want := []skill.Removal{
		{Outcome: skill.Removed, Dir: home(env, ".agents/skills/tk")},
		{Outcome: skill.Removed, Dir: home(env, ".claude/skills/tk")},
		{Outcome: skill.Removed, Dir: home(env, ".codex/skills/tk")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Uninstall = %+v, want %+v", got, want)
	}
	for _, r := range want {
		if _, err := os.Lstat(r.Dir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still exists: %v", r.Dir, err)
		}
	}

	// A second run finds nothing, so uninstall is safe to retry.
	got, err = s.Uninstall(env, skill.Global, nil)
	if err != nil {
		t.Fatalf("second Uninstall: %v", err)
	}
	for i := range want {
		want[i].Outcome = skill.Absent
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("second Uninstall = %+v, want %+v", got, want)
	}
}

func TestUninstallKeepsWhatItDoesNotOwn(t *testing.T) {
	tests := []struct {
		name   string
		plant  func(t *testing.T, dir string)
		reason string
	}{
		{"extra file", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "SKILL.md"), testText, 0o644)
			writeFile(t, filepath.Join(dir, "notes.md"), "mine\n", 0o644)
		}, "directory does not hold SKILL.md alone"},
		{"empty directory", func(t *testing.T, dir string) {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}, "directory does not hold SKILL.md alone"},
		{"other name", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: webctl\n---\n", 0o644)
		}, `frontmatter name is "webctl", not "tk"`},
		{"unreadable frontmatter", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "SKILL.md"), "# tk\n", 0o644)
		}, "unreadable frontmatter: first line is not ---"},
		{"not a directory", func(t *testing.T, dir string) {
			writeFile(t, dir, testText, 0o644)
		}, "not a directory"},
		{"SKILL.md is a directory", func(t *testing.T, dir string) {
			if err := os.MkdirAll(filepath.Join(dir, "SKILL.md"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, "SKILL.md is not a regular file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newEnv(t, "claude-code")
			dir := home(env, ".claude/skills/tk")
			tt.plant(t, dir)
			got, err := newSkill(t).Uninstall(env, skill.Global, nil)
			if err != nil {
				t.Fatalf("Uninstall: %v", err)
			}
			want := []skill.Removal{{Outcome: skill.Kept, Dir: dir, Reason: tt.reason}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Uninstall = %+v, want %+v", got, want)
			}
			if _, err := os.Lstat(dir); err != nil {
				t.Errorf("%s was removed: %v", dir, err)
			}
		})
	}
}

func TestUninstallKeepsRootClaimedOutsideTargets(t *testing.T) {
	// codex and amp share the agents root; claude-code also has a copy at
	// its own root, which no target claims.
	env := newEnv(t, "codex", "amp", "claude-code")
	s := newSkill(t)
	if _, err := s.Install(env, skill.Global, nil); err != nil {
		t.Fatal(err)
	}
	got, err := s.Uninstall(env, skill.Global, []string{"codex"})
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	want := []skill.Removal{
		{Outcome: skill.Kept, Dir: home(env, ".agents/skills/tk"), Reason: "claimed by amp"},
		{Outcome: skill.Absent, Dir: home(env, ".codex/skills/tk")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Uninstall = %+v, want %+v", got, want)
	}
	if got := readFile(t, home(env, ".agents/skills/tk/SKILL.md")); got != testText {
		t.Errorf("claimed copy = %q, want it unchanged", got)
	}

	// With every claimant named, the shared root is removed.
	got, err = s.Uninstall(env, skill.Global, []string{"codex", "amp"})
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	want = []skill.Removal{
		{Outcome: skill.Removed, Dir: home(env, ".agents/skills/tk")},
		{Outcome: skill.Absent, Dir: home(env, ".codex/skills/tk")},
		{Outcome: skill.Absent, Dir: home(env, ".config/amp/skills/tk")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Uninstall = %+v, want %+v", got, want)
	}
}

func TestUninstallClaimedRootWithoutCopyIsAbsent(t *testing.T) {
	env := newEnv(t, "codex", "amp")
	got, err := newSkill(t).Uninstall(env, skill.Global, []string{"codex"})
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	want := []skill.Removal{
		{Outcome: skill.Absent, Dir: home(env, ".agents/skills/tk")},
		{Outcome: skill.Absent, Dir: home(env, ".codex/skills/tk")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Uninstall = %+v, want %+v", got, want)
	}
}
