package generation

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

func TestNormalizeRequestOptionsKeepsProviderSpecificGeometry(t *testing.T) {
	request, err := normalizeRequestOptions(GenerateRequest{Size: "1024x1024", AspectRatio: "16:9", Resolution: "2K", OutputFormat: "webp"})
	if err != nil {
		t.Fatalf("normalizeRequestOptions() error = %v", err)
	}
	if request.Size != "1024x1024" || request.AspectRatio != "16:9" || request.Resolution != "2K" || request.OutputFormat != "webp" {
		t.Fatalf("provider-specific options changed: %#v", request)
	}
	if _, err := normalizeRequestOptions(GenerateRequest{Quality: "ultra"}); err == nil {
		t.Fatalf("unsupported quality should fail")
	}
	if _, err := normalizeRequestOptions(GenerateRequest{OutputFormat: "gif"}); err == nil {
		t.Fatalf("unsupported format should fail")
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
