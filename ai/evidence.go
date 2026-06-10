package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	sdkconfig "github.com/privateerproj/privateer-sdk/config"
)

// DefaultPacketVersion is the schema version stamped into assessment.json
// when PacketAttempt.PacketVersion is not set.
const DefaultPacketVersion = "1"

// PacketAttempt is the provider-neutral input for one AI evidence packet.
// It carries everything WritePacket needs to persist a reviewable per-attempt
// artifact set under <write-dir>/<service>/ai-evidence/<control-id>/. Fields
// are intentionally generic so future AI-assisted controls (and future
// providers) can reuse the same writer without modification.
type PacketAttempt struct {
	// ControlID is the OSPS control identifier (e.g. "OSPS-QA-06.02").
	// Required: it becomes the parent directory under ai-evidence/.
	ControlID string

	// PacketVersion is the schema version stamped into assessment.json.
	// Defaults to DefaultPacketVersion when empty.
	PacketVersion string

	// Repository context. Optional, included in assessment.json when set.
	RepositoryOwner string
	RepositoryName  string
	DefaultBranch   string
	CommitSHA       string

	// Outcome is a free-form lifecycle label, typically "succeeded" or
	// "failed". AttemptStage describes where in the attempt the writer was
	// called from (e.g. "client_construction", "provider_call",
	// "schema_validation", "assessment_completed").
	Outcome      string
	AttemptStage string

	// Result, Confidence, and Verdict are the caller-domain summary fields
	// rendered into assessment.json. They are strings so the SDK does not
	// depend on any particular verdict type.
	Result     string
	Confidence string
	Verdict    string

	// Reasoning and EvidenceLocation come from the parsed structured AI
	// response. The writer sanitizes them using configured values and
	// token-shaped patterns before persistence.
	Reasoning        string
	EvidenceLocation string

	// AssessmentMessage is the human-readable summary that the caller
	// surfaces (e.g. in scanner logs). FailureMessage is derived from
	// Failure when set.
	AssessmentMessage string
	Failure           error

	// Provider interaction artifacts. The writer sanitizes prompt, evidence,
	// and response content before persistence. Schema is copied for
	// auditability, and the final JSON bytes pass through the same sanitizer
	// as a defense-in-depth layer before disk write.
	Prompt          string
	Schema          *Schema
	Evidence        string
	EvidenceSources []string
	Response        *AnalyzeResponse

	// ExtraSecretValues lets callers register additional secret strings
	// (e.g. GitHub tokens read from config.Vars) so the sanitizer can redact
	// those exact values when they appear in packet text.
	ExtraSecretValues []string
}

// AssessmentFile is the per-attempt summary written to assessment.json. It
// captures the verdict mapping, attempt context, and redacted AI runtime
// metadata in one human-readable file.
type AssessmentFile struct {
	PacketVersion     string                 `json:"packet_version"`
	CapturedAt        string                 `json:"captured_at"`
	ControlID         string                 `json:"control_id"`
	ServiceName       string                 `json:"service_name"`
	RepositoryOwner   string                 `json:"repository_owner,omitempty"`
	RepositoryName    string                 `json:"repository_name,omitempty"`
	DefaultBranch     string                 `json:"default_branch,omitempty"`
	CommitSHA         string                 `json:"commit_sha,omitempty"`
	Provider          Provider               `json:"provider"`
	Model             string                 `json:"model"`
	RequestID         string                 `json:"request_id,omitempty"`
	FinishReason      string                 `json:"finish_reason,omitempty"`
	Outcome           string                 `json:"outcome"`
	AttemptStage      string                 `json:"attempt_stage,omitempty"`
	Result            string                 `json:"result,omitempty"`
	Confidence        string                 `json:"confidence,omitempty"`
	AssessmentMessage string                 `json:"assessment_message,omitempty"`
	FailureMessage    string                 `json:"failure_message,omitempty"`
	Verdict           string                 `json:"verdict,omitempty"`
	Reasoning         string                 `json:"reasoning,omitempty"`
	EvidenceLocation  string                 `json:"evidence_location,omitempty"`
	RedactedConfig    map[string]interface{} `json:"redacted_config"`
}

// AIInteractionFile is the per-attempt file written to ai_interaction.json. It
// holds the prompt, the schema, the exact evidence body sent to the model,
// the source locations the evidence was assembled from, and the raw
// structured response, so a reviewer can reproduce or audit the verdict.
type AIInteractionFile struct {
	Prompt   string                `json:"prompt,omitempty"`
	Schema   *AIInteractionSchema  `json:"schema,omitempty"`
	Evidence AIInteractionEvidence `json:"evidence"`
	Response json.RawMessage       `json:"response,omitempty"`
}

