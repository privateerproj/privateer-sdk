# The AI client contract

The `ai` package exposes a **provider-neutral client**: callers use only the
shared types re-exported from the single `ai` import, and concrete backends
(currently OpenAI and Anthropic) are adapters registered internally. Under the
hood the
subsystem is split into subpackages — `ai/provider` holds the neutral contract
and the base adapters build on, and each backend lives in its own sibling
package (e.g. `ai/openai`, `ai/anthropic`) — but plugins never need to import
them directly.
Adding or swapping a built-in provider does not change any calling code. For
the higher-level assessment accelerator built on top of this client, see
[AI-assisted assessments](ai-assist.md).

## Constructing a client

Plugins should use **`ai.NewClient(sdkconfig.Config)`**. It reads the operator's
`ai_*` settings (see the config table in
[AI-assisted assessments](ai-assist.md)) and builds the client from them, and
returns `(nil, nil)` when none are set — so a plugin can treat AI as an optional
capability without special-casing the unset path. Check the returned client, not
just the error: a nil client means AI is disabled. This is deliberate — the
provider, model, and credentials are the *operator's* choice, made in config,
not something a plugin hardcodes.

`ai.NewClientWithAIConfig(ai.Config)` is the lower-level primitive that `NewClient`
builds on. Reach for it directly only when you already hold a hand-built
`ai.Config` rather than an SDK config — for example in tests, when injecting a
custom `HTTPClient` or `BaseURL` for instrumentation, or when embedding the SDK
in a tool that sources its settings elsewhere. A typical plugin does not
hand-build a `Config`.

`NewClientWithAIConfig` normalizes and validates the Config, then returns the
adapter registered for `Config.Provider`; an unregistered provider surfaces as
an error at construction rather than at first use.

## Config

`Config` holds provider-neutral settings; provider-specific concerns (auth
header shape, endpoint paths, request body schema) are the adapter's
responsibility.

<!-- markdownlint-disable MD013 -->

| Field | Purpose | Zero value |
| --- | --- | --- |
| `Provider` | Selects which adapter is constructed. | required |
| `APIKey` | Credential passed to the provider. | required for live calls |
| `Model` | Provider model id (e.g. `gpt-4o-mini`). | required |
| `BaseURL` | Overrides the adapter's default endpoint (proxies, gateways, self-hosted). | adapter default |
| `Timeout` | Bounds a single `Analyze` call. | `30s` |
| `MaxTokens` | Caps response length, bounding cost and latency. | `1024` |
| `HTTPClient` | Injects a custom transport (tests, instrumentation). | a `Timeout`-honoring client |

<!-- markdownlint-enable MD013 -->

`Model` may differ from the model the provider reports using when the requested
name is an alias resolved to a pinned version (e.g. `gpt-4o-mini` ->
`gpt-4o-mini-2024-07-18`); the resolved name comes back on
`ResponseMetadata.Model`.

## Structured output with a Schema

Passing a `*Schema` to `Analyze` tells the provider to return a structured JSON
answer instead of free-form text. The adapter translates the JSON Schema
document into whatever the provider expects (OpenAI's `response_format`
`json_schema`, Anthropic's `output_config.format`, Gemini's `responseSchema`).
When a
schema is supplied, `AnalyzeResponse.JSON` holds the parsed payload, ready to
`json.Unmarshal` into a Go type.

<!-- markdownlint-disable MD013 -->

| Field | Meaning |
| --- | --- |
| `Name` | Labels the schema for the provider (some display or log it). OpenAI requires it when a schema is supplied; Anthropic ignores it. |
| `Description` | Short sentence telling the model what the schema is for. Optional; improves output when field names are ambiguous. |
| `Value` | The JSON Schema document the response must conform to. |
| `Strict` | Asks the provider to reject responses that do not match `Value` exactly. Providers without strict mode ignore it. |

<!-- markdownlint-enable MD013 -->

```go
schema := &ai.Schema{
    Name:        "doc_grade",
    Description: "Grade for a repository's documentation quality.",
    Strict:      true,
    Value: json.RawMessage(`{
        "type": "object",
        "properties": {
            "verdict":    {"type": "string", "enum": ["pass", "fail"]},
            "confidence": {"type": "number"},
            "reason":     {"type": "string"}
        },
        "required": ["verdict", "confidence", "reason"],
        "additionalProperties": false
    }`),
}

resp, err := client.Analyze(ctx, prompt, readme, schema)
// resp.JSON: {"verdict":"pass","confidence":0.82,"reason":"..."}
```

## AnalyzeResponse and metadata

`AnalyzeResponse` is the normalized result every adapter returns. Without a
schema, only `Text` is populated. With a schema, `JSON` holds the parsed
structured payload and `Text` holds the raw message content.

`ResponseMetadata` is diagnostic-only — the SDK does not read it itself. It is
returned so callers can log it, persist it in audit trails, attribute results
across multiple clients, or branch on signals like `FinishReason`:

- `Provider` — the adapter that produced the response.
- `Model` — the model the provider reports having used (see the alias note above).
- `RequestID` — the provider's per-call id, for correlating with provider-side
  logs. Adapters prefer the HTTP response header and fall back to a body field.
- `FinishReason` — the provider's native reason for ending generation. Passed
  through unchanged, so treat it as provider-specific rather than assuming one
  shared set of strings across providers.

## Errors

Adapters normalize provider-specific failures into a single `*ai.Error` type so
callers never depend on the exact wording or status code a provider returned.
Inspect `Kind` (and, when relevant, `StatusCode`); the original transport error
is preserved via `Unwrap`, so `errors.Is` / `errors.As` still reach it.

<!-- markdownlint-disable MD013 -->

| `ErrorKind` | Meaning | Caller reaction |
| --- | --- | --- |
| `unauthorized` | Credential missing, invalid, or lacking permission for the model. | Fix credentials. |
| `rate_limited` | Provider asked us to back off. | Retry with backoff. |
| `timeout` | Request exceeded `Timeout` or the context was cancelled. | Retry or raise `Timeout`. |
| `invalid_request` | Provider rejected the request body or parameters (400, 422). | Fix the request. |
| `invalid_response` | Provider returned data we could not parse (malformed JSON, missing choices). | Usually a provider hiccup; retry. |
| `unsupported_config` | Caller-side config problem (e.g. schema supplied without a `Name`). | Fix the `Config`/`Schema`. |
| `provider_error` | Catch-all for upstream failures not in a more specific kind (5xx, unknown 4xx). | Retry or escalate. |

<!-- markdownlint-enable MD013 -->

## Adding a provider

Create a new subpackage next to `ai/openai` (e.g. `ai/mycloud`) that implements
`provider.Client`, embedding `provider.Base` for shared HTTP handling, exports a
`Provider` constant, and offers a `NewClient(provider.Config)` constructor. Then
register that constructor in the `clientFactories` map in the root `ai` package.
No calling code changes. Adapters should reject malformed requests before any
network call (see the OpenAI adapter's `validateRequest` for the pattern).
