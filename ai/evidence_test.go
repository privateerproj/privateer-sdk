package ai

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdkconfig "github.com/privateerproj/privateer-sdk/config"
)

func newTestSDKConfig(t *testing.T, writeDir string) sdkconfig.Config {
	t.Helper()
	return sdkconfig.Config{
		ServiceName:    "my-scan",
		WriteDirectory: writeDir,
		Write:          true,
		Vars: map[string]interface{}{
			"owner":             "test-owner",
			"repo":              "test-repo",
			"token":             "ghp_var_secret_123456",
			"ai_provider":       "openai",
			"ai_model":          "gpt-test",
			"ai_api_key":        "super-secret-key",
			"ai_base_url":       "https://proxy-user:proxy-pass@example.test/v1?api_key=query-secret&mode=test",
			"ai_max_tokens":     256,
			"ai_write_evidence": true,
		},
	}
}

func TestWritePacketSucceededAttempt(t *testing.T) {
	tempDir := t.TempDir()
	config := newTestSDKConfig(t, tempDir)
	attempt := PacketAttempt{
		ControlID:         "CTL-QA-06.02",
		Outcome:           "succeeded",
		AttemptStage:      "assessment_completed",
		RepositoryOwner:   "test-owner",
		RepositoryName:    "test-repo",
		DefaultBranch:     "main",
		CommitSHA:         "abc123def456",
		Result:            "Passed",
		Confidence:        "High",
		Verdict:           "pass",
		Reasoning:         "README explains contributors should run go test before opening a PR. Authorization: Bearer sk-live-1234567890abcdef",
		EvidenceLocation:  "README#testing ghp_var_secret_123456",
		AssessmentMessage: "[AI-Assisted] verdict=pass confidence=0.91",
		Prompt:            "You are assessing test execution documentation with super-secret-key.",
		Schema: &Schema{
			Name:        "test_execution_documentation_assessment",
			Description: "Structured assessment.",
			Strict:      true,
			Value:       json.RawMessage(`{"type":"object","properties":{"verdict":{"type":"string"}},"required":["verdict"]}`),
		},
		Evidence: "README\nRun `go test ./...` before opening a PR. token: super-secret-key",
		EvidenceSources: []string{
			"https://github.com/test-owner/test-repo/blob/abc123def456/README.md",
			"https://example.test/source?token=ghp_var_secret_123456&mode=test",
		},
		Response: &AnalyzeResponse{
			Text: "raw model response",
			JSON: []byte(`{"verdict":"pass","confidence":0.91,"reasoning":"README explains workflow. Authorization: Bearer sk-live-1234567890abcdef","evidence_location":"README#testing ghp_var_secret_123456"}`),
			Metadata: ResponseMetadata{
				Provider:     ProviderOpenAI,
				Model:        "gpt-test-2024",
				RequestID:    "req-XYZ_789",
				FinishReason: "stop",
			},
		},
		ExtraSecretValues: []string{"ghp_var_secret_123456"},
	}

	if err := WritePacket(config, attempt); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(tempDir, "my-scan", "ai-evidence", "CTL-QA-06.02", "*"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 packet directory, got %d (%v)", len(matches), matches)
	}
	packetDir := matches[0]

	for _, name := range []string{"assessment.json", "ai_interaction.json"} {
		if _, err := os.Stat(filepath.Join(packetDir, name)); err != nil {
			t.Fatalf("expected packet file %s: %v", name, err)
		}
	}

	assessmentBytes, err := os.ReadFile(filepath.Join(packetDir, "assessment.json"))
	if err != nil {
		t.Fatalf("read assessment: %v", err)
	}
	assessmentText := string(assessmentBytes)
	for _, want := range []string{
		`"packet_version": "1"`,
		`"control_id": "CTL-QA-06.02"`,
		`"service_name": "my-scan"`,
		`"repository_owner": "test-owner"`,
		`"commit_sha": "abc123def456"`,
		`"provider": "openai"`,
		`"model": "gpt-test-2024"`,
		`"request_id": "req-XYZ_789"`,
		`"outcome": "succeeded"`,
		`"attempt_stage": "assessment_completed"`,
		`"result": "Passed"`,
		`"verdict": "pass"`,
		`"api_key": "REDACTED"`,
		`"base_url": "https://REDACTED:REDACTED@example.test/v1?api_key=REDACTED"`,
	} {
		if !strings.Contains(assessmentText, want) {
			t.Fatalf("assessment.json missing %s; got %s", want, assessmentText)
		}
	}

	interactionBytes, err := os.ReadFile(filepath.Join(packetDir, "ai_interaction.json"))
	if err != nil {
		t.Fatalf("read ai_interaction: %v", err)
	}
	interactionText := string(interactionBytes)
	for _, want := range []string{
		`"prompt": "You are assessing test execution documentation with REDACTED."`,
		`"schema":`,
		`"name": "test_execution_documentation_assessment"`,
		`"strict": true`,
		`"evidence":`,
		`"sources":`,
		`"https://github.com/test-owner/test-repo/blob/abc123def456/README.md"`,
		`"https://example.test/source?mode=test\u0026token=REDACTED"`,
		`"content":`,
		`"response":`,
		`"verdict": "pass"`,
	} {
		if !strings.Contains(interactionText, want) {
			t.Fatalf("ai_interaction.json missing %s; got %s", want, interactionText)
		}
	}

	if err := filepath.WalkDir(packetDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		for _, secret := range []string{
			"super-secret-key",
			"proxy-user",
			"proxy-pass",
			"query-secret",
			"ghp_var_secret_123456",
			"sk-live-1234567890abcdef",
		} {
			if strings.Contains(text, secret) {
				t.Fatalf("packet file %s leaked secret %q", path, secret)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("walk: %v", err)
	}
}

func TestWritePacketFailedAttempt(t *testing.T) {
	tempDir := t.TempDir()
	config := newTestSDKConfig(t, tempDir)

	attempt := PacketAttempt{
		ControlID:         "CTL-QA-06.02",
		Outcome:           "failed",
		AttemptStage:      "provider_call",
		AssessmentMessage: "Review project documentation to ensure it explains when and how tests are run",
		Failure:           errors.New("provider unavailable"),
		Prompt:            "prompt",
		Evidence:          "README\nbody",
		EvidenceSources:   []string{"https://github.com/test-owner/test-repo/blob/abc123def456/README.md"},
	}

	if err := WritePacket(config, attempt); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(tempDir, "my-scan", "ai-evidence", "CTL-QA-06.02", "*"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 packet directory, got %d", len(matches))
	}
	body, err := os.ReadFile(filepath.Join(matches[0], "assessment.json"))
	if err != nil {
		t.Fatalf("read assessment: %v", err)
	}
	for _, want := range []string{
		`"provider": "openai"`,
		`"model": "gpt-test"`,
		`"outcome": "failed"`,
		`"attempt_stage": "provider_call"`,
		`"failure_message": "provider unavailable"`,
		`"assessment_message": "Review project documentation to ensure it explains when and how tests are run"`,
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("assessment.json missing %s; got %s", want, string(body))
		}
	}
}

func TestWritePacketFinalRedactionPassCoversMetadataSchemaAndDirectory(t *testing.T) {
	tempDir := t.TempDir()
	config := newTestSDKConfig(t, tempDir)
	escapedSecret := `pa&ss"word\tail<end>`
	config.Vars["token"] = escapedSecret

	attempt := PacketAttempt{
		ControlID:       "CTL-QA-06.02",
		Outcome:         "succeeded",
		AttemptStage:    "assessment_completed",
		RepositoryOwner: "test-owner",
		RepositoryName:  "test-repo",
		DefaultBranch:   "super-secret-key",
		Verdict:         escapedSecret,
		Schema: &Schema{
			Name:        "schema_name",
			Description: "schema description with super-secret-key and " + escapedSecret,
			Strict:      true,
			Value:       json.RawMessage(`{"type":"object","properties":{"token":{"type":"string"}},"required":["token"]}`),
		},
		Response: &AnalyzeResponse{
			JSON: []byte(`{"token":"ghp_response_secret_123456","reasoning":"ok"}`),
			Metadata: ResponseMetadata{
				Provider:  ProviderOpenAI,
				Model:     "gpt-test",
				RequestID: "ghp_request_secret_123456",
			},
		},
	}

	if err := WritePacket(config, attempt); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(tempDir, "my-scan", "ai-evidence", "CTL-QA-06.02", "*"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 packet directory, got %d (%v)", len(matches), matches)
	}
	if strings.Contains(filepath.Base(matches[0]), "ghp_request_secret_123456") {
		t.Fatalf("packet directory leaked request id secret: %s", matches[0])
	}

	if err := filepath.WalkDir(matches[0], func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, secret := range []string{
			"super-secret-key",
			escapedSecret,
			jsonEscapedString(escapedSecret),
			"ghp_response_secret_123456",
			"ghp_request_secret_123456",
		} {
			if strings.Contains(string(body), secret) {
				t.Fatalf("packet file %s leaked secret %q", path, secret)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("walk: %v", err)
	}
}

func TestWritePacketNoopWhenWriteDisabled(t *testing.T) {
	tempDir := t.TempDir()
	config := newTestSDKConfig(t, tempDir)
	config.Write = false

	if err := WritePacket(config, PacketAttempt{ControlID: "CTL-QA-06.02"}); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}
	if entries, _ := os.ReadDir(tempDir); len(entries) != 0 {
		t.Fatalf("expected no packet output, found %d entries", len(entries))
	}
}

func TestWritePacketNoopWhenEvidenceWritingDisabled(t *testing.T) {
	tempDir := t.TempDir()
	config := newTestSDKConfig(t, tempDir)
	config.Vars["ai_write_evidence"] = false

	if err := WritePacket(config, PacketAttempt{ControlID: "CTL-QA-06.02"}); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}
	if entries, _ := os.ReadDir(tempDir); len(entries) != 0 {
		t.Fatalf("expected no packet output when ai_write_evidence is false, found %d entries", len(entries))
	}
}

func TestWritePacketRejectsWrongTypedEvidenceWritingConfig(t *testing.T) {
	tempDir := t.TempDir()
	config := newTestSDKConfig(t, tempDir)
	config.Vars["ai_write_evidence"] = "true"

	err := WritePacket(config, PacketAttempt{ControlID: "CTL-QA-06.02"})
	if err == nil {
		t.Fatal("expected wrong-typed ai_write_evidence error, got nil")
	}
	if !strings.Contains(err.Error(), "ai_write_evidence must be a bool") {
		t.Fatalf("error = %q, want ai_write_evidence type error", err.Error())
	}
	if entries, _ := os.ReadDir(tempDir); len(entries) != 0 {
		t.Fatalf("expected no packet output when ai_write_evidence is wrong-typed, found %d entries", len(entries))
	}
}

func TestWritePacketNoopWhenAIUnconfigured(t *testing.T) {
	tempDir := t.TempDir()
	config := newTestSDKConfig(t, tempDir)
	config.Vars = map[string]interface{}{
		"ai_write_evidence": true,
	}

	if err := WritePacket(config, PacketAttempt{ControlID: "CTL-QA-06.02"}); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}
	if entries, _ := os.ReadDir(tempDir); len(entries) != 0 {
		t.Fatalf("expected no packet output when AI is unconfigured, found %d entries", len(entries))
	}
}