// AIInteractionSchema is the persisted view of the JSON Schema used for the
// structured response. It is a plain serializable copy of ai.Schema.
type AIInteractionSchema struct {
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Strict      bool            `json:"strict"`
	Value       json.RawMessage `json:"value,omitempty"`
}

// AIInteractionEvidence groups the evidence body and the source locations it
// was assembled from. Sources are typically commit-pinned URLs.
type AIInteractionEvidence struct {
	Sources []string `json:"sources,omitempty"`
	Content string   `json:"content,omitempty"`
}

// WritePacket persists a per-attempt AI evidence packet under
// <write-dir>/<service>/ai-evidence/<control-id>/<timestamp>-<request-id>/
// honoring the SDK config's Write flag and ai_write_evidence switch. It returns
// nil (no-op) when writes are disabled, AI is not configured, AI evidence
// writing is not explicitly enabled, WriteDirectory is empty, or ControlID is
// empty.
//
// Two files are written per attempt:
//   - assessment.json: verdict, attempt stage, outcome, repository context,
//     and redacted AI runtime metadata.
//   - ai_interaction.json: prompt, schema, evidence (sources + redacted body
//     sent to the model), and the raw structured AI response.
//
// Before bytes are written to disk, packet content is sanitized using known
// configured values (APIKey, BaseURL credentials, sensitive config.Vars, and
// any ExtraSecretValues supplied by the caller) plus pattern-based scrubbers
// for bearer tokens and other common token-shaped values.
func WritePacket(config sdkconfig.Config, attempt PacketAttempt) error {
	if !config.Write {
		return nil
	}
	evidenceEnabled, err := evidenceWritingEnabled(config)
	if err != nil {
		return err
	}
	if !evidenceEnabled {
		return nil
	}
	controlID := strings.TrimSpace(attempt.ControlID)
	if controlID == "" {
		return nil
	}
	writeDir := strings.TrimSpace(config.WriteDirectory)
	if writeDir == "" {
		return nil
	}

	serviceName := strings.TrimSpace(config.ServiceName)
	if serviceName == "" {
		serviceName = "overview"
	}

	aiConfig, configured, err := ConfigFromSDKConfig(config)
	if err != nil {
		return err
	}
	// Evidence packets contain the prompt, evidence body, and model response, so
	// callers must opt in twice: configure AI assessment and enable packet writing.
	if !configured {
		return nil
	}
	sanitizer := newPacketSanitizer(config, aiConfig, attempt.ExtraSecretValues)

	packetDir := filepath.Join(
		writeDir,
		serviceName,
		"ai-evidence",
		controlID,
		packetDirectoryName(sanitizer.RedactText(packetRequestID(attempt.Response))),
	)
	if err := os.MkdirAll(packetDir, 0o750); err != nil {
		return err
	}

	packetVersion := strings.TrimSpace(attempt.PacketVersion)
	if packetVersion == "" {
		packetVersion = DefaultPacketVersion
	}

	assessmentFile := AssessmentFile{
		PacketVersion:     packetVersion,
		CapturedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		ControlID:         controlID,
		ServiceName:       serviceName,
		RepositoryOwner:   strings.TrimSpace(attempt.RepositoryOwner),
		RepositoryName:    strings.TrimSpace(attempt.RepositoryName),
		DefaultBranch:     strings.TrimSpace(attempt.DefaultBranch),
		CommitSHA:         strings.TrimSpace(attempt.CommitSHA),
		Provider:          packetProvider(attempt.Response, aiConfig),
		Model:             packetModel(attempt.Response, aiConfig),
		RequestID:         packetRequestID(attempt.Response),
		FinishReason:      packetFinishReason(attempt.Response),
		Outcome:           strings.TrimSpace(attempt.Outcome),
		AttemptStage:      strings.TrimSpace(attempt.AttemptStage),
		Result:            strings.TrimSpace(attempt.Result),
		Confidence:        strings.TrimSpace(attempt.Confidence),
		AssessmentMessage: sanitizer.RedactText(attempt.AssessmentMessage),
		FailureMessage:    sanitizer.RedactText(errorMessage(attempt.Failure)),
		Verdict:           strings.TrimSpace(attempt.Verdict),
		Reasoning:         sanitizer.RedactText(attempt.Reasoning),
		EvidenceLocation:  sanitizer.RedactText(attempt.EvidenceLocation),
		RedactedConfig: map[string]interface{}{
			"provider":   aiConfig.Provider,
			"model":      aiConfig.Model,
			"base_url":   SanitizeURL(aiConfig.BaseURL),
			"timeout":    aiConfig.Timeout.String(),
			"max_tokens": aiConfig.MaxTokens,
			"api_key":    RedactConfigValue(aiConfig.APIKey),
		},
	}

	if err := writePacketJSON(filepath.Join(packetDir, "assessment.json"), assessmentFile, sanitizer); err != nil {
		return err
	}

	interaction := buildAIInteraction(attempt, sanitizer)
	if err := writePacketJSON(filepath.Join(packetDir, "ai_interaction.json"), interaction, sanitizer); err != nil {
		return err
	}
	return nil
}

