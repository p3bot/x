// Package fault holds the p3bot CLI error contract: process exit codes, the
// error and warning payloads, the canonical code table, and the coded-error
// interfaces any package can implement to describe its errors.
//
// The coded-error interfaces (Coder, Fixer, ValidValuer, RetryAfterer, and
// ExitCoder) are frozen from clikit/v0.1.0, v0 included. A method is never
// renamed, retyped, or merged into another interface, and a new payload
// property arrives as a new one-method interface. Implementers outside this
// module satisfy them structurally, so any change would silently stop their
// errors matching instead of failing a build.
package fault

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// Process exit codes, per the p3bot CLI exit-code taxonomy. A run ended by a
// signal exits 128+signum instead.
const (
	ExitSuccess    = 0  // Continue
	ExitFailure    = 1  // General failure; do not retry blindly
	ExitUsage      = 2  // Bad arguments; fix and retry
	ExitNotFound   = 3  // Resource not found
	ExitPermission = 4  // Permission denied; authenticate or escalate
	ExitConflict   = 5  // Resource state prevents the operation
	ExitTransient  = 75 // Retry with backoff (sysexits EX_TEMPFAIL)
	ExitConfig     = 78 // Configuration error (sysexits EX_CONFIG)
)

// CodeUnknown is the payload code for an error that carries no code.
const CodeUnknown = "UNKNOWN"

// canonicalExit is the canonical code table, verbatim. A domain code must not
// appear here, and warning codes never do.
var canonicalExit = map[string]int{
	"INVALID_ARGUMENT":    ExitUsage,
	"INPUT_REQUIRED":      ExitUsage,
	"ENV_REQUIRED":        ExitUsage,
	"OUTPUT_REQUIRED":     ExitUsage,
	"INVALID_FIELDS":      ExitUsage,
	"INVALID_COLOR":       ExitUsage,
	"BAD_REQUEST":         ExitUsage,
	"AGENT_UNKNOWN":       ExitUsage,
	"NO_SKILLS_CONCEPT":   ExitUsage,
	"NO_SKILLS_ROOT":      ExitUsage,
	"NO_AGENTS":           ExitUsage,
	"NOT_FOUND":           ExitNotFound,
	"UNAUTHENTICATED":     ExitPermission,
	"AUTH_EXPIRED":        ExitPermission,
	"PERMISSION_DENIED":   ExitPermission,
	"CONFLICT":            ExitConflict,
	"CURSOR_INVALID":      ExitConflict,
	"RATE_LIMITED":        ExitTransient,
	"UPSTREAM_ERROR":      ExitTransient,
	"WAIT_TIMEOUT":        ExitTransient,
	"CATALOG_UNAVAILABLE": ExitTransient,
	CodeUnknown:           ExitFailure,
	"UPSTREAM_REJECTED":   ExitFailure,
	"CATALOG_INVALID":     ExitFailure,
	"CONFIG_INVALID":      ExitConfig,
}

// Coder is implemented by an error that carries a payload code. The first
// error in a chain that implements Coder is the payload's single source:
// the other interfaces are read from that same error only.
type Coder interface {
	Code() string
}

// Fixer is implemented by a coded error that names the next command or action.
type Fixer interface {
	Fix() string
}

// ValidValuer is implemented by a coded error that rejected a value against a
// closed set, and returns that set.
type ValidValuer interface {
	ValidValues() []string
}

// RetryAfterer is implemented by a coded error that knows when a retry may
// succeed. The value uses the duration grammar, such as "30s" or "1h30m".
type RetryAfterer interface {
	RetryAfter() string
}

// ExitCoder is implemented by a coded error whose domain code has its own
// exit code. It is ignored for a canonical code, whose exit code always comes
// from the table.
type ExitCoder interface {
	ExitCode() int
}

type codedError interface {
	error
	Coder
}

// ErrorPayload describes a failure in terms an agent can act on. Its JSON form
// adds transient, derived from the exit code, so callers never set it.
type ErrorPayload struct {
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	Fix         string   `json:"fix"`
	ValidValues []string `json:"valid_values,omitempty"`
	RetryAfter  string   `json:"retry_after,omitempty"`
	// ExitCode is a domain code's own exit code. It is ignored for a
	// canonical code, and zero means ExitFailure.
	ExitCode int `json:"-"`
}

// WarningPayload is a non-fatal condition reported alongside a success.
type WarningPayload struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Fix        string `json:"fix,omitempty"`
	RetryAfter string `json:"retry_after,omitempty"`
}

// Exit returns the process exit code for p: the table's code for a canonical
// code, else p.ExitCode when set, else ExitFailure.
func (p ErrorPayload) Exit() int {
	if exit, ok := canonicalExit[p.Code]; ok {
		return exit
	}
	if p.ExitCode != 0 {
		return p.ExitCode
	}
	return ExitFailure
}

// Transient reports whether retrying can help, which is true exactly when p
// exits ExitTransient.
func (p ErrorPayload) Transient() bool { return p.Exit() == ExitTransient }

// MarshalJSON encodes p with transient derived from its exit code, so the two
// cannot disagree.
func (p ErrorPayload) MarshalJSON() ([]byte, error) {
	type wire ErrorPayload
	return json.Marshal(struct {
		wire
		Transient bool `json:"transient"`
	}{wire(p), p.Transient()})
}

// Payload derives the payload describing err, or returns nil for a nil err.
//
// The message is the whole err's text, so context added with %w survives. The
// code, fix, valid values, retry hint, and domain exit code all come from the
// first error in the chain that implements Coder, so a payload never mixes
// fields from different errors. With no Coder in the chain the code is
// CodeUnknown.
func Payload(err error) *ErrorPayload {
	if err == nil {
		return nil
	}
	p := &ErrorPayload{Code: CodeUnknown, Message: err.Error()}
	coded, ok := errors.AsType[codedError](err)
	if !ok {
		if p.Message == "" {
			p.Message = fmt.Sprintf("unclassified failure (%T)", err)
		}
		return p
	}
	if code := coded.Code(); code != "" {
		p.Code = code
	}
	if p.Message == "" {
		p.Message = fmt.Sprintf("%s failure with no message (%T)", p.Code, err)
	}
	if f, ok := coded.(Fixer); ok {
		p.Fix = f.Fix()
	}
	if v, ok := coded.(ValidValuer); ok {
		p.ValidValues = slices.Clone(v.ValidValues())
	}
	if r, ok := coded.(RetryAfterer); ok {
		p.RetryAfter = r.RetryAfter()
	}
	if e, ok := coded.(ExitCoder); ok {
		p.ExitCode = e.ExitCode()
	}
	return p
}

// ExitCode returns the process exit code for err, and is the one exit decision
// main makes. It returns ExitSuccess for a nil err. Otherwise the first error
// in the chain created by WithFinalExit supplies its code, so a code fixed when an
// error was printed or a signal was caught is never recomputed; failing that,
// the code is that of err's Payload.
func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	if e, ok := errors.AsType[*exitError](err); ok {
		return e.code
	}
	return Payload(err).Exit()
}
