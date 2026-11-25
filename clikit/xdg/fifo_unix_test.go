//go:build unix

package xdg_test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSystemConfigFileSkipsFifo(t *testing.T) {
	fifoDir, realDir := t.TempDir(), t.TempDir()
	fifo := filepath.Join(fifoDir, "tk", "config.cue")
	if err := os.MkdirAll(filepath.Dir(fifo), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	want := writeConfig(t, realDir)
	env := envOf("linux", t.TempDir(), map[string]string{
		"XDG_CONFIG_DIRS": fifoDir + string(os.PathListSeparator) + realDir,
	})
	d := mustNew(t, env)

	type result struct {
		got      string
		warnings []error
		err      error
	}
	done := make(chan result, 1)
	go func() {
		var warnings []error
		got, err := d.SystemConfigFile("config.cue", func(w error) { warnings = append(warnings, w) })
		done <- result{got, warnings, err}
	}()

	var res result
	select {
	case res = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SystemConfigFile blocked on a named pipe")
	}
	if res.err != nil || res.got != want || len(res.warnings) != 1 || !strings.Contains(res.warnings[0].Error(), "not a regular file") {
		t.Errorf("got %q, %v, warnings %v; want %q after one non-regular-file warning", res.got, res.err, res.warnings, want)
	}
}
