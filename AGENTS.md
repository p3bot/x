# p3bot/x

Multi-module Go repository of library modules imported by the p3bot CLIs. Library only: no binaries, no `cmd/`.

## Layout

| Path | Contents |
| --- | --- |
| `clikit/` | Module `github.com/p3bot/x/clikit`: packages `fault`, `mode`, `output`, `interrupt`, `duration`, `version`, `xdg`, `atomicfile` |
| `skill/` | Module `github.com/p3bot/x/skill` (not yet created) |
| `.golangci.yml` | golangci-lint v2 config shared by every module |

There is no module at the repository root. Each module declares `go 1.27`. `go.work` is gitignored; never commit one.

## Commands

There is no hosted CI; run all of these before a commit or a tag. Run from a module directory, with the workspace off so the module's own `go.mod` decides versions:

```bash
cd clikit
GOWORK=off CGO_ENABLED=0 go test ./...
GOWORK=off go test -race ./...
GOWORK=off golangci-lint run ./...
GOWORK=off GOOS=windows go vet ./...
GOWORK=off GOOS=darwin go vet ./...
GOWORK=off go mod tidy
```

## Coded-error contract

`clikit/fault` defines one-method optional interfaces: `Code() string`, `Fix() string`, `ValidValues() []string`, `RetryAfter() string`, `ExitCode() int`.

- Frozen from `clikit/v0.1.0`, v0 included. Never rename, retype, or merge a method. A new payload property is a new one-method interface
- Signatures use predeclared types only, so packages outside fault implement them without importing it
- Payload derivation reads every field from the first error in the chain that implements `Code()`, never from errors further down. Do not change this rule

Implementers satisfy the interfaces structurally, so breaking the contract compiles everywhere and silently turns their errors into `UNKNOWN`.

## Tags

A module is released with a `<module>/vX.Y.Z` tag, for example `clikit/v0.1.0`. The `v` is required. From v2 the module path gains `/v2` and tags stay `clikit/v2.x.y`. Both modules stay on v0 until three CLIs have migrated.

## Dependencies

| Module | Allowed |
| --- | --- |
| clikit | Standard library, `golang.org/x/term`, `github.com/fatih/color` |
| skill | Standard library, `github.com/p3bot/x/clikit`, `github.com/goccy/go-yaml` |

No module imports cobra, pflag, agentdex, or any CLI repository. Adding anything else needs owner approval.

## Conventions

- Pure Go, no cgo. Must compile for linux, darwin, and windows
- No package-level mutable state in `output`. A CLI builds one `Output` per invocation
- `interrupt` holds no package state; `fault.ExitCode` stays the single exit decision `main` makes
- Injected process state (`mode.Env`, `xdg.Env`) has a hermetic zero value; `ProcessEnv()` returns the running process
- `atomicfile` never creates directories; the caller (or `xdg.Ensure`) does
- Text output, error rendering, and field selection get the design and test effort; keep the JSON envelope minimal
- MPL-2.0. Scoped Commits messages. No `Co-Authored-By` trailers
