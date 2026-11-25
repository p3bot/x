//go:build unix

package interrupt_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/p3bot/x/clikit/fault"
	"github.com/p3bot/x/clikit/interrupt"
)

// These tests signal the test process itself, so none may run in parallel:
// a signal sent outside Run would take its default action and end the run.

func signalSelf(t *testing.T, sig syscall.Signal) {
	t.Helper()
	if err := syscall.Kill(os.Getpid(), sig); err != nil {
		t.Fatalf("send %v: %v", sig, err)
	}
}

func waitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("context not cancelled within 10s of the signal")
	}
}

func TestRunSignal(t *testing.T) {
	tests := []struct {
		name     string
		sig      syscall.Signal
		ret      error
		wantExit int
	}{
		{"SIGINT, work returns nil", syscall.SIGINT, nil, 130},
		{"SIGTERM, work returns nil", syscall.SIGTERM, nil, 143},
		{"SIGINT, work returns ctx.Err", syscall.SIGINT, context.Canceled, 130},
		{"SIGTERM, work returns a coded error", syscall.SIGTERM, fault.New("WAIT_TIMEOUT", "gave up", "retry"), 143},
	}
	for _, tt := range tests {
		var cause error
		err := interrupt.Run(func(ctx context.Context) error {
			derived, cancel := context.WithTimeout(ctx, time.Hour)
			defer cancel()
			signalSelf(t, tt.sig)
			waitDone(t, derived.Done())
			cause = context.Cause(derived)
			return tt.ret
		})

		var fromCause *interrupt.Error
		if !errors.As(cause, &fromCause) || fromCause.Signal != tt.sig {
			t.Errorf("%s: cause on derived context = %v, want *interrupt.Error for %v", tt.name, cause, tt.sig)
		}
		var returned *interrupt.Error
		if !errors.As(err, &returned) || returned.Signal != tt.sig {
			t.Errorf("%s: Run = %v, want *interrupt.Error for %v", tt.name, err, tt.sig)
		}
		if got := fault.ExitCode(err); got != tt.wantExit {
			t.Errorf("%s: exit %d, want %d", tt.name, got, tt.wantExit)
		}
	}
}

const helperEnv = "CLIKIT_INTERRUPT_HELPER"

// TestSecondSignalKills runs Run in a child process whose work ignores
// cancellation, and checks that the second SIGTERM ends the child.
func TestSecondSignalKills(t *testing.T) {
	if os.Getenv(helperEnv) == "1" {
		secondSignalHelper()
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSecondSignalKills$")
	cmd.Env = append(os.Environ(), helperEnv+"=1")
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	lines := bufio.NewScanner(stdout)
	next := func(want string) {
		t.Helper()
		if !lines.Scan() {
			t.Fatalf("child ended before %q: %v", want, lines.Err())
		}
		switch got := lines.Text(); got {
		case want:
		case "ignored":
			t.Skip("SIGTERM is ignored in this process tree")
		default:
			t.Fatalf("child printed %q, want %q", got, want)
		}
	}

	next("ready")
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	next("cancelled")
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("child Wait = %v, want it killed by SIGTERM", err)
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGTERM {
		t.Fatalf("child ended with %v, want killed by SIGTERM", exitErr)
	}
}

func secondSignalHelper() {
	if signal.Ignored(syscall.SIGTERM) {
		fmt.Println("ignored")
		os.Exit(0)
	}
	err := interrupt.Run(func(ctx context.Context) error {
		fmt.Println("ready")
		<-ctx.Done()
		fmt.Println("cancelled")
		time.Sleep(time.Minute)
		return nil
	})
	fmt.Println("survived:", err)
	os.Exit(0)
}
