package interrupt_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/p3bot/x/clikit/fault"
	"github.com/p3bot/x/clikit/interrupt"
)

func TestRunWithoutSignal(t *testing.T) {
	coded := fault.New("NOT_FOUND", "no such ticket", "tk list")
	tests := []struct {
		name     string
		ret      error
		wantExit int
	}{
		{"success", nil, fault.ExitSuccess},
		{"coded failure", coded, fault.ExitNotFound},
		{"plain failure", errors.New("boom"), fault.ExitFailure},
	}
	for _, tt := range tests {
		var sawLive bool
		err := interrupt.Run(func(ctx context.Context) error {
			sawLive = ctx.Err() == nil
			return tt.ret
		})
		if !sawLive {
			t.Errorf("%s: work got a cancelled context", tt.name)
		}
		if !errors.Is(err, tt.ret) || fmt.Sprintf("%T", err) != fmt.Sprintf("%T", tt.ret) {
			t.Errorf("%s: Run = %#v, want work's error %#v unchanged", tt.name, err, tt.ret)
		}
		if got := fault.ExitCode(err); got != tt.wantExit {
			t.Errorf("%s: exit %d, want %d", tt.name, got, tt.wantExit)
		}
	}
}

func TestRunCancelsWhenWorkReturns(t *testing.T) {
	var kept context.Context
	if err := interrupt.Run(func(ctx context.Context) error {
		kept = ctx
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if kept.Err() == nil {
		t.Error("context still live after Run returned")
	}
	if _, ok := errors.AsType[*interrupt.Error](context.Cause(kept)); ok {
		t.Error("context cause is an interrupt although no signal arrived")
	}
}

func TestErrorMessage(t *testing.T) {
	e := &interrupt.Error{Signal: 2}
	if got, want := e.Error(), "interrupted by signal 2 ("+e.Signal.String()+")"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
