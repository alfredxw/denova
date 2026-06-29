package app

import (
	"strings"
	"testing"

	"nova/internal/imagepreset"
	"nova/internal/interactive"
)

func TestShouldGenerateInteractiveImageModes(t *testing.T) {
	turns := []interactive.TurnEvent{{ID: "t1"}, {ID: "t2"}, {ID: "t3"}}
	tests := []struct {
		name     string
		settings interactive.StoryImageSettings
		index    int
		source   string
		force    bool
		want     bool
		reason   string
	}{
		{name: "manual auto skip", settings: interactive.StoryImageSettings{Mode: interactive.StoryImageModeManual, IntervalTurns: 3}, index: 0, source: interactiveImageSourceAuto, want: false, reason: "manual_mode"},
		{name: "manual click generate", settings: interactive.StoryImageSettings{Mode: interactive.StoryImageModeManual, IntervalTurns: 3}, index: 0, source: interactiveImageSourceManual, want: true},
		{name: "one turn interval auto generate", settings: interactive.StoryImageSettings{Mode: interactive.StoryImageModeInterval, IntervalTurns: 1}, index: 0, source: interactiveImageSourceAuto, want: true},
		{name: "interval wait", settings: interactive.StoryImageSettings{Mode: interactive.StoryImageModeInterval, IntervalTurns: 3}, index: 1, source: interactiveImageSourceAuto, want: false, reason: "interval"},
		{name: "interval hit", settings: interactive.StoryImageSettings{Mode: interactive.StoryImageModeInterval, IntervalTurns: 3}, index: 2, source: interactiveImageSourceAuto, want: true},
		{name: "force ignores mode", settings: interactive.StoryImageSettings{Mode: interactive.StoryImageModeManual, IntervalTurns: 3}, index: 0, source: interactiveImageSourceAuto, force: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := shouldGenerateInteractiveImage(tt.settings, turns, tt.index, tt.source, tt.force)
			if got != tt.want || reason != tt.reason {
				t.Fatalf("shouldGenerateInteractiveImage = (%v, %q), want (%v, %q)", got, reason, tt.want, tt.reason)
			}
		})
	}
}

func TestInteractiveImageSystemPromptUsesImagePreset(t *testing.T) {
	prompt := interactiveImageSystemPrompt(imagepreset.Preset{
		ID:   "realistic",
		Name: "写实",
		Slots: []imagepreset.Slot{
			{ID: "system", Name: "系统", Target: imagepreset.TargetAgentSystem, Enabled: true, Content: "理解真实光影。"},
			{ID: "tool", Name: "请求", Target: imagepreset.TargetToolRequest, Enabled: true, Content: "原样请求风格。"},
		},
	})
	if !strings.Contains(prompt, "图像方案预设") || !strings.Contains(prompt, "理解真实光影") {
		t.Fatalf("system prompt should include image preset:\n%s", prompt)
	}
	if strings.Contains(prompt, "原样请求风格") || strings.Contains(prompt, "image_prompt") || strings.Contains(prompt, "叙事编排") {
		t.Fatalf("system prompt should not mention legacy teller image_prompt:\n%s", prompt)
	}
}