func TestWritePacketNoopWhenControlIDMissing(t *testing.T) {
	tempDir := t.TempDir()
	config := newTestSDKConfig(t, tempDir)

	if err := WritePacket(config, PacketAttempt{}); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}
	if entries, _ := os.ReadDir(tempDir); len(entries) != 0 {
		t.Fatalf("expected no packet output when ControlID is empty, found %d", len(entries))
	}
}

func TestCreatePacketDirectoryFailsWhenPacketDirectoryExists(t *testing.T) {
	packetDir := filepath.Join(t.TempDir(), "my-scan", "ai-evidence", "CTL-QA-06.02", "packet")
	if err := createPacketDirectory(packetDir); err != nil {
		t.Fatalf("createPacketDirectory first call: %v", err)
	}
	if err := createPacketDirectory(packetDir); err == nil {
		t.Fatal("createPacketDirectory second call succeeded, want collision error")
	}
}

func TestSanitizerRedactsConfiguredAndPatternSecrets(t *testing.T) {
	config := newTestSDKConfig(t, t.TempDir())
	config.Vars["token_list"] = []interface{}{"list-secret-token", 123, ""}
	sanitizer := NewSanitizer(config, "extra-secret-token")

	got := sanitizer.RedactText("super-secret-key extra-secret-token list-secret-token Authorization: Bearer sk-abcdefg12345 ghp_abcdefg12345")
	for _, leaked := range []string{"super-secret-key", "extra-secret-token", "list-secret-token", "sk-abcdefg12345", "ghp_abcdefg12345"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("Sanitizer leaked %q in %q", leaked, got)
		}
	}
}