func evidenceWritingEnabled(config sdkconfig.Config) (bool, error) {
	return getSDKConfigBool(config, "ai_write_evidence")
}

func buildAIInteraction(attempt PacketAttempt, sanitizer Sanitizer) AIInteractionFile {
	file := AIInteractionFile{
		Prompt: sanitizer.RedactText(attempt.Prompt),
		Evidence: AIInteractionEvidence{
			Sources: sanitizeEvidenceSources(attempt.EvidenceSources, sanitizer),
			Content: sanitizer.RedactText(attempt.Evidence),
		},
	}
	if attempt.Schema != nil {
		file.Schema = &AIInteractionSchema{
			Name:        attempt.Schema.Name,
			Description: attempt.Schema.Description,
			Strict:      attempt.Schema.Strict,
			Value:       append(json.RawMessage(nil), attempt.Schema.Value...),
		}
	}
	if attempt.Response != nil && len(attempt.Response.JSON) > 0 {
		sanitized := sanitizer.RedactText(string(attempt.Response.JSON))
		if json.Valid([]byte(sanitized)) {
			file.Response = json.RawMessage(sanitized)
		} else if encoded, err := json.Marshal(map[string]string{"raw_text": sanitized}); err == nil {
			file.Response = encoded
		}
	}
	return file
}

func sanitizeEvidenceSources(sources []string, sanitizer Sanitizer) []string {
	if len(sources) == 0 {
		return nil
	}
	sanitized := make([]string, 0, len(sources))
	for _, source := range sources {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		sanitized = append(sanitized, sanitizer.RedactText(SanitizeURL(source)))
	}
	return sanitized
}

func packetDirectoryName(requestID string) string {
	base := time.Now().UTC().Format("20060102T150405.000000000Z")
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return base + "-no-request-id"
	}

	var builder strings.Builder
	for _, r := range requestID {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	safe := strings.Trim(builder.String(), "._-")
	if safe == "" {
		safe = "request-id"
	}
	if len(safe) > 48 {
		safe = safe[:48]
	}
	return base + "-" + safe
}

func packetProvider(response *AnalyzeResponse, config Config) Provider {
	if response != nil && response.Metadata.Provider != "" {
		return response.Metadata.Provider
	}
	return config.Provider
}

func packetModel(response *AnalyzeResponse, config Config) string {
	if response != nil && strings.TrimSpace(response.Metadata.Model) != "" {
		return response.Metadata.Model
	}
	return config.Model
}

func packetRequestID(response *AnalyzeResponse) string {
	if response == nil {
		return ""
	}
	return response.Metadata.RequestID
}

func packetFinishReason(response *AnalyzeResponse) string {
	if response == nil {
		return ""
	}
	return response.Metadata.FinishReason
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Sanitizer is a provider-neutral text scrubber for AI evidence packets.
// It performs exact-value replacement for known sensitive values gathered
// from config plus pattern-based redaction for common token-shaped strings
// that may appear in evidence or model output.
type Sanitizer struct {
	secretValues []string
}

// NewSanitizer assembles a Sanitizer for the given SDK config. It extracts
// known sensitive values (AI API key, credentials embedded in ai_base_url,
// and any config.Vars whose key looks sensitive) and combines them with
// the supplied extraSecretValues. Callers can use the returned Sanitizer
// directly to sanitize log output before WritePacket runs.
func NewSanitizer(config sdkconfig.Config, extraSecretValues ...string) Sanitizer {
	aiConfig, _, _ := ConfigFromSDKConfig(config)
	return newPacketSanitizer(config, aiConfig, extraSecretValues)
}

func newPacketSanitizer(config sdkconfig.Config, aiConfig Config, extraSecretValues []string) Sanitizer {
	secretValues := []string{}
	seen := map[string]struct{}{}
	addSecretValue := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || value == "REDACTED" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		secretValues = append(secretValues, value)
	}

	addSecretValue(aiConfig.APIKey)
	for _, value := range urlSecretValues(aiConfig.BaseURL) {
		addSecretValue(value)
	}
	for key, value := range config.Vars {
		if !isSensitivePacketKey(key) {
			continue
		}
		for _, secretValue := range packetSecretStrings(value) {
			addSecretValue(secretValue)
		}
	}
	for _, value := range extraSecretValues {
		addSecretValue(value)
	}

	sort.Slice(secretValues, func(i, j int) bool {
		return len(secretValues[i]) > len(secretValues[j])
	})

	return Sanitizer{secretValues: secretValues}
}

