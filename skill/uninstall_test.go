package skill_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
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

func TestUninstallKeepsSymlinkedSkillDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	const reason = "symbolic link, not a directory"
	link := func(t *testing.T, target, dir string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, dir); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("link outside every root", func(t *testing.T) {
		env := newEnv(t, "claude-code")
		target := filepath.Join(t.TempDir(), "tk")
		writeFile(t, filepath.Join(target, "SKILL.md"), testText, 0o644)
		dir := home(env, ".claude/skills/tk")
		link(t, target, dir)
		got, err := newSkill(t).Uninstall(env, skill.Global, nil)
		if err != nil {
			t.Fatalf("Uninstall: %v", err)
		}
		want := []skill.Removal{{Outcome: skill.Kept, Dir: dir, Reason: reason}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Uninstall = %+v, want %+v", got, want)
		}
		if got := readFile(t, filepath.Join(target, "SKILL.md")); got != testText {
			t.Errorf("link target = %q, want it unchanged", got)
		}
		if _, err := os.Lstat(dir); err != nil {
			t.Errorf("link was removed: %v", err)
		}
	})

	t.Run("link into another agent's claimed copy", func(t *testing.T) {
		env := newEnv(t, "claude-code", "codex")
		s := newSkill(t)
		// codex claims the shared agents root, which opencode installs into.
		shared := home(env, ".agents/skills/tk")
		if _, err := s.Install(env, skill.Global, []string{"opencode"}); err != nil {
			t.Fatal(err)
		}
		dir := home(env, ".claude/skills/tk")
		link(t, shared, dir)
		got, err := s.Uninstall(env, skill.Global, []string{"claude-code"})
		if err != nil {
			t.Fatalf("Uninstall: %v", err)
		}
		want := []skill.Removal{{Outcome: skill.Kept, Dir: dir, Reason: reason}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Uninstall = %+v, want %+v", got, want)
		}
		if got := readFile(t, filepath.Join(shared, "SKILL.md")); got != testText {
			t.Errorf("claimed copy = %q, want it unchanged", got)
		}
		if _, err := os.Lstat(dir); err != nil {
			t.Errorf("link was removed: %v", err)
		}
	})

	t.Run("dangling link", func(t *testing.T) {
		env := newEnv(t, "claude-code")
		dir := home(env, ".claude/skills/tk")
		link(t, filepath.Join(t.TempDir(), "gone"), dir)
		got, err := newSkill(t).Uninstall(env, skill.Global, nil)
		if err != nil {
			t.Fatalf("Uninstall: %v", err)
		}
		want := []skill.Removal{{Outcome: skill.Kept, Dir: dir, Reason: reason}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Uninstall = %+v, want %+v", got, want)
		}
		if _, err := os.Lstat(dir); err != nil {
			t.Errorf("link was removed: %v", err)
		}
	})
}

func TestUninstallKeepsClaimedRootReachedThroughLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	// claude-code's skills root is a link to the shared agents root, which
	// codex claims.
	env := newEnv(t, "claude-code", "codex")
	shared := home(env, ".agents/skills")
	linked := home(env, ".claude/skills")
	for _, dir := range []string{shared, filepath.Dir(linked)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(shared, linked); err != nil {
		t.Fatal(err)
	}
	s := newSkill(t)
	if _, err := s.Install(env, skill.Global, nil); err != nil {
		t.Fatal(err)
	}
	got, err := s.Uninstall(env, skill.Global, []string{"claude-code"})
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	want := []skill.Removal{
		{Outcome: skill.Kept, Dir: filepath.Join(linked, "tk"), Reason: "claimed by codex"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Uninstall = %+v, want %+v", got, want)
	}
	if got := readFile(t, filepath.Join(shared, "tk", "SKILL.md")); got != testText {
		t.Errorf("claimed copy = %q, want it unchanged", got)
	}
	if info, err := os.Lstat(linked); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("skills root link is gone: %v", err)
	}
}

func TestUninstallNamesEachClaimantOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	// codex's native root is a link to the shared agents root, so codex
	// claims that directory by two paths.
	env := newEnv(t, "claude-code", "codex", "amp")
	shared := home(env, ".agents/skills")
	native := home(env, ".codex/skills")
	for _, dir := range []string{shared, filepath.Dir(native)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(shared, native); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(shared, "tk", "SKILL.md"), testText, 0o644)
	got, err := newSkill(t).Uninstall(env, skill.Global, []string{"opencode"})
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	want := skill.Removal{Outcome: skill.Kept, Dir: filepath.Join(shared, "tk"), Reason: "claimed by amp,codex"}
	if len(got) == 0 || got[0] != want {
		t.Errorf("Uninstall = %+v, want first %+v", got, want)
	}
}

func TestUninstallReadOnlyRootKeepsCopyForRetry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory modes do not refuse writes on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	env := newEnv(t, "claude-code")
	root := home(env, ".claude/skills")
	dir := filepath.Join(root, "tk")
	file := filepath.Join(dir, "SKILL.md")
	// An edited copy, so a restore from the embedded text would show.
	const edited = "---\nname: tk\n---\nedited\n"
	writeFile(t, file, edited, 0o644)
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	s := newSkill(t)
	got, err := s.Uninstall(env, skill.Global, nil)
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || pathErr.Path != dir || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Uninstall error = %v, want the permission error for %s", err, dir)
	}
	if len(got) != 0 {
		t.Errorf("Uninstall = %+v, want no removals", got)
	}
	if got := readFile(t, file); got != edited {
		t.Errorf("SKILL.md = %q, want it restored", got)
	}
	if info, err := os.Stat(file); err != nil {
		t.Errorf("stat SKILL.md: %v", err)
	} else if info.Mode().Perm() != 0o644 {
		t.Errorf("SKILL.md mode = %v, want 0644", info.Mode().Perm())
	}

	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err = s.Uninstall(env, skill.Global, nil)
	if err != nil {
		t.Fatalf("retried Uninstall: %v", err)
	}
	want := []skill.Removal{{Outcome: skill.Removed, Dir: dir}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("retried Uninstall = %+v, want %+v", got, want)
	}
}

func TestUninstallErrorReturnsRemovalsMade(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory modes do not refuse reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	env := newEnv(t, "claude-code", "codex")
	s := newSkill(t)
	if _, err := s.Install(env, skill.Global, nil); err != nil {
		t.Fatal(err)
	}
	// The directory under the second root cannot be listed.
	dir := home(env, ".claude/skills/tk")
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	got, err := s.Uninstall(env, skill.Global, nil)
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || pathErr.Path != dir {
		t.Fatalf("Uninstall error = %v, want the *os.PathError for %s", err, dir)
	}
	want := []skill.Removal{{Outcome: skill.Removed, Dir: home(env, ".agents/skills/tk")}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Uninstall = %+v, want %+v", got, want)
	}
}

func TestLocalRoundTrip(t *testing.T) {
	env := newEnv(t, "claude-code")
	s := newSkill(t)
	// A global copy, which no local operation may report or remove.
	global, err := s.Install(env, skill.Global, nil)
	if err != nil {
		t.Fatalf("global Install: %v", err)
	}
	dir := filepath.Join(env.WorkingDir, ".claude", "skills", "tk")
	file := filepath.Join(dir, "SKILL.md")
	written, err := s.Install(env, skill.Local, nil)
	if err != nil || !slices.Equal(written, []string{file}) {
		t.Fatalf("Install = %q, %v; want %q", written, err, file)
	}
	copies, err := s.List(env, skill.Local)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	wantCopies := []skill.Copy{{Path: file, Agents: []string{"claude-code"}}}
	if !reflect.DeepEqual(copies, wantCopies) {
		t.Errorf("List = %+v, want %+v", copies, wantCopies)
	}
	got, err := s.Uninstall(env, skill.Local, nil)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	want := []skill.Removal{{Outcome: skill.Removed, Dir: dir}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Uninstall = %+v, want %+v", got, want)
	}
	if _, err := os.Lstat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s still exists: %v", dir, err)
	}
	for _, path := range global {
		if got := readFile(t, path); got != testText {
			t.Errorf("%s = %q, want it unchanged", path, got)
		}
	}
}