func TestNewSanitizerWithConfigReturnsAIConfigErrors(t *testing.T) {
	config := newTestSDKConfig(t, t.TempDir())
	config.Vars["ai_timeout"] = "bad-timeout"

	sanitizer, err := NewSanitizerWithConfig(config, "extra-secret-token")
	if err == nil {
		t.Fatal("expected invalid AI config error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid ai_timeout") {
		t.Fatalf("error = %q, want invalid ai_timeout", err.Error())
	}

	got := sanitizer.RedactText("super-secret-key extra-secret-token")
	for _, leaked := range []string{"super-secret-key", "extra-secret-token"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("Sanitizer leaked %q in %q", leaked, got)
		}
	}
}

func TestNewSanitizerRemainsBestEffortWhenAIConfigIsInvalid(t *testing.T) {
	config := newTestSDKConfig(t, t.TempDir())
	config.Vars["ai_timeout"] = "bad-timeout"

	sanitizer := NewSanitizer(config, "extra-secret-token")
	got := sanitizer.RedactText("super-secret-key extra-secret-token")
	for _, leaked := range []string{"super-secret-key", "extra-secret-token"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("Sanitizer leaked %q in %q", leaked, got)
		}
	}
}

func TestSanitizerDoesNotTreatAuthorAsSensitiveKey(t *testing.T) {
	config := newTestSDKConfig(t, t.TempDir())
	config.Vars["author"] = "Jane Doe"
	config.Vars["auth_token"] = "real-secret-token"
	config.Vars["authorization"] = "Bearer authorization-secret"
	config.Vars["client-secret"] = "client-secret-value"
	config.Vars["service.api-key"] = "api-key-value"
	config.Vars["accessToken"] = "access-token-value"
	config.Vars["clientSecret"] = "client-secret-camel-value"
	config.Vars["APIKey"] = "api-key-initialism-value"
	sanitizer := NewSanitizer(config)

	got := sanitizer.RedactText("Jane Doe real-secret-token Bearer authorization-secret client-secret-value api-key-value access-token-value client-secret-camel-value api-key-initialism-value")
	if !strings.Contains(got, "Jane Doe") {
		t.Fatalf("Sanitizer unexpectedly redacted author value: %q", got)
	}
	for _, leaked := range []string{"real-secret-token", "Bearer authorization-secret", "client-secret-value", "api-key-value", "access-token-value", "client-secret-camel-value", "api-key-initialism-value"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("Sanitizer leaked sensitive key value %q in %q", leaked, got)
		}
	}
}

