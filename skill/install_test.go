package skill_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/p3bot/x/clikit/fault"
	"github.com/p3bot/x/skill"
)

func TestInstallDefaultSet(t *testing.T) {
	env := newEnv(t, "claude-code", "codex", "amp")
	written, err := newSkill(t).Install(env, skill.Global, nil)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	// codex and amp share their primary root, the shared agents root.
	want := []string{
		home(env, ".agents/skills/tk/SKILL.md"),
		home(env, ".claude/skills/tk/SKILL.md"),
	}
	if !slices.Equal(written, want) {
		t.Fatalf("written = %q, want %q", written, want)
	}
	for _, path := range want {
		if got := readFile(t, path); got != testText {
			t.Errorf("%s = %q, want %q", path, got, testText)
		}
		if runtime.GOOS == "windows" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode().Perm() != 0o644 {
			t.Errorf("%s mode = %v, want 0644", path, info.Mode().Perm())
		}
	}
}

func TestInstallNamed(t *testing.T) {
	env := newEnv(t)
	// Named agents resolve uninstalled: codex at its native root, opencode,
	// which has no native global root, at the shared agents root.
	written, err := newSkill(t).Install(env, skill.Global, []string{"opencode", "codex", "codex"})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	want := []string{
		home(env, ".agents/skills/tk/SKILL.md"),
		home(env, ".codex/skills/tk/SKILL.md"),
	}
	if !slices.Equal(written, want) {
		t.Errorf("written = %q, want %q", written, want)
	}
}

func TestInstallLocal(t *testing.T) {
	env := newEnv(t, "claude-code")
	written, err := newSkill(t).Install(env, skill.Local, nil)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	want := []string{filepath.Join(env.WorkingDir, ".claude", "skills", "tk", "SKILL.md")}
	if !slices.Equal(written, want) {
		t.Errorf("written = %q, want %q", written, want)
	}
}

func TestInstallReplacesOwnedCopy(t *testing.T) {
	env := newEnv(t, "claude-code")
	dir := home(env, ".claude/skills/tk")
	// An edited copy that still names tk is owned, and other files beside it
	// do not block install.
	writeFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: tk\n---\nold\n", 0o644)
	writeFile(t, filepath.Join(dir, "notes.md"), "mine\n", 0o644)
	if _, err := newSkill(t).Install(env, skill.Global, nil); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "SKILL.md")); got != testText {
		t.Errorf("SKILL.md = %q, want %q", got, testText)
	}
	if got := readFile(t, filepath.Join(dir, "notes.md")); got != "mine\n" {
		t.Errorf("notes.md = %q, want it unchanged", got)
	}
}

func TestInstallIntoDirWithoutSkillFile(t *testing.T) {
	env := newEnv(t, "claude-code")
	dir := home(env, ".claude/skills/tk")
	writeFile(t, filepath.Join(dir, "notes.md"), "mine\n", 0o644)
	if _, err := newSkill(t).Install(env, skill.Global, nil); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "SKILL.md")); got != testText {
		t.Errorf("SKILL.md = %q, want %q", got, testText)
	}
}

func TestInstallForeignCopyWritesNothing(t *testing.T) {
	tests := []struct {
		name  string
		plant func(t *testing.T, dir string)
	}{
		{"other name", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: webctl\n---\n", 0o644)
		}},
		{"unreadable frontmatter", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "SKILL.md"), "# no frontmatter\n", 0o644)
		}},
		{"skill dir is a file", func(t *testing.T, dir string) {
			writeFile(t, dir, "a file\n", 0o644)
		}},
		{"SKILL.md is a directory", func(t *testing.T, dir string) {
			if err := os.MkdirAll(filepath.Join(dir, "SKILL.md"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newEnv(t, "claude-code", "codex")
			// The foreign copy sits under the root sorted last, so a check
			// interleaved with writes would already have written the first.
			tt.plant(t, home(env, ".claude/skills/tk"))
			written, err := newSkill(t).Install(env, skill.Global, nil)
			assertPayload(t, err, skill.ErrForeignCopy, "CONFLICT", fault.ExitConflict,
				"inspect the copy named in the message, remove it if unwanted, and rerun", nil)
			if written != nil {
				t.Errorf("written = %q, want nil", written)
			}
			if _, err := os.Lstat(home(env, ".agents/skills/tk")); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("Install wrote under the clean root: %v", err)
			}
		})
	}
}

func TestInstallSymlinkedSkillFileIsForeign(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	env := newEnv(t, "claude-code")
	target := filepath.Join(t.TempDir(), "SKILL.md")
	const owned = "---\nname: tk\n---\ndotfiles copy\n"
	writeFile(t, target, owned, 0o644)
	dir := home(env, ".claude/skills/tk")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	_, err := newSkill(t).Install(env, skill.Global, nil)
	assertPayload(t, err, skill.ErrForeignCopy, "CONFLICT", fault.ExitConflict,
		"inspect the copy named in the message, remove it if unwanted, and rerun", nil)
	if got := readFile(t, target); got != owned {
		t.Errorf("link target = %q, want it unchanged", got)
	}
}