// RedactText performs exact-value replacement followed by pattern-based
// scrubbing and returns the sanitized string.
func (s Sanitizer) RedactText(value string) string {
	for _, secretValue := range s.secretValues {
		value = strings.ReplaceAll(value, secretValue, "REDACTED")
	}
	return RedactPatterns(value)
}

// RedactConfigValue returns the empty string for empty input and "REDACTED"
// for any other non-empty value. It is the standard renderer for known
// secret config fields (such as ai_api_key) in packet metadata.
func RedactConfigValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "REDACTED"
}

// SanitizeURL preserves the structure of a URL while redacting embedded
// credentials and the values of any sensitive query parameters. Unparseable
// values are collapsed to "REDACTED".
func SanitizeURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return RedactConfigValue(rawURL)
	}

	if parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			parsed.User = url.UserPassword("REDACTED", "REDACTED")
		} else {
			parsed.User = url.User("REDACTED")
		}
	}

	query := parsed.Query()
	queryChanged := false
	for key, values := range query {
		if !isSensitivePacketKey(key) {
			continue
		}
		queryChanged = true
		for index := range values {
			values[index] = "REDACTED"
		}
		query[key] = values
	}
	if queryChanged {
		parsed.RawQuery = query.Encode()
	}

	return parsed.String()
}

// packetPatternRedactors is a defensive fallback for secrets that are not
// present in config but still appear in evidence or model output.
var packetPatternRedactors = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	{
		pattern:     regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)([^\s"'` + "`" + `]+)`),
		replacement: `${1}REDACTED`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)(\b(?:api[_-]?key|token|secret|password|auth)\b\s*[:=]\s*)([^\s"'` + "`" + `,;&\\]+)`),
		replacement: `${1}REDACTED`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)("(?:api[_-]?key|token|secret|password|auth)"\s*:\s*")([^"]+)(")`),
		replacement: `${1}REDACTED${3}`,
	},
	{
		pattern:     regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{8,}\b`),
		replacement: `REDACTED`,
	},
	{
		pattern:     regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}\b`),
		replacement: `REDACTED`,
	},
}

// RedactPatterns applies the package's pattern-based token scrubbers to the
// supplied string. It is exposed so callers can redact text destined for
// places other than the packet itself (such as scanner logs) using the same
// rules as WritePacket.
func RedactPatterns(value string) string {
	for _, redactor := range packetPatternRedactors {
		value = redactor.pattern.ReplaceAllString(value, redactor.replacement)
	}
	return value
}

func urlSecretValues(rawURL string) []string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil
	}

	secretValues := []string{}
	if parsed.User != nil {
		if username := strings.TrimSpace(parsed.User.Username()); username != "" {
			secretValues = append(secretValues, username)
		}
		if password, hasPassword := parsed.User.Password(); hasPassword && strings.TrimSpace(password) != "" {
			secretValues = append(secretValues, password)
		}
	}
	for key, values := range parsed.Query() {
		if !isSensitivePacketKey(key) {
			continue
		}
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				secretValues = append(secretValues, value)
			}
		}
	}

	return secretValues
}

func isSensitivePacketKey(key string) bool {
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	for _, pattern := range []string{"token", "auth", "password", "secret", "apikey", "api_key"} {
		if strings.Contains(lowerKey, pattern) {
			return true
		}
	}
	return false
}

func packetSecretStrings(value interface{}) []string {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{typed}
	case []string:
		secretValues := []string{}
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				secretValues = append(secretValues, item)
			}
		}
		return secretValues
	case []byte:
		if strings.TrimSpace(string(typed)) == "" {
			return nil
		}
		return []string{string(typed)}
	default:
		return nil
	}
}

func writePacketJSON(path string, value interface{}, sanitizer Sanitizer) error {
	var body []byte
	var err error
	if rawJSON, ok := value.(json.RawMessage); ok {
		var formatted bytes.Buffer
		if err = json.Indent(&formatted, rawJSON, "", "  "); err != nil {
			return fmt.Errorf("indent json for %s: %w", path, err)
		}
		body = formatted.Bytes()
	} else {
		body, err = json.MarshalIndent(value, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal json for %s: %w", path, err)
		}
	}
	body = []byte(sanitizer.RedactText(string(body)))
	if !json.Valid(body) {
		return fmt.Errorf("redacted json for %s is invalid", path)
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o640)
}