func TestSanitizeURLRedactsCredentialsAndQuery(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "credentials and query", input: "https://proxy-user:proxy-pass@example.test/v1?api_key=secret&mode=test", want: "https://REDACTED:REDACTED@example.test/v1?api_key=REDACTED&mode=test"},
		{name: "authorization query preserves author", input: "https://example.test/v1?authorization=bearer-secret&author=Jane&mode=test", want: "https://example.test/v1?author=Jane&authorization=REDACTED&mode=test"},
		{name: "invalid", input: "://bad url", want: "REDACTED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeURL(tt.input); got != tt.want {
				t.Fatalf("SanitizeURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRedactPatterns(t *testing.T) {
	input := strings.Join([]string{
		"Authorization: Bearer sk-live-1234567890abcdef",
		"api_key=plain-secret",
		"authorization=bearer-secret",
		"authorization=Bearer bearer-assignment-secret",
		"password=abc,def;ghi&jkl\\tail",
		"secret=abc&next=still-secret",
		`token=abc\u0026mode=still-secret`,
		`api_key=REDACTED\u0026mode=test`,
		"password=REDACTED&still-secret",
		"password=REDACTED&still=secret",
		"password=REDACTED-real-secret",
		"password=Bearer",
		"token: ghp_secret_12345678",
		`{"token":"ghp_json_secret_12345678","api_key":"plain-json-secret","authorization":"json-authorization-secret","password":"quoted\"secret-value"}`,
	}, "\n")
	got := RedactPatterns(input)
	for _, leaked := range []string{"sk-live-1234567890abcdef", "plain-secret", "bearer-secret", "bearer-assignment-secret", "abc,def;ghi&jkl\\tail", "abc&next=still-secret", `abc\u0026mode=still-secret`, "REDACTED&still-secret", "REDACTED&still=secret", "REDACTED-real-secret", "ghp_secret_12345678", "ghp_json_secret_12345678", "plain-json-secret", "json-authorization-secret", `quoted\"secret-value`} {
		if strings.Contains(got, leaked) {
			t.Fatalf("RedactPatterns leaked %q in %q", leaked, got)
		}
	}
	for _, want := range []string{"Authorization: Bearer REDACTED", "api_key=REDACTED", "authorization=REDACTED", "authorization=Bearer REDACTED", "password=REDACTED", "secret=REDACTED", `token=REDACTED`, "token: REDACTED", `"token":"REDACTED"`, `"api_key":"REDACTED"`, `"authorization":"REDACTED"`, `"password":"REDACTED"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("RedactPatterns missing %q in %q", want, got)
		}
	}
	inputJSON := strings.Split(input, "\n")[13]
	gotJSON := strings.Split(got, "\n")[13]
	if !json.Valid([]byte(inputJSON)) {
		t.Fatal("test fixture must remain valid JSON")
	}
	if !json.Valid([]byte(gotJSON)) {
		t.Fatalf("RedactPatterns produced invalid JSON object: %q", gotJSON)
	}
}

func TestRedactConfigValue(t *testing.T) {
	if got := RedactConfigValue(""); got != "" {
		t.Fatalf("empty input: got %q", got)
	}
	if got := RedactConfigValue("  "); got != "" {
		t.Fatalf("whitespace input: got %q", got)
	}
	if got := RedactConfigValue("anything"); got != "REDACTED" {
		t.Fatalf("non-empty input: got %q", got)
	}
}
