package app

import (
	"testing"

	"denova/internal/interactive"
)

func TestCompleteGameAndSkillContextLimitsAreAbove128KB(t *testing.T) {
	const minimumBytes = 128 * 1024
	limits := map[string]int{
		"game runtime context fragment":       interactiveStoryRuntimeContextBytes,
		"game resolved lore context":          interactiveResolvedLoreContextMaxBytes,
		"director context fragment":           interactive.DirectorContextMaxBytes,
		"director active lore":                interactive.DirectorLoreActiveContextMaxBytes,
		"config manager resource skill":       configManagerResourceSkillMaxSourceBytes,
		"config manager resource skill total": configManagerResourceSkillMaxTotalSourceBytes,
	}
	for name, limit := range limits {
		if limit <= minimumBytes {
			t.Errorf("%s limit = %d bytes, want above %d", name, limit, minimumBytes)
		}
	}
}
