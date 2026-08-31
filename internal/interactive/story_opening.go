package interactive

import (
	"fmt"
	"strings"
)

// StoryOpeningInstruction derives the first-turn model instruction entirely
// from durable story metadata. It is host-owned, English-only, and is committed
// as model-only input so it never appears as a player-authored action.
func StoryOpeningInstruction(meta StoryMeta) (string, error) {
	meta = normalizeStoryMeta(meta)
	opening := meta.Opening
	var source string
	switch opening.Mode {
	case StoryOpeningModeAI:
		source = "Create the opening from the story premise."
	case StoryOpeningModePreset:
		if opening.PresetText == "" {
			return "", fmt.Errorf("preset opening text is required")
		}
		source = "Preserve the core scene and hook from this story-owned preset snapshot:\n" + opening.PresetText
	case StoryOpeningModeCustom:
		if opening.CustomText == "" {
			return "", fmt.Errorf("custom opening text is required")
		}
		source = "Treat this user-authored opening as authoritative and continue from it:\n" + opening.CustomText
	default:
		return "", fmt.Errorf("unsupported story opening mode: %q", opening.Mode)
	}
	premise := strings.TrimSpace(meta.Origin)
	if premise == "" {
		premise = "No additional premise was provided."
	}
	return strings.Join([]string{
		"[Source: story opening configuration; purpose: generate the first playable turn]",
		"Story title: " + strings.TrimSpace(meta.Title),
		"Story premise: " + premise,
		source,
		"Generate the first playable interactive-story scene. Establish the immediate situation, a meaningful objective or pressure, and actionable space for the protagonist. Do not explain rules or provide an outline. Output narrative only and stop at a meaningful decision point.",
	}, "\n\n"), nil
}
