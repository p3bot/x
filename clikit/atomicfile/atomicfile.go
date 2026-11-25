// Package atomicfile replaces a file's contents so that readers see either the
// old file or the new one, never a partial write.
//
// The data goes to a temp file in the destination directory, so the final
// rename stays on one filesystem. After the rename the directory is synced,
// so a nil return means the new contents survive a crash. On Windows the
// rename fails while another process holds the destination open, the mode
// only toggles read-only, and the directory sync is skipped because a
// directory cannot be opened for it there.
package atomicfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"unicode/utf8"
)

// Write writes data to path with mode perm, replacing any existing file.
//
// perm is applied to the temp file explicitly, so the umask does not narrow
// it. Write never creates directories: a missing parent fails with an error
// matching fs.ErrNotExist. A nil return means the new contents are durable.
// A failure before the rename removes the temp file and leaves an existing
// file at path unchanged. A failure to sync the directory after the rename
// (reported as "sync dir") means the new file is already in place but the
// replacement may not survive a crash.
//
// When path is a symlink, Write follows it and replaces the file it resolves
// to, so a link into a dotfiles checkout survives and its target gets the new
// contents. A missing path or a dangling link is written at path itself.
func Write(path string, data []byte, perm fs.FileMode) (err error) {
	dest, err := destination(path)
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	dir, base := filepath.Split(dest)
	if dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, "."+tempStem(base)+".*.tmp")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			if rmErr := os.Remove(tmp.Name()); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
				err = fmt.Errorf("%w (temp file %s left behind: %w)", err, tmp.Name(), rmErr)
			}
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("write %s: set mode: %w", path, err)
	}
	// Without the sync, a crash after the rename can leave an empty file on
	// filesystems that delay allocation.
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("write %s: sync: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: close: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	// The rename changed dir, the directory holding dest; until dir is
	// synced a crash can undo it. The deferred cleanup's Remove of the temp
	// name now reports not-exist, which it ignores, so dest is never touched.
	if err := syncDir(dir); err != nil {
		return fmt.Errorf("write %s: sync dir: %w", path, err)
	}
	return nil
}

// maxTempStem bounds, in bytes, how much of the destination name a temp name
// repeats. The temp name adds its own dots, random digits, and suffix, so
// repeating a long name whole would pass the usual 255-byte limit.
const maxTempStem = 200

// tempStem returns base cut to at most maxTempStem bytes without splitting a
// UTF-8 character.
func tempStem(base string) string {
	if len(base) <= maxTempStem {
		return base
	}
	n := maxTempStem
	for n > 0 && !utf8.RuneStart(base[n]) {
		n--
	}
	return base[:n]
}

// syncDir flushes dir's entries to stable storage. Windows cannot open a
// directory for sync, so it is a no-op there.
func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	if closeErr := d.Close(); err == nil {
		err = closeErr
	}
	return err
}

// destination returns the file Write replaces: path itself, or the target it
// resolves to when path is a symlink. The temp file goes beside that target,
// so the rename stays on one filesystem and lands on the real file.
func destination(path string) (string, error) {
	fi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", err
	}
	if fi.Mode()&fs.ModeSymlink == 0 {
		return path, nil
	}
	target, err := filepath.EvalSymlinks(path)
	if errors.Is(err, fs.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", err
	}
	return target, nil
}
