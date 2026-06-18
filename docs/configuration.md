# Configuration

Privateer reads configuration from (highest precedence first): command-line
flags, `PVTR_*` environment variables, the config file (`config.yml`, found via
`--config` or `./config.yml` then `~/.privateer/config.yml`), then built-in
defaults.

## Harness keys

These drive the harness (`pvtr install` / `publish` / `run` / `list`), not a
plugin serving itself. Their flags are registered by `harness.SetHarnessFlags`
on the CLI root.

| Config key (`config.yml`) | Flag | Env var | Default | Purpose |
|---|---|---|---|---|
| `hub-url` | `--hub-url` | `PVTR_HUB_URL` | `https://hub.grc.store` | grc.store hub base URL. The OCI registry host is *discovered* from it (`/.well-known/grc-store-configuration`); never hardcode the registry host. |
| `autoinstall` | `--autoinstall` | `PVTR_AUTOINSTALL` | `false` | When true, `pvtr run` first installs any config-requested plugins that are not yet installed — a single `pvtr run` installs-and-runs (useful in CI). |
| `binaries-path` | — | `PVTR_BINARIES_PATH` | — | Directory where plugins are installed. (Config/env only; no flag.) |

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
