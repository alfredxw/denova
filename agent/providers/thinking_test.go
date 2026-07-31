package providers

import "testing"

func TestThinkingLevelsAreCompleteAndStable(t *testing.T) {
	want := []ThinkingLevel{
		ThinkingLevelDefault,
		ThinkingLevelOff,
		ThinkingLevelMinimal,
		ThinkingLevelLow,
		ThinkingLevelMedium,
		ThinkingLevelHigh,
		ThinkingLevelXHigh,
		ThinkingLevelMax,
	}
	got := ThinkingLevels()
	if len(got) != len(want) {
		t.Fatalf("thinking levels = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("thinking levels = %v, want %v", got, want)
		}
	}
	got[0] = "mutated"
	if ThinkingLevels()[0] != ThinkingLevelDefault {
		t.Fatal("ThinkingLevels returned mutable catalog storage")
	}
}

func TestParseThinkingLevelNormalizesSupportedAliases(t *testing.T) {
	tests := map[string]ThinkingLevel{
		"":              ThinkingLevelDefault,
		"model_default": ThinkingLevelDefault,
		"none":          ThinkingLevelOff,
		"light":         ThinkingLevelLow,
		"extra high":    ThinkingLevelXHigh,
		"maximum":       ThinkingLevelMax,
	}
	for input, want := range tests {
		got, err := ParseThinkingLevel(input)
		if err != nil || got != want {
			t.Fatalf("ParseThinkingLevel(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := ParseThinkingLevel("turbo"); err == nil {
		t.Fatal("expected unknown thinking level to fail")
	}
}
