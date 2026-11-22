package fault_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"testing"

	"github.com/p3bot/x/clikit/fault"
)

// fixOnly is declared outside package fault and implements only Code and
// Fix, proving a package can produce payloads without importing fault.
type fixOnly struct {
	code, fix string
}

func (e fixOnly) Error() string { return "fix only failed" }
func (e fixOnly) Code() string  { return e.code }
func (e fixOnly) Fix() string   { return e.fix }

// domainErr carries every optional property, including a domain exit code.
type domainErr struct {
	code  string
	exit  int
	cause error
}

func (e *domainErr) Error() string         { return "domain failed" }
func (e *domainErr) Unwrap() error         { return e.cause }
func (e *domainErr) Code() string          { return e.code }
func (e *domainErr) Fix() string           { return "inner fix" }
func (e *domainErr) ValidValues() []string { return []string{"a", "b"} }
func (e *domainErr) RetryAfter() string    { return "30s" }
func (e *domainErr) ExitCode() int         { return e.exit }

func TestPayload(t *testing.T) {
	inner := &domainErr{code: "INNER_DOMAIN", exit: fault.ExitConflict}
	tests := []struct {
		name string
		err  error
		want fault.ErrorPayload
		exit int
	}{
		{
			name: "local type with only code and fix takes the table's exit code",
			err:  fixOnly{code: "NOT_FOUND", fix: "list them first"},
			want: fault.ErrorPayload{Code: "NOT_FOUND", Message: "fix only failed", Fix: "list them first"},
			exit: fault.ExitNotFound,
		},
		{
			name: "plain error is unknown",
			err:  errors.New("boom"),
			want: fault.ErrorPayload{Code: fault.CodeUnknown, Message: "boom"},
			exit: fault.ExitFailure,
		},
		{
			name: "empty message is described by type",
			err:  errors.New(""),
			want: fault.ErrorPayload{Code: fault.CodeUnknown, Message: "unclassified failure (*errors.errorString)"},
			exit: fault.ExitFailure,
		},
		{
			name: "coded error with an empty message names its code",
			err:  fault.New("NOT_FOUND", "", "list pages"),
			want: fault.ErrorPayload{Code: "NOT_FOUND", Message: "NOT_FOUND failure with no message (*fault.Error)", Fix: "list pages"},
			exit: fault.ExitNotFound,
		},
		{
			name: "wrapped coded error keeps the whole message and the inner code",
			err:  fmt.Errorf("context: %w", fault.New("NOT_FOUND", "page missing", "list pages")),
			want: fault.ErrorPayload{Code: "NOT_FOUND", Message: "context: page missing", Fix: "list pages"},
			exit: fault.ExitNotFound,
		},
		{
			name: "domain code supplies its own exit code",
			err:  inner,
			want: fault.ErrorPayload{
				Code: "INNER_DOMAIN", Message: "domain failed", Fix: "inner fix",
				ValidValues: []string{"a", "b"}, RetryAfter: "30s", ExitCode: fault.ExitConflict,
			},
			exit: fault.ExitConflict,
		},
		{
			name: "canonical code ignores a domain exit code",
			err:  &domainErr{code: "INVALID_ARGUMENT", exit: fault.ExitConflict},
			want: fault.ErrorPayload{
				Code: "INVALID_ARGUMENT", Message: "domain failed", Fix: "inner fix",
				ValidValues: []string{"a", "b"}, RetryAfter: "30s", ExitCode: fault.ExitConflict,
			},
			exit: fault.ExitUsage,
		},
		{
			name: "unlisted code without an exit code exits 1",
			err:  fault.New("SOMETHING_ODD", "odd", "look closer"),
			want: fault.ErrorPayload{Code: "SOMETHING_ODD", Message: "odd", Fix: "look closer"},
			exit: fault.ExitFailure,
		},
		{
			name: "outer coded wrapper does not inherit the inner error's fields",
			err:  fault.New("PARTIAL_FAILURE", "some failed", "retry the failed ones").WithCause(inner),
			want: fault.ErrorPayload{Code: "PARTIAL_FAILURE", Message: "some failed", Fix: "retry the failed ones"},
			exit: fault.ExitFailure,
		},
		{
			name: "outer local coded type without fix leaves fix empty",
			err:  &codeOnly{cause: inner},
			want: fault.ErrorPayload{Code: "OUTER", Message: "outer: domain failed"},
			exit: fault.ExitFailure,
		},
		{
			name: "empty code reads as unknown",
			err:  fixOnly{fix: "f"},
			want: fault.ErrorPayload{Code: fault.CodeUnknown, Message: "fix only failed", Fix: "f"},
			exit: fault.ExitFailure,
		},
		{
			name: "fault error with every property",
			err: fault.New("RATE_LIMITED", "slow down", "wait and retry").
				WithRetryAfter("1m").WithValidValues("x").WithDomainExitCode(fault.ExitUsage),
			want: fault.ErrorPayload{
				Code: "RATE_LIMITED", Message: "slow down", Fix: "wait and retry",
				ValidValues: []string{"x"}, RetryAfter: "1m", ExitCode: fault.ExitUsage,
			},
			exit: fault.ExitTransient,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fault.Payload(tt.err)
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("Payload() = %+v, want %+v", *got, tt.want)
			}
			if got.Exit() != tt.exit {
				t.Errorf("Exit() = %d, want %d", got.Exit(), tt.exit)
			}
			if code := fault.ExitCode(tt.err); code != tt.exit {
				t.Errorf("ExitCode() = %d, want %d", code, tt.exit)
			}
		})
	}
}

