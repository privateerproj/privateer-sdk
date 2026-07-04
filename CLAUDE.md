# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

The Privateer SDK is a Go library (`github.com/privateerproj/privateer-sdk`) for
building **Privateer plugins** and the **harnesses** (the `pvtr` CLI) that drive
them. Plugins evaluate infrastructure/software assets against control catalogs
and emit machine-readable results. Privateer aligns with the
[Gemara](https://gemara.openssf.org) model — this SDK depends on
`github.com/gemaraproj/go-gemara` and speaks in Gemara types (`ControlCatalog`,
`ControlEvaluation`, `EvaluationLog`, `AssessmentStep`, `Result`).

## Commands

- `make build` — `go mod tidy` + `go build ./...` + tests
- `make test` — `go vet ./...`, clears test cache, `go test ./...`
- `make testcov` — tests with coverage; prints total coverage %
- `make bench` — benchmarks
- Run one test: `go test ./pluginkit/ -run TestEvaluationOrchestrator_AddEvaluationSuite -v`

Go version is pinned in `go.mod` (currently 1.26.x). Markdown is linted with
`markdownlint-cli2` (config lives at the workspace root).

## Architecture

### The two import surfaces (this split is load-bearing — respect it)

- **`command`** is the *plugin-facing* surface. A plugin's `main` imports it for
  `NewPluginCommands`, `SetBase`, `ReadConfig`. Keep its transitive dependency
  closure small.
- **`command/harness`** is the *harness-facing* surface (install / publish /
  login / run-many). It deliberately isolates the heavy dependency stack
  (`internal/install`, `internal/publish`, `internal/oci`, sigstore, oras,
  go-git) so that plugins importing `command` do **not** pull those into their
  `go.sum`. Most harness commands are thin forwarders to implementations still
  in `command`; logic migrates from `command` → `command/harness` over time.
  See the package doc in `command/harness/harness.go`. When adding
  harness-only code, do not create an import edge that drags harness deps back
  into the plugin-facing `command` surface.

### Plugin execution flow

1. Plugin `main` builds an `pluginkit.EvaluationOrchestrator`, registers
   evaluation suites (`AddEvaluationSuite` / `AddEvaluationSuiteForAllCatalogs`,
   each taking a `DataLoader` and a `map[controlID][]AssessmentStep`), and passes
   it to `command.NewPluginCommands`.
2. The plugin binary is a [`hashicorp/go-plugin`](https://github.com/hashicorp/go-plugin)
   server. `shared.Serve` serves it; the harness dispenses it over `net/rpc`.
   The RPC contract is the one-method `shared.Pluginer` interface: `Start() (int, error)`.
3. `Plugin.Start` calls `orchestrator.Mobilize()`, which loads catalogs, filters
   to the requested service/test-suites, runs each `EvaluationSuite.Evaluate`,
   and writes results.

### Exit codes (`shared/exitcodes.go`)

Canonical codes live in `shared` so `command` and `pluginkit` can share them
without an import cycle: `TestPass, TestFail, Aborted, InternalError, BadUsage,
NoTests`. **Typed errors do not survive `net/rpc`** — the plugin side classifies
the outcome into an exit code via `pluginkit.ExitCodeFor`; don't rely on error
identity across the RPC boundary. Across multi-plugin runs the *most severe*
outcome wins (`mergeExitCode` in `command/run.go`).

### Change management (`pluginkit/change.go`)

Assessments that mutate a target register a `Change` (apply/revert funcs) with a
`ChangeManager`. Changes only run when config is `Invasive`, and are gated by
`ChangeManager.Allow()`. Failure to revert sets `CorruptedState`, surfaced on the
`EvaluationSuite`.

### Config (`config`)

Precedence (highest first): CLI flags → `PVTR_*` env vars → config file
(`--config`, else `./config.yml`, else `~/.privateer/config.yml`) → defaults.
Built on `spf13/viper` + `cobra`. `ai_*` keys are inherited from the top level
into each service (`inheritedTopLevelVarKeys`). Output formats: `yaml`, `json`,
`sarif`, `gemara`. See `docs/configuration.md`.

### AI subsystem (`ai`)

Provider-neutral client contract. Callers use only the shared types; concrete
backends are adapters registered in `clientFactories` (`openai.go` is the only
built-in). Entry point `ai.NewClient(Config)`. A **dry-run** client
(`ai_dry_run`, user flag `--dry-run-ai`) returns deterministic responses with
`FinishReason == FinishReasonDryRun` and needs no API key. `ai/assist.go`
provides `Assist`, the plugin-facing accelerator: it asks the model for a
structured `Verdict` against an SDK-owned schema and returns a `gemara.Evidence`
(type `ai-assessment`) whose `EvidencePayload` carries the verdict, the exact
prompt/content sent, and provider provenance — the caller decides whether to
record it and how the verdict folds into the step result. Content is recorded
verbatim; callers must not pass secrets. See `docs/ai-assist.md`. To add a
provider: implement `Client`, add a `Provider` constant, register it in
`clientFactories` — no caller changes.

### Publishing (`internal/publish`, `internal/oci`, `internal/auth`, `internal/verify`)

Plugins are published to a grc.store hub as signed OCI artifacts.
`pvtr publish` needs two independent tokens: a hub/registry bearer (`PVTR_TOKEN`
or `pvtr login`) and a Sigstore keyless signing identity (`SIGSTORE_ID_TOKEN`,
audience `sigstore`, auto-detected in GitHub Actions). Install verifies
signatures via sigstore. The publish coordinate is `<Publisher>/<PluginName>`;
`Publisher` and `License` (SPDX) on the orchestrator are inert at run time and
required only to publish. See `docs/ci-publishing.md`.

## Conventions

- Errors in `pluginkit` are constructed via helpers in `pluginkit/errors.go`
  that take a short call-site code (e.g. `CONFIG_NOT_INITIALIZED("ev10")`);
  follow that pattern for new error sites.
- Exported struct fields carry both `json` and `yaml` tags — output must stay
  machine-readable and standardized, and new SDK features must remain
  backward-compatible with existing community-maintained plugins.
- `internal/*` is not importable by plugins by design; keep harness-only
  machinery there.
