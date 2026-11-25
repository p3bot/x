package atomicfile_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/p3bot/x/clikit/atomicfile"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// assertOnly fails unless dir holds exactly the named entries, so a leftover
// temp file shows up.
func assertOnly(t *testing.T, dir string, names ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if len(got) != len(names) {
		t.Fatalf("%s holds %q, want %q", dir, got, names)
	}
	for i := range names {
		if got[i] != names[i] {
			t.Fatalf("%s holds %q, want %q", dir, got, names)
		}
	}
}

func TestWriteCreates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := atomicfile.Write(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := readFile(t, path); got != "hello\n" {
		t.Errorf("content = %q", got)
	}
	assertOnly(t, dir, "f.txt")
}

func TestWriteReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := atomicfile.Write(path, []byte("first, and longer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := atomicfile.Write(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "second" {
		t.Errorf("content = %q, want %q", got, "second")
	}
	assertOnly(t, dir, "f")
}

func TestWriteRelativePath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := atomicfile.Write("rel", []byte("x"), 0o600); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "rel")); got != "x" {
		t.Errorf("content = %q", got)
	}
	assertOnly(t, dir, "rel")
}

func TestWriteLongName(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{"ascii", strings.Repeat("a", 255)},
		// A cut at a fixed even byte count would split one of these characters.
		{"multibyte", "a" + strings.Repeat("é", 127)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(t.TempDir(), tt.file), nil, 0o600); err != nil {
				t.Skipf("filesystem rejects a %d-byte name: %v", len(tt.file), err)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, tt.file)
			if err := atomicfile.Write(path, []byte("x"), 0o600); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if got := readFile(t, path); got != "x" {
				t.Errorf("content = %q", got)
			}
			assertOnly(t, dir, tt.file)
		})
	}
}

func TestWriteMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows modes only toggle read-only")
	}
	dir := t.TempDir()
	for _, perm := range []fs.FileMode{0o644, 0o600, 0o755} {
		path := filepath.Join(dir, perm.String())
		// The temp file starts at 0600, so 0644 and 0755 prove the mode is set
		// explicitly rather than inherited.
		if err := atomicfile.Write(path, []byte("x"), perm); err != nil {
			t.Fatal(err)
		}
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != perm {
			t.Errorf("mode = %v, want %v", fi.Mode().Perm(), perm)
		}
	}
}

func TestWriteMissingParent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing", "f")
	err := atomicfile.Write(path, []byte("x"), 0o600)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Write = %v, want an error matching fs.ErrNotExist", err)
	}
	assertOnly(t, dir)
}

func TestWriteRenameFailureCleansUp(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "keep"), []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := atomicfile.Write(target, []byte("x"), 0o600); err == nil {
		t.Fatal("Write over a non-empty directory succeeded, want an error")
	}
	assertOnly(t, dir, "target")
	assertOnly(t, target, "keep")
}

func TestWriteThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	targetDir := filepath.Join(dir, "dotfiles")
	if err := os.Mkdir(targetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(targetDir, "config")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config")
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Fatal(err)
	}

	if err := atomicfile.Write(link, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&fs.ModeSymlink == 0 {
		t.Fatalf("%s is no longer a symlink (mode %v)", link, fi.Mode())
	}
	if got := readFile(t, target); got != "new" {
		t.Fatalf("target holds %q, want %q", got, "new")
	}
	assertOnly(t, targetDir, "config")
	assertOnly(t, dir, "config", "dotfiles")
}

func TestWriteDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	link := filepath.Join(dir, "config")
	if err := os.Symlink(filepath.Join(missing, "config"), link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Fatal(err)
	}

	if err := atomicfile.Write(link, []byte("new"), 0o600); err != nil {
		t.Fatalf("Write: %v", err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.Mode().IsRegular() {
		t.Fatalf("%s has mode %v, want a regular file in place of the link", link, fi.Mode())
	}
	if got := readFile(t, link); got != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
	if _, err := os.Lstat(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Lstat(%s) = %v, want it still missing", missing, err)
	}
	assertOnly(t, dir, "config")
}
