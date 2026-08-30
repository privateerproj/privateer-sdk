# Configuration

Privateer reads configuration from (highest precedence first): command-line
flags, `PVTR_*` environment variables, the config file (`config.yml`, found via
`--config` or `./config.yml` then `~/.privateer/config.yml`), then built-in
defaults.

## Harness keys

These drive the harness (`pvtr install` / `publish` / `run` / `list`), not a
plugin serving itself. Their flags sit on the CLI root, except `--concurrency`,
which only affects `pvtr run` (the flag also appears on plugin binaries, which
share the flag set, but a plugin serving itself ignores it).

<!-- markdownlint-disable MD013 -->

| Config key | Flag | Env var | Default | Purpose |
| --- | --- | --- | --- | --- |
| `hub-url` | `--hub-url` | `PVTR_HUB_URL` | `https://hub.grc.store` | Hub base URL. Registry host is discovered from it. |
| `autoinstall` | `--autoinstall` | `PVTR_AUTOINSTALL` | `false` | Auto-install missing plugins before `pvtr run`. |
| `concurrency` | `pvtr run --concurrency` | `PVTR_CONCURRENCY` | `1` | Max plugins to run at once. `1` is sequential; `0` means one per CPU. Explicit values are capped at `100` or the CPU count, whichever is larger. Negative or non-integer values are rejected as bad usage. Above `1`, plugin console output may interleave. |
| `binaries-path` | -- | `PVTR_BINARIES_PATH` | -- | Plugin install directory. Config/env only. |
| `benchmark` | -- | `PVTR_BENCHMARK` | `false` | Time the loader and every step; write `benchmark.json` next to results. Set by `pvtr benchmark`; env only for direct plugin runs. |
| `benchmark-payload-only` | -- | `PVTR_BENCHMARK_PAYLOAD_ONLY` | `false` | Time the loader only and skip assessment steps. Ignored unless `benchmark` is set. |

<!-- markdownlint-enable MD013 -->

Example `config.yml`:

```yaml
hub-url: https://hub.preview.grc.store
autoinstall: true
binaries-path: ./.privateer/bin
targets:
  my-target:
    plugin: ossf/pvtr-github-repo-scanner
    version: 1.4.0   # optional; omit for the latest installed version
```

## Targets and the legacy `services` alias

`targets:` names the things a run evaluates. The `services:` key and the
`--service` / `-s` flag are legacy aliases kept for compatibility: `services:`
works exactly like `targets:`, and `--service` works like `--target`.

Resolution rules:

- An explicitly passed flag (`--target` or `--service`) always wins over
  `PVTR_TARGET` / `PVTR_SERVICE` env vars and config-file values, regardless
  of which alias it uses. Within the same source, `target` wins over `service`.
- A non-empty `targets:` map replaces `services:` entirely; the two maps are
  never merged. An empty or absent `targets:` falls back to `services:`.

Prefer `targets:` and `--target` in new configs; `--target` has no shorthand
because `-t` belongs to `--test-suites`.

### Targeted runs

When a target is set (via flag, env var, or config key), `pvtr run` is scoped
to that single target: only its plugin is validated, autoinstalled (when
`autoinstall: true`), and executed. The run prints the active target to its
output up front, so a scope inherited from an env var or config key is always
visible. Target names match config entries case-insensitively. Other entries
in the config are ignored entirely, so a shared config with a broken or
not-yet-installed entry does not block a targeted run. A target that names no
configured entry fails with an error listing the targets the config defines.

Without a target, a run covers every configured entry. Any plugins that are
requested but not installed are reported together in a single error; with
`autoinstall: true`, the install preflight still stops at the first plugin
that fails to install. `pvtr install --from-config` always installs the whole
config regardless of any target setting.

## Publishing from CI

See [ci-publishing.md](./ci-publishing.md) for the `PVTR_TOKEN` (hub bearer) and
`SIGSTORE_ID_TOKEN` (signing identity) tokens that `pvtr publish` needs in CI.
