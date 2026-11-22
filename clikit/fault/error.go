package fault

import (
	"errors"
	"fmt"
	"slices"
)

// Error is a coded error built by New. It implements every coded-error
// interface, so Payload reads all of its fields. Its With methods return a
// modified copy, so a package-level Error is safe to derive from.
type Error struct {
	code        string
	message     string
	fix         string
	validValues []string
	retryAfter  string
	exitCode    int
	cause       error
}

// New returns an error with code, message, and fix. A canonical code takes its
// exit code from the table; a domain code sets one with WithDomainExitCode.
func New(code, message, fix string) *Error {
	return &Error{code: code, message: message, fix: fix}
}

// Error returns the message.
func (e *Error) Error() string { return e.message }

// Unwrap returns the cause set by WithCause.
func (e *Error) Unwrap() error { return e.cause }

// Code returns the payload code.
func (e *Error) Code() string { return e.code }

// Fix returns the next command or action.
func (e *Error) Fix() string { return e.fix }

// ValidValues returns a copy of the accepted values, or nil when the accepted
// set is open.
func (e *Error) ValidValues() []string { return slices.Clone(e.validValues) }

// RetryAfter returns the retry hint in the duration grammar, or "".
func (e *Error) RetryAfter() string { return e.retryAfter }

// ExitCode returns the domain exit code set by WithDomainExitCode, or 0.
func (e *Error) ExitCode() int { return e.exitCode }

// WithValidValues returns a copy of e that lists the accepted values.
func (e *Error) WithValidValues(values ...string) *Error {
	c := *e
	c.validValues = slices.Clone(values)
	return &c
}

// WithRetryAfter returns a copy of e with a retry hint in the duration grammar.
func (e *Error) WithRetryAfter(d string) *Error {
	c := *e
	c.retryAfter = d
	return &c
}

// WithDomainExitCode returns a copy of e with a domain exit code. The code is
// ignored when e's code is canonical.
func (e *Error) WithDomainExitCode(code int) *Error {
	c := *e
	c.exitCode = code
	return &c
}

// WithCause returns a copy of e that wraps cause, so errors.Is and errors.As
// still reach it. Payload never reads coded fields from the cause.
func (e *Error) WithCause(cause error) *Error {
	c := *e
	c.cause = cause
	return &c
}

// exitError fixes the process exit code for the error it wraps.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err == nil {
		return fmt.Sprintf("exit status %d", e.code)
	}
	return e.err.Error()
}

func (e *exitError) Unwrap() error { return e.err }

// WithFinalExit returns an error that wraps err and pins the process exit
// code at code, which ExitCode then returns without consulting err's payload.
// Use it once an exit code is decided, such as when an error is printed or a
// signal is caught. A nil err is allowed and reads as "exit status <code>".
func WithFinalExit(code int, err error) error {
	return &exitError{code: code, err: err}
}

// printedError marks an error that has already been written for the user.
type printedError struct {
	err error
}

func (e *printedError) Error() string { return e.err.Error() }
func (e *printedError) Unwrap() error { return e.err }

// MarkPrinted returns err marked as already written, so the error boundary and
// main do not print it a second time. It returns nil for a nil err.
func MarkPrinted(err error) error {
	if err == nil {
		return nil
	}
	return &printedError{err: err}
}

// IsPrinted reports whether err, or any error in its chain, was marked by
// MarkPrinted.
func IsPrinted(err error) bool {
	_, ok := errors.AsType[*printedError](err)
	return ok
}
