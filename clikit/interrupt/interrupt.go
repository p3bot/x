// Package interrupt runs a CLI invocation under a context that SIGINT or
// SIGTERM cancels, and turns a caught signal into the 128+signum exit code.
//
// main calls Run once and hands its result to fault.ExitCode, which stays the
// single exit decision:
//
//	func main() {
//		os.Exit(fault.ExitCode(interrupt.Run(cli.Execute)))
//	}
//
// The first signal cancels the context and restores the default action, so a
// second signal ends the process even when work ignores cancellation. On
// Windows, Ctrl-C and Ctrl-Break arrive as SIGINT and exit 130. Closing the
// console, logging off, and shutting down arrive as SIGTERM, which cancels
// the context, but Windows still ends the process after a short timeout, so
// cleanup must be quick.
package interrupt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/p3bot/x/clikit/fault"
)

// Error reports that a signal interrupted the run. It is the cancellation
// cause of the context Run passes to work, so code inside work detects an
// interrupt on any derived context with errors.As(context.Cause(ctx), &e).
type Error struct {
	Signal syscall.Signal
}

func (e *Error) Error() string {
	return fmt.Sprintf("interrupted by signal %d (%s)", int(e.Signal), e.Signal)
}

// Run calls work with a context cancelled on the first SIGINT or SIGTERM.
//
// When a signal was caught, Run returns its *Error with the exit code pinned
// at 128+signum by fault.WithFinalExit, whatever work returned, nil included.
// Otherwise it returns work's error unchanged. Signal handling is process-wide,
// so Run must not be called while another call is in progress.
func Run(work func(ctx context.Context) error) error {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		select {
		case s := <-sigs:
			// Stopping before cancel guarantees that once work sees the
			// cancellation, a second signal takes the default action.
			signal.Stop(sigs)
			cancel(newError(s))
		case <-done:
		}
	}()

	err := work(ctx)
	signal.Stop(sigs)
	close(done)
	<-watched
	// A signal delivered before Stop but not taken by the watcher still counts.
	select {
	case s := <-sigs:
		cancel(newError(s))
	default:
	}

	if e, ok := errors.AsType[*Error](context.Cause(ctx)); ok {
		return fault.WithFinalExit(128+int(e.Signal), e)
	}
	return err
}

func newError(s os.Signal) *Error {
	// Notify delivers only the signals registered above, which are
	// syscall.Signal values on every supported platform.
	sig, _ := s.(syscall.Signal)
	return &Error{Signal: sig}
}
