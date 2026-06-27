package app

import (
	"testing"

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
