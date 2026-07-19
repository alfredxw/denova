package app

import (
	"testing"

	"denova/internal/interactive"
)

func TestCompleteGameAndSkillContextLimitsAreAtLeast128KB(t *testing.T) {
	const minimumBytes = 128 * 1024
	limits := map[string]int{
		"game runtime context fragment":       interactiveStoryRuntimeContextBytes,
		"game resolved lore context":          interactiveResolvedLoreContextMaxBytes,
		"director context fragment":           interactive.DirectorContextMaxBytes,
		"director active lore":                interactive.DirectorLoreActiveContextMaxBytes,
		"config manager resource skill":       configManagerResourceSkillMaxBytes,
		"config manager resource skill total": configManagerResourceSkillMaxTotalBytes,
	}
	for name, limit := range limits {
		if limit < minimumBytes {
			t.Errorf("%s limit = %d bytes, want at least %d", name, limit, minimumBytes)
		}
	}
}
