# AI-assisted assessments

The `ai` package lets a plugin add an **AI-assisted follow-up** to an assessment
step: when a deterministic check is inconclusive, ask a model, and record its
answer as [go-gemara](https://gemara.openssf.org) `Evidence` that flows into the
plugin's normal `EvaluationLog` output. There is no separate evidence store or
file format — the AI answer lands in the assessment log alongside every other
piece of evidence.

The SDK provides two things:

- a provider-neutral **client** (`ai.NewClient`) that talks to a model,
  driven entirely by config so operators, not plugin authors, choose the
  provider, model, and credentials — see
  [The AI client contract](ai-client.md); and
- an **accelerator** (`ai.Assist`) that asks the model for a structured verdict
  against an SDK-owned schema and hands back a ready-to-record `gemara.Evidence`.

## Configuration

**AI is enabled when `ai_provider` is set, unless `ai_skip` is `true`.** When
`ai_provider` is unset, `ai.NewClient` returns `(nil, nil)` and a plugin should
simply skip its AI-assisted paths; no other recognized `ai_*` key turns AI on
or causes an error while dormant. Keys set at the top level of the config file
are inherited into every service.

<!-- markdownlint-disable MD013 -->

| Config key | Env var | Default | Purpose |
| --- | --- | --- | --- |
| `ai_provider` | `PVTR_AI_PROVIDER` | -- | Backend adapter. Currently `openai` or `anthropic`. Enables AI. |
| `ai_model` | `PVTR_AI_MODEL` | -- | Provider model id (e.g. `gpt-4o-mini`). Required when `ai_provider` is set. |
| `ai_api_key` | `PVTR_AI_API_KEY` | -- | Provider credential. A config-file value is accepted with a warning; prefer the environment or `ai_api_key_env`. |
| `ai_api_key_env` | -- | -- | Name of the environment variable holding the credential. Config-file only, so a target can point at its own variable. |
| `ai_base_url` | `PVTR_AI_BASE_URL` | adapter default | Absolute HTTP(S) API-root URL for a proxy, gateway, or self-hosted endpoint; no userinfo, query, or fragment. Stands in for the credential when the endpoint needs none. |
| `ai_timeout` | `PVTR_AI_TIMEOUT` | `30s` | Per-call timeout (Go duration string). Must be positive. |
| `ai_max_tokens` | `PVTR_AI_MAX_TOKENS` | `1024` | Response length cap. Must be positive. |
| `ai_skip` | `PVTR_AI_SKIP` | `false` | Turn AI off without removing the rest of the config. True at any level wins. A non-boolean config-file value is a startup error, not a skip; the environment side is looser and parses any Go boolean literal. |

<!-- markdownlint-enable MD013 -->

For settings other than credentials and `ai_skip`, precedence is
`PVTR_AI_*` environment variable, target `vars`, top-level `vars:` compatibility
value, flat top-level config value, then the built-in default. `ai_skip` is true
if it is true at any of those levels. `ai_api_key_env` is config-file only;
`PVTR_AI_API_KEY_ENV` is not supported.

A credential is resolved highest-priority first: the target's `ai_api_key`, the
target's `ai_api_key_env`, `PVTR_AI_API_KEY`, the top-level `ai_api_key`, then
the top-level `ai_api_key_env`. When `ai_api_key_env` is the selected source, an
unset or empty named variable is an error rather than a signal to try a
lower-priority source. Top-level credential sources may use flat keys or the
compatibility `vars:` map; when both spellings define the same key, `vars:` wins.

A `Config` built by hand rather than by `NewConfig` has no target entry to read,
so its `Vars` are treated as the target tier and the top-level pass is skipped.
Such a config also does not inherit `PVTR_AI_*` from the process environment,
because those values reach the SDK through Viper's env binding, which only a
Privateer command sets up.

Privateer accepts `ai_api_key` in the config file for compatibility and for
ephemeral secret-mounted configurations, but warns whenever the flat top level,
global `vars:`, or selected target stores a non-empty value, even when a
higher-priority credential is selected. A key under an unselected target warns
when that target is evaluated. Avoid committing credentials; prefer
`PVTR_AI_API_KEY` for one shared credential or a target's `ai_api_key_env` for
per-target credentials. Neither the warning nor config tracing includes the
credential value.

A run fails at startup when AI is enabled but misconfigured, rather than at the
first AI call.

Example `config.yml`:

```yaml
ai_provider: openai
ai_model: gpt-4o-mini
services:
  my-service:
    plugin: ossf/pvtr-github-repo-scanner
```

## Adding an AI-assisted step

Build the client once at plugin startup and make it reachable from the payload
your steps receive:

```go
client, err := ai.NewClient(cfg)
if err != nil {
    return err
}
// Stash client on the payload (or a package variable) so steps can reach it.
// When AI is not configured, client is nil — guard for it in the step.
```

Then, in a `gemara.AssessmentStep`, fall back to the model only when the
deterministic check cannot answer. The step decides whether to record the
evidence and how the verdict folds into its result — `Assist` never calls
`AddEvidence` and never chooses the result for you.

<!-- markdownlint-disable MD013 -->

```go
// HasUserGuides passes when a user guide is declared in Security Insights, and
// otherwise asks the model to look for one before giving up.
func HasUserGuides(payload any) (gemara.Result, string, gemara.ConfidenceLevel) {
    p := payload.(*data.Payload)

    if p.Insights.Project.Documentation.DetailedGuide != "" {
        return gemara.Passed, "user guide declared in Security Insights", gemara.High
    }
    if p.AIClient == nil {
        return gemara.Failed, "no user guide declared in Security Insights", gemara.High
    }

    // Deterministic check was inconclusive: run the AI-assisted follow-up.
    response, evidence, err := ai.Assist(context.Background(), p.AIClient, ai.Question{
        Prompt:   "Does this repository document a user guide? Cite where you found it.",
        Material: p.Readme,
    })
    if err != nil {
        return gemara.Unknown, err.Error(), gemara.Undetermined
    }

    p.AddEvidence(evidence) // p embeds gemara.EvidenceCollector
    return response.GemaraResult(), response.Summary(), response.GemaraConfidence()
}
```

<!-- markdownlint-enable MD013 -->

For the payload to carry evidence, embed `gemara.EvidenceCollector` in it; the
`AssessmentLog` harvests whatever a step adds after the step runs:

```go
type Payload struct {
    gemara.EvidenceCollector
    AIClient ai.Client
    // ... your data ...
}
```

## The response and evidence

`Assist` requests an SDK-owned JSON Schema, so plugin authors never write one.
The parsed answer is `ai.Response`:

<!-- markdownlint-disable MD013 -->

| Field | Values | Notes |
| --- | --- | --- |
| `Result` | `pass` / `fail` / `needs_review` | `GemaraResult()` maps to `Passed` / `Failed` / `NeedsReview`. |
| `Confidence` | `low` / `medium` / `high` | `GemaraConfidence()` maps to the matching `gemara.ConfidenceLevel`. |
| `Message` | one sentence | What was found or missing. Guaranteed single-line, at most 160 chars (SDK-owned). |
| `Explanation` | free text | Verbose justification. Capped at `Question.MaxExplanationChars` (default 1500). |
| `Citations` | strings | Optional pointers to where support was found. |

<!-- markdownlint-enable MD013 -->

Anything other than an explicit `pass`/`fail` — including an unrecognized value
— maps to `NeedsReview` via `GemaraResult()`. An AI-assisted check therefore
**never silently passes a control**; the worst case is that a human is asked to
review.

The model is asked for two separately budgeted texts so the short one can be
the assessment message and the long one lives only in the evidence:

- `Message` is the step's message: use `response.Summary()`, which renders it
  as `[AI-Assisted] <message>` (e.g. `[AI-Assisted] No evidence found for when
  tests are run.`). The SDK enforces the single-line shape and the 160-char cap
  after parsing, so the message always looks like every other assessment
  message.
- `Explanation` and `Citations` are the verbose record. They are already inside
  the returned `Evidence` payload — do not restate them in the message. Plugins
  that want a longer or shorter explanation set `Question.MaxExplanationChars`.

When `Assist` gets a usable structured response from the provider, it returns a
self-describing `gemara.Evidence`:

- `Type` is `ai-assessment`, marking the record as software-assisted rather than
  directly observed.
- `Payload` is an `ai.EvidencePayload` carrying the `Response`, the exact
  question asked (prompt and material), and provenance (provider, model,
  request id) — so a reviewer can see precisely what the model was shown and
  audit or reproduce the answer without provider-side request logs.
- `Description` is fixed to `"AI Assisted Review"`.

If `Assist` never got a response at all — nil client, a provider error, a nil
response, or an unparseable body — it returns a zero `Response`, a zero
`gemara.Evidence`, and a non-nil `error`. There is nothing to record as
evidence in that case; callers should fold the error into the step's own
result (see the example above) rather than looking for evidence.

Because the prompt and material are recorded verbatim in the evaluation output,
**never put secrets in `Question.Material`** — anything sent to the model also
lands in the results file. The SDK deliberately performs no redaction: a secret
that does not belong in a results file does not belong in a prompt to an AI
provider in the first place.

## Advanced: custom schemas

`Assist` covers the common pass/fail/needs-review case. When a step needs a
different structured answer, call the client directly with your own schema and
build the `gemara.Evidence` yourself:

```go
resp, err := client.Analyze(ctx, prompt, content, &ai.Schema{
    Name:   "my_schema",
    Strict: true,
    Value:  json.RawMessage(`{ "type": "object", ... }`),
})
```

See the `ai` package GoDoc for the full `Client`, `Schema`, and `AnalyzeResponse`
contract.