type codeOnly struct{ cause error }

func (e *codeOnly) Error() string { return "outer: " + e.cause.Error() }
func (e *codeOnly) Unwrap() error { return e.cause }
func (e *codeOnly) Code() string  { return "OUTER" }

func TestPayloadNil(t *testing.T) {
	if p := fault.Payload(nil); p != nil {
		t.Errorf("Payload(nil) = %+v, want nil", p)
	}
}

func TestPayloadClonesValidValues(t *testing.T) {
	values := []string{"a"}
	err := fault.New("INVALID_FIELDS", "bad", "fix").WithValidValues(values...)
	values[0] = "changed"
	p := fault.Payload(err)
	p.ValidValues[0] = "mutated"
	if got := err.ValidValues(); !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("ValidValues() = %v, want [a]", got)
	}
}

func TestExitCode(t *testing.T) {
	coded := fault.New("NOT_FOUND", "missing", "list")
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil succeeds", nil, fault.ExitSuccess},
		{"plain error fails", errors.New("x"), fault.ExitFailure},
		{"exit-carrying error alone", fault.WithFinalExit(130, nil), 130},
		{"exit-carrying error beats a coded error beneath it", fault.WithFinalExit(130, coded), 130},
		{"exit-carrying error found under a coded wrapper", fault.New("CONFLICT", "c", "f").WithCause(fault.WithFinalExit(143, coded)), 143},
		{"exit-carrying error found through %w", fmt.Errorf("run: %w", fault.WithFinalExit(fault.ExitUsage, coded)), fault.ExitUsage},
		{"first exit-carrying error wins", fault.WithFinalExit(4, fault.WithFinalExit(5, nil)), 4},
		{"printed marker alone does not change the code", fault.MarkPrinted(coded), fault.ExitNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fault.ExitCode(tt.err); got != tt.want {
				t.Errorf("ExitCode() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWithFinalExitKeepsChain(t *testing.T) {
	err := fault.WithFinalExit(fault.ExitUsage, fmt.Errorf("open: %w", fs.ErrNotExist))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Error("errors.Is lost the wrapped error")
	}
	if got := err.Error(); got != "open: file does not exist" {
		t.Errorf("Error() = %q", got)
	}
	if got := fault.WithFinalExit(130, nil).Error(); got != "exit status 130" {
		t.Errorf("nil Error() = %q", got)
	}
}

func TestMarkPrinted(t *testing.T) {
	base := fault.New("NOT_FOUND", "missing", "list")
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unmarked", base, false},
		{"marked", fault.MarkPrinted(base), true},
		{"marked then wrapped", fmt.Errorf("ctx: %w", fault.MarkPrinted(base)), true},
		{"marked inside an exit error", fault.WithFinalExit(2, fault.MarkPrinted(base)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fault.IsPrinted(tt.err); got != tt.want {
				t.Errorf("IsPrinted() = %v, want %v", got, tt.want)
			}
		})
	}
	if fault.MarkPrinted(nil) != nil {
		t.Error("MarkPrinted(nil) != nil")
	}
	marked := fault.MarkPrinted(base)
	if !errors.Is(marked, base) || marked.Error() != "missing" {
		t.Errorf("marked error lost its chain or message: %v", marked)
	}
}

func TestErrorWithReturnsCopies(t *testing.T) {
	base := fault.New("CONFLICT", "busy", "wait")
	cause := errors.New("cause")
	_ = base.WithCause(cause).WithValidValues("a").WithRetryAfter("5s").WithDomainExitCode(9)
	if base.Unwrap() != nil || base.ValidValues() != nil || base.RetryAfter() != "" || base.ExitCode() != 0 {
		t.Errorf("With methods mutated the receiver: %+v", base)
	}
	derived := base.WithCause(cause)
	if !errors.Is(derived, cause) {
		t.Error("WithCause did not wrap the cause")
	}
}

func TestErrorPayloadJSON(t *testing.T) {
	tests := []struct {
		name string
		p    fault.ErrorPayload
		want string
	}{
		{
			name: "transient derived from canonical exit",
			p:    fault.ErrorPayload{Code: "RATE_LIMITED", Message: "slow", Fix: "wait", RetryAfter: "30s"},
			want: `{"code":"RATE_LIMITED","message":"slow","fix":"wait","retry_after":"30s","transient":true}`,
		},
		{
			name: "domain exit 75 is transient",
			p:    fault.ErrorPayload{Code: "QUEUE_BUSY", Message: "busy", Fix: "retry", ExitCode: fault.ExitTransient},
			want: `{"code":"QUEUE_BUSY","message":"busy","fix":"retry","transient":true}`,
		},
		{
			name: "valid values and non-transient",
			p:    fault.ErrorPayload{Code: "INVALID_FIELDS", Message: "bad", Fix: "f", ValidValues: []string{"id"}},
			want: `{"code":"INVALID_FIELDS","message":"bad","fix":"f","valid_values":["id"],"transient":false}`,
		},
		{
			name: "fix is always present",
			p:    fault.ErrorPayload{Code: "UNKNOWN", Message: "m"},
			want: `{"code":"UNKNOWN","message":"m","fix":"","transient":false}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.p)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestWarningPayloadJSON(t *testing.T) {
	got, err := json.Marshal(fault.WarningPayload{Code: "FLAG_DEPRECATED", Message: "old flag"})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"code":"FLAG_DEPRECATED","message":"old flag"}`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
