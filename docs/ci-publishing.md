# Publishing from CI

`pvtr publish` needs **two independent tokens**:

1. **Hub/registry bearer** — authenticates the push + `/sync`. Supplied via the
   `PVTR_TOKEN` env var (or `pvtr login` for an interactive human). Provider-agnostic
   in the SDK; the **hub** decides which tokens it accepts.
2. **Sigstore signing identity** — the keyless Fulcio cert. Supplied via an OIDC
   token with audience `sigstore` (`SIGSTORE_ID_TOKEN` env, or auto-detected in
   GitHub Actions). See `internal/auth/signing.go`.

## GitHub Actions

Works with no extra config: the signing token is auto-detected from the runner's
OIDC service, and `PVTR_TOKEN` is set from the workflow's OIDC/secret.

> TODO: add the canonical GitHub Actions workflow snippet (permissions:
> `id-token: write`, how `PVTR_TOKEN` is populated).

## GitLab CI

<!-- STUB — do not ship without filling in. Blocked on the hub
     bearer model (see below); the signing half already works
     once SIGSTORE_ID_TOKEN is set. -->

The **signing** side works today: declare an `id_tokens` entry with audience
`sigstore` and expose it as `SIGSTORE_ID_TOKEN`:

```yaml
# .gitlab-ci.yml (sketch — verify before publishing)
publish:
  id_tokens:
    SIGSTORE_ID_TOKEN:
      aud: sigstore
  script:
    - pvtr publish --dist dist
```

> TODO (blocked): the **bearer** (`PVTR_TOKEN`) side depends
> on how GitLab authenticates to the hub. Resolve which model
> applies, then document it:
>
> - **OIDC trusted-publishing** -- requires the hub's Keycloak
>   to add GitLab as a trusted issuer (a hub-side change, NOT
>   in this repo). Document the `id_tokens` block for the hub
>   audience + how it maps to `PVTR_TOKEN`.
> - **Static token secret** -- document storing a hub-accepted
>   token as a masked CI variable and exporting it as
>   `PVTR_TOKEN`.
>
> Confirm the model with whoever owns grc.store before
> finalizing this section.

## Other CI providers

`SIGSTORE_ID_TOKEN` covers any provider that can mint a sigstore-audience OIDC
token into an env var. Providers whose token comes from a subprocess/metadata
call (Buildkite `buildkite-agent oidc request-token`, GCP metadata server) need
dedicated detectors in `internal/auth/signing.go` (see the TODO there) — not yet
implemented.

**CircleCI is unsupported** for keyless signing: its OIDC audience is locked to
the org and cannot be set to `sigstore`, so public-good Fulcio will reject it.
