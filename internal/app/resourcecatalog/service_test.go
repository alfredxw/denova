package resourcecatalog

import (
	"os"
	"testing"

	"denova/internal/presetlayout"
)

func TestServiceUsesUnifiedPresetLayoutForSharedAndGameCatalogs(t *testing.T) {
	dataRoot := t.TempDir()
	service := NewService(dataRoot, nil)

	tellers, err := service.Tellers()
	if err != nil || len(tellers) == 0 {
		t.Fatalf("shared narrative styles = %d, err=%v", len(tellers), err)
	}
	images, err := service.ImagePresets()
	if err != nil || len(images) == 0 {
		t.Fatalf("shared image presets = %d, err=%v", len(images), err)
	}
	planning, err := service.GamePlanningTemplates()
	if err != nil || len(planning) == 0 {
		t.Fatalf("game planning templates = %d, err=%v", len(planning), err)
	}
	events, err := service.EventPackages()
	if err != nil || len(events) == 0 {
		t.Fatalf("game event packages = %d, err=%v", len(events), err)
	}
	rules, err := service.RuleSystems()
	if err != nil || len(rules) == 0 {
		t.Fatalf("game rule systems = %d, err=%v", len(rules), err)
	}
	states, err := service.ActorStates()
	if err != nil || len(states) == 0 {
		t.Fatalf("game actor states = %d, err=%v", len(states), err)
	}

	for _, path := range []string{
		presetlayout.NarrativeStyles(dataRoot),
		presetlayout.Image(dataRoot),
		presetlayout.GamePlanning(dataRoot),
		presetlayout.EventPackages(dataRoot),
		presetlayout.RuleSystems(dataRoot),
		presetlayout.ActorStates(dataRoot),
	} {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Fatalf("preset catalog directory %s: info=%v err=%v", path, info, err)
		}
	}
}