func TestInstallReadOnlyRootIsNoSkillsRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory modes do not refuse writes on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	env := newEnv(t, "claude-code")
	root := home(env, ".claude/skills")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	_, err := newSkill(t).Install(env, skill.Global, nil)
	assertPayload(t, err, skill.ErrNoSkillsRoot, "NO_SKILLS_ROOT", fault.ExitUsage,
		"tk skill > <agent-skills-dir>/tk/SKILL.md", nil)
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("error %v does not wrap the permission error", err)
	}
}

func TestInstallNoRootAtLocation(t *testing.T) {
	// hermes has global skills roots only.
	t.Run("named", func(t *testing.T) {
		_, err := newSkill(t).Install(newEnv(t), skill.Local, []string{"hermes"})
		assertPayload(t, err, skill.ErrNoSkillsRoot, "NO_SKILLS_ROOT", fault.ExitUsage,
			"tk skill > <agent-skills-dir>/tk/SKILL.md", nil)
	})
	t.Run("default set", func(t *testing.T) {
		_, err := newSkill(t).Install(newEnv(t, "hermes"), skill.Local, nil)
		assertPayload(t, err, skill.ErrNoSkillsRoot, "NO_SKILLS_ROOT", fault.ExitUsage,
			"tk skill > <agent-skills-dir>/tk/SKILL.md", nil)
	})
}

func TestInstallSymlinkedSkillDirIsForeign(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	const owned = "---\nname: tk\n---\ndotfiles copy\n"
	tests := []struct {
		name  string
		plant func(t *testing.T, target string)
	}{
		{"owned copy behind the link", func(t *testing.T, target string) {
			writeFile(t, filepath.Join(target, "SKILL.md"), owned, 0o644)
		}},
		{"no SKILL.md behind the link", func(t *testing.T, target string) {
			if err := os.MkdirAll(target, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{"dangling link", func(*testing.T, string) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newEnv(t, "claude-code", "codex")
			target := filepath.Join(t.TempDir(), "tk")
			tt.plant(t, target)
			// The link sits under the root sorted last, so a check
			// interleaved with writes would already have written the first.
			dir := home(env, ".claude/skills/tk")
			if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, dir); err != nil {
				t.Fatal(err)
			}
			written, err := newSkill(t).Install(env, skill.Global, nil)
			assertPayload(t, err, skill.ErrForeignCopy, "CONFLICT", fault.ExitConflict,
				"inspect the copy named in the message, remove it if unwanted, and rerun", nil)
			if written != nil {
				t.Errorf("written = %q, want nil", written)
			}
			if _, err := os.Lstat(home(env, ".agents/skills/tk")); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("Install wrote under the clean root: %v", err)
			}
			file := filepath.Join(target, "SKILL.md")
			data, err := os.ReadFile(file)
			switch tt.name {
			case "owned copy behind the link":
				if err != nil || string(data) != owned {
					t.Errorf("link target = %q (err %v), want it unchanged", data, err)
				}
			default:
				if !errors.Is(err, os.ErrNotExist) {
					t.Errorf("Install wrote through the link: %v", err)
				}
			}
		})
	}
}

func TestInstallUnreadableSkillFilePassesThrough(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes do not refuse reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	env := newEnv(t, "claude-code", "codex")
	// The unreadable copy sits under the root sorted last, so a check
	// interleaved with writes would already have written the first.
	file := home(env, ".claude/skills/tk/SKILL.md")
	writeFile(t, file, testText, 0o000)
	t.Cleanup(func() { _ = os.Chmod(file, 0o644) })
	written, err := newSkill(t).Install(env, skill.Global, nil)
	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) || pathErr.Path != file {
		t.Fatalf("error = %v, want the *fs.PathError for %s", err, file)
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("error %v does not match os.ErrPermission", err)
	}
	if errors.Is(err, skill.ErrNoSkillsRoot) {
		t.Errorf("error %v is ErrNoSkillsRoot, want it unchanged", err)
	}
	if p := fault.Payload(err); p.Code != "UNKNOWN" {
		t.Errorf("code = %q, want UNKNOWN", p.Code)
	}
	if written != nil {
		t.Errorf("written = %q, want nil", written)
	}
	if _, err := os.Lstat(home(env, ".agents/skills/tk")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Install wrote under the clean root: %v", err)
	}
}

func TestInstallWriteFailureReturnsWrittenPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory modes do not refuse writes on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	env := newEnv(t, "claude-code", "codex")
	// The root sorted last has no skill directory, so it passes the check and
	// refuses the write after the first root is written.
	root := home(env, ".claude/skills")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	written, err := newSkill(t).Install(env, skill.Global, nil)
	assertPayload(t, err, skill.ErrNoSkillsRoot, "NO_SKILLS_ROOT", fault.ExitUsage,
		"tk skill > <agent-skills-dir>/tk/SKILL.md", nil)
	want := []string{home(env, ".agents/skills/tk/SKILL.md")}
	if !slices.Equal(written, want) {
		t.Fatalf("written = %q, want %q", written, want)
	}
	if got := readFile(t, want[0]); got != testText {
		t.Errorf("%s = %q, want %q", want[0], got, testText)
	}
}
