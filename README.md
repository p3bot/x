# p3bot/x

Open source Go library modules shared by the [p3bot](https://github.com/p3bot) command-line tools. This repository is library only: it ships no binaries.

## Modules

Each module lives in its own directory with its own `go.mod` and is versioned independently.

| Module | Import path | Purpose |
| --- | --- | --- |
| clikit | `github.com/p3bot/x/clikit` | The tool-agnostic parts of the p3bot CLI design: exit codes and error payloads, runtime modes, and output |
| skill | `github.com/p3bot/x/skill` | Agent catalog and agent skill install, list, and uninstall (planned) |

clikit packages:

| Package | What it provides |
| --- | --- |
| `clikit/fault` | Exit code constants, the error and warning payloads, the canonical error-code table, the coded-error interfaces, and `fault.ExitCode(err)` for `main` |
| `clikit/mode` | Detection of interactive, decorated, animated, and debug modes from flags and the environment, and the colour switch for `fatih/color` |
| `clikit/output` | An `Output` value per invocation that writes results, errors, warnings, notices, hints, and progress in text or JSON |
| `clikit/interrupt` | `interrupt.Run` for `main`: a context cancelled by SIGINT or SIGTERM, a second signal that kills, and the 128+signum exit code |
| `clikit/duration` | Parse and format the duration grammar (`30s`, `1h30m`, `7d`, `0`), and a flag value for cobra/pflag |
| `clikit/version` | The reported version from a CLI's ldflags value or the module build info, plus the VCS revision and time for diagnostics |
| `clikit/xdg` | Per-user config, data, state, and cache directories with macOS defaults, and system config lookup across `$XDG_CONFIG_DIRS` |
| `clikit/atomicfile` | Replace a file through a temp file and rename, so readers never see a partial write |

## Using a module

Require the module; `@latest` resolves to the newest tagged version (see [Versions and tags](#versions-and-tags) to pin one):

```bash
go get github.com/p3bot/x/clikit@latest
```

A coded error needs no import of clikit. Any error type that implements `Code() string` becomes an error payload, and may add any of `Fix() string`, `ValidValues() []string`, `RetryAfter() string`, and `ExitCode() int`:

```go
type lockedError struct{ branch string }

func (e lockedError) Error() string { return e.branch + " is locked" }
func (e lockedError) Code() string  { return "BRANCH_LOCKED" }
func (e lockedError) Fix() string   { return "mycli unlock " + e.branch }
func (e lockedError) ExitCode() int { return 5 }
```

A CLI's `main` hands the signal-aware run to `fault.ExitCode`, which decides the exit code for errors and signals alike:

```go
func main() {
	os.Exit(fault.ExitCode(interrupt.Run(cli.Execute)))
}
```

## Versions and tags

Go resolves a module in a subdirectory from tags prefixed with that directory, so releases are tagged `<module>/vX.Y.Z`:

- `clikit/v0.1.0`
- `skill/v0.1.0`

The `v` is required; `clikit/0.1.0` is not a module version and the Go toolchain ignores it. From v2 a module's import path gains the major version suffix (`github.com/p3bot/x/clikit/v2`) and its tags stay `clikit/v2.x.y`.

Both modules stay on v0 while the first CLIs migrate to them, so their APIs may still change between minor versions. The coded-error interfaces in `clikit/fault` are the exception: they are frozen from `clikit/v0.1.0`.

## Developing

The modules require Go 1.27. There is no module at the repository root; run commands from a module directory:

```bash
cd clikit
go test ./...
golangci-lint run ./...
```

To work on a module and a consumer together, create a `go.work` in a parent directory. `go.work` is ignored by git. Before committing, test each module against the versions its own `go.mod` requires by running with `GOWORK=off`; [AGENTS.md](AGENTS.md) lists the full set of checks.

## License

[MPL-2.0](LICENSE)
