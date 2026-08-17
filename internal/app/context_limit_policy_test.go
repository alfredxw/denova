package app

import (
	"testing"

	configmanagerapp "denova/internal/app/configmanager"
	interactiveapp "denova/internal/app/interactive"
	"denova/internal/interactive"
)

func TestCompleteGameAndSkillContextLimitsAreAbove128KB(t *testing.T) {
	const minimumBytes = 128 * 1024
	limits := map[string]int{
		"game runtime context fragment":       interactiveapp.StoryRuntimeContextMaxBytes,
		"game resolved lore context":          interactiveapp.ResolvedLoreContextMaxBytes,
		"director context fragment":           interactive.DirectorContextMaxBytes,
		"director active lore":                interactive.DirectorLoreActiveContextMaxBytes,
		"config manager resource skill":       configmanagerapp.ResourceSkillMaxSourceBytes,
		"config manager resource skill total": configmanagerapp.ResourceSkillMaxTotalSourceBytes,
	}
	for name, limit := range limits {
		if limit <= minimumBytes {
			t.Errorf("%s limit = %d bytes, want above %d", name, limit, minimumBytes)
		}
	}
}
