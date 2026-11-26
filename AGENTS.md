# p3bot/x

Multi-module Go repository of library modules imported by the p3bot CLIs. Library only: no binaries, no `cmd/`.

## Layout

| Path | Contents |
| --- | --- |
| `clikit/` | Module `github.com/p3bot/x/clikit`: packages `fault`, `mode`, `output`, `interrupt`, `duration`, `version`, `xdg`, `atomicfile` |
| `skill/` | Module `github.com/p3bot/x/skill`: package `skill` at the module root (install, list, and uninstall of a CLI's agent skill, with coded errors) and package `catalog` (agent catalog and detection) |
| `.golangci.yml` | golangci-lint v2 config shared by every module |

There is no module at the repository root. Each module declares `go 1.27`. `go.work` is gitignored; never commit one.

## Commands

There is no hosted CI; run all of these in every module (`clikit`, `skill`) before a commit or a tag. Run from the module directory, with the workspace off so the module's own `go.mod` decides versions:

```bash
cd clikit   # then again in skill
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

## Agent catalog

`skill/catalog/agents.json` is the agent catalog and its only source. Edit it by hand; there is no CUE and no generator. It was exported once from agentdex `catalog/agents.cue` with `cue export`. The `catalog` tests enforce the rules agentdex's `schema.cue` enforced, so a bad edit fails `go test`.

Warning: catalog freeze. Until start reads `skill/catalog` instead of the agentdex registry module, do not edit `agents.json` or agentdex `catalog/agents.cue`. They must stay equivalent on every field the schema names, because start reads the registry copy while the skill-shipping CLIs read the embedded one, and the freeze keeps both agreeing on every agent's roots.

## Tags

A module is released with a `<module>/vX.Y.Z` tag, for example `clikit/v0.1.0`. The `v` is required. From v2 the module path gains `/v2` and tags stay `clikit/v2.x.y`. Both modules stay on v0 until three CLIs have migrated.

Release order across modules: a change skill needs from clikit is tagged in clikit first, then skill's `go.mod` requirement is bumped to that tag, then skill is tagged. skill uses clikit at runtime only for `atomicfile` (`fault` is a test dependency), so a change to `fault`, `mode`, or `output` never forces a skill release while the coded-error interfaces stay frozen.

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
- Injected process state (`mode.Env`, `xdg.Env`, `catalog.Env`) has a hermetic zero value; `ProcessEnv()` returns the running process
- `atomicfile` never creates directories; the caller (or `xdg.Ensure`) does
- `skill` never imports `clikit/fault` outside its tests and has no code-mapping function: its errors carry codes through fault's interfaces, and its tests assert the payloads through `fault.Payload`
- Text output, error rendering, and field selection get the design and test effort; keep the JSON envelope minimal
- MPL-2.0. Scoped Commits messages. No `Co-Authored-By` trailers
