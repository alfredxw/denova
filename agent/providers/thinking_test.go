package providers

import (
	"slices"
	"testing"
)

func TestThinkingLevelsExcludeMinimal(t *testing.T) {
	want := []ThinkingLevel{
		ThinkingLevelDefault,
		ThinkingLevelOff,
		ThinkingLevelLow,
		ThinkingLevelMedium,
		ThinkingLevelHigh,
		ThinkingLevelXHigh,
		ThinkingLevelMax,
	}
	if got := ThinkingLevels(); !slices.Equal(got, want) {
		t.Fatalf("thinking levels = %v, want %v", got, want)
	}
}

func TestMinimalThinkingLevelNormalizesToLow(t *testing.T) {
	if got := NormalizeThinkingLevel("minimal"); got != ThinkingLevelLow {
		t.Fatalf("normalized minimal thinking level = %q, want low", got)
	}
	got, err := ParseThinkingLevel("minimal")
	if err != nil || got != ThinkingLevelLow {
		t.Fatalf("parsed minimal thinking level = %q, %v; want low", got, err)
	}
}
