# Configuration

Privateer reads configuration from (highest precedence first): command-line
flags, `PVTR_*` environment variables, the config file (`config.yml`, found via
`--config` or `./config.yml` then `~/.privateer/config.yml`), then built-in
defaults.

## Harness keys

These drive the harness (`pvtr install` / `publish` / `run` / `list`), not a
plugin serving itself. Their flags are registered by `harness.SetHarnessFlags`
on the CLI root.

<!-- markdownlint-disable MD013 -->

| Config key | Flag | Env var | Default | Purpose |
| --- | --- | --- | --- | --- |
| `hub-url` | `--hub-url` | `PVTR_HUB_URL` | `https://hub.grc.store` | Hub base URL. Registry host is discovered from it. |
| `autoinstall` | `--autoinstall` | `PVTR_AUTOINSTALL` | `false` | Auto-install missing plugins before `pvtr run`. |
| `binaries-path` | -- | `PVTR_BINARIES_PATH` | -- | Plugin install directory. Config/env only. |
| `benchmark` | -- | `PVTR_BENCHMARK` | `false` | Time the loader and every step; write `benchmark.json` next to results. Set by `pvtr benchmark`; env only for direct plugin runs. |
| `benchmark-payload-only` | -- | `PVTR_BENCHMARK_PAYLOAD_ONLY` | `false` | Time the loader only and skip assessment steps. Ignored unless `benchmark` is set. |

<!-- markdownlint-enable MD013 -->

Example `config.yml`:

```yaml
hub-url: https://hub.preview.grc.store
autoinstall: true
binaries-path: ./.privateer/bin
services:
  my-service:
    plugin: ossf/pvtr-github-repo-scanner
    version: 1.4.0   # optional; omit for the latest installed version
```

## Publishing from CI

See [ci-publishing.md](./ci-publishing.md) for the `PVTR_TOKEN` (hub bearer) and
`SIGSTORE_ID_TOKEN` (signing identity) tokens that `pvtr publish` needs in CI.
