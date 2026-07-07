package imagegen

import (
	"strings"
	"testing"
)

func TestNormalizeRequestOptionsAcceptsConfiguredResolutionList(t *testing.T) {
	request, err := normalizeRequestOptions(GenerateRequest{
		Size:         "4096x2304",
		OutputFormat: "jpeg",
	})
	if err != nil {
		t.Fatalf("normalizeRequestOptions() error = %v", err)
	}
	if request.Size != "4096x2304" || request.OutputFormat != "jpeg" {
		t.Fatalf("normalized request mismatch: %#v", request)
	}
}

func TestNormalizeRequestOptionsRejectsUnsupportedResolutionAndFormat(t *testing.T) {
	if _, err := normalizeRequestOptions(GenerateRequest{Size: "1024x1024"}); err == nil {
		t.Fatalf("1024x1024 should no longer be accepted")
	}
	if _, err := normalizeRequestOptions(GenerateRequest{OutputFormat: "webp"}); err == nil {
		t.Fatalf("webp should no longer be accepted")
	}
}

func TestNormalizeRequestOptionsTreatsAutoSizeAsUnset(t *testing.T) {
	request, err := normalizeRequestOptions(GenerateRequest{Size: "auto"})
	if err != nil {
		t.Fatalf("normalizeRequestOptions() error = %v", err)
	}
	if request.Size != "" {
		t.Fatalf("auto size should be unset, got %q", request.Size)
	}
}

func TestPromptSummaryDoesNotExposeFullPrompt(t *testing.T) {
	prompt := strings.Repeat("private prompt content ", 20)
	summary := promptSummary(prompt)
	if strings.Contains(summary, prompt) {
		t.Fatalf("prompt summary should not include full prompt: %s", summary)
	}
	if !strings.Contains(summary, "hash=sha256:") || !strings.Contains(summary, "chars=") || !strings.Contains(summary, "preview=") {
		t.Fatalf("prompt summary should include bounded diagnostics: %s", summary)
	}
}
