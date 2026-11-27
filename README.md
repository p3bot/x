# p3bot/x

Open source Go library packages shared by the [p3bot](https://github.com/p3bot) command-line tools. This repository is library only: it ships no binaries.

## Packages

The repository is one Go module, `github.com/p3bot/x`, versioned as a whole. Its packages fall in two groups:

| Group | Import path | Purpose |
| --- | --- | --- |
| clikit | `github.com/p3bot/x/clikit/...` | The tool-agnostic parts of the p3bot CLI design: exit codes and error payloads, runtime modes, and output |
| skill | `github.com/p3bot/x/skill/...` | Agent skill install, list, and uninstall for a CLI, on top of the agent catalog and installed-agent detection |

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

skill packages:

| Package | What it provides |
| --- | --- |
| `skill` | A CLI's agent skill built from its embedded `SKILL.md`: install into, list, and uninstall from agent skills roots, touching only copies the CLI owns, with coded errors (`AGENT_UNKNOWN`, `NO_SKILLS_CONCEPT`, `NO_SKILLS_ROOT`, `NO_AGENTS`, `CONFLICT`) |
| `skill/catalog` | The known AI coding agents (binary, config directories, skills roots, providers) from an embedded `agents.json`, and detection of which are installed with their paths resolved |

The catalog is `skill/catalog/agents.json`, edited by hand and validated by the package tests. It needs no network, no CUE, and no first-run fetch. While start still reads the agentdex registry catalog, `agents.json` is frozen: it must stay equivalent to agentdex's `catalog/agents.cue`, and neither file is edited.

## Using the module

Require the module; `@latest` resolves to the newest tagged version:

```bash
go get github.com/p3bot/x@latest
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

A CLI's skill commands are thin calls into `skill`. Its errors already carry their code and fix, so the CLI's error boundary passes them through.

`catalog.ProcessEnv` reports a home or working directory it cannot read and leaves that field empty. `skill` skips the roots it cannot resolve, so `List` and `Uninstall` would then succeed with nothing. Install, list, and uninstall each build their `Env` through a check that the directory their location needs is there: the home directory for `skill.Global`, the working directory for `skill.Local`.

```go
//go:embed skill.md
var skillText string

// skillEnv returns the process Env, or an error when the directory that holds
// the skills roots of loc is unusable. The other directory may be missing.
func skillEnv(loc skill.Location) (catalog.Env, error) {
	env, err := catalog.ProcessEnv()
	name, dir := "home", env.Home
	if loc == skill.Local {
		name, dir = "working", env.WorkingDir
	}
	if !filepath.IsAbs(dir) {
		missing := fmt.Errorf("%v skills roots need an absolute %s directory, got %q", loc, name, dir)
		return env, errors.Join(missing, err)
	}
	return env, nil
}

func runSkillInstall(cmd *cobra.Command, args []string) error {
	s, err := skill.New("mycli", skillText)
	if err != nil {
		return err
	}
	// skill list and skill uninstall start with the same call.
	env, err := skillEnv(skill.Global)
	if err != nil {
		return err
	}
	written, err := s.Install(env, skill.Global, args)
	for _, path := range written {
		fmt.Fprintln(cmd.OutOrStdout(), path)
	}
	return err
}
```

## Versions and tags

Releases are tagged `vX.Y.Z` at the repository root, starting at `v0.1.0`. The `v` is required. From v2 the import paths gain the major version suffix (`github.com/p3bot/x/v2/clikit/fault`).

The module stays on v0 while the first CLIs migrate to it, so its APIs may still change between minor versions. The coded-error interfaces in `clikit/fault` are the exception: they are frozen from `v0.1.0`.

## Developing

The module requires Go 1.27. Run commands from the repository root:

```bash
go test ./...
golangci-lint run ./...
```

To work on x and a consumer together, create a `go.work` in a parent directory. `go.work` is ignored by git. Before committing, test against the versions `go.mod` requires by running with `GOWORK=off`; [AGENTS.md](AGENTS.md) lists the full set of checks.

## License

[MPL-2.0](LICENSE)
