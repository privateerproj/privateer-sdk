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

AI is opt-in. When none of the `ai_*` keys are set, `ai.NewClient` returns
`(nil, nil)` and a plugin should simply skip its AI-assisted paths. Keys set at
the top level of the config file are inherited into every service.

<!-- markdownlint-disable MD013 -->

| Config key | Env var | Default | Purpose |
| --- | --- | --- | --- |
| `ai_provider` | `PVTR_AI_PROVIDER` | -- | Backend adapter. Currently `openai` or `anthropic`. |
| `ai_model` | `PVTR_AI_MODEL` | -- | Provider model id (e.g. `gpt-4o-mini`). |
| `ai_api_key` | `PVTR_AI_API_KEY` | -- | Provider credential. |
| `ai_base_url` | `PVTR_AI_BASE_URL` | adapter default | Override endpoint (proxy, gateway, self-hosted). |
| `ai_timeout` | `PVTR_AI_TIMEOUT` | `30s` | Per-call timeout (Go duration string). |
| `ai_max_tokens` | `PVTR_AI_MAX_TOKENS` | `256` | Response length cap. |

<!-- markdownlint-enable MD013 -->

Example `config.yml`:

```yaml
ai_provider: openai
ai_model: gpt-4o-mini
# Prefer the env var PVTR_AI_API_KEY for the credential rather than the file.
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
    return response.GemaraResult(), response.Reasoning, response.GemaraConfidence()
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
| `Reasoning` | free text | Short justification. |
| `Citations` | strings | Optional pointers to where support was found. |

<!-- markdownlint-enable MD013 -->

Anything other than an explicit `pass`/`fail` — including an unrecognized value
— maps to `NeedsReview` via `GemaraResult()`. An AI-assisted check therefore
**never silently passes a control**; the worst case is that a human is asked to
review.

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
