package interactive

import (
	"fmt"
	"strings"
)

const (
	StoryProtagonistModeDefault = "default"
	StoryProtagonistModeCustom  = "custom"
	StoryProtagonistModeLore    = "lore"
)

func normalizeStoryProtagonist(protagonist StoryProtagonist) StoryProtagonist {
	mode := strings.ToLower(strings.TrimSpace(protagonist.Mode))
	switch mode {
	case "":
		mode = StoryProtagonistModeDefault
	case StoryProtagonistModeDefault, StoryProtagonistModeCustom, StoryProtagonistModeLore:
	}
	result := StoryProtagonist{
		Mode:                mode,
		Name:                strings.TrimSpace(protagonist.Name),
		Profile:             strings.TrimSpace(protagonist.Profile),
		SourceLoreItemID:    strings.TrimSpace(protagonist.SourceLoreItemID),
		SourceLoreUpdatedAt: strings.TrimSpace(protagonist.SourceLoreUpdatedAt),
	}
	if mode == StoryProtagonistModeDefault {
		return StoryProtagonist{Mode: StoryProtagonistModeDefault}
	}
	if mode == StoryProtagonistModeCustom {
		result.SourceLoreItemID = ""
		result.SourceLoreUpdatedAt = ""
	}
	return result
}

func validateStoryProtagonist(protagonist StoryProtagonist) error {
	switch protagonist.Mode {
	case StoryProtagonistModeDefault:
		return nil
	case StoryProtagonistModeCustom:
		if protagonist.Name == "" {
			return fmt.Errorf("自定义主角名称不能为空 / Custom protagonist name is required")
		}
	case StoryProtagonistModeLore:
		if protagonist.SourceLoreItemID == "" || protagonist.Name == "" {
			return fmt.Errorf("资料库主角缺少来源或名称 / Lore protagonist source and name are required")
		}
	default:
		return fmt.Errorf("主角模式无效 / Invalid protagonist mode: %q", protagonist.Mode)
	}
	return nil
}

func applyStoryProtagonistToActorState(system StoryDirectorActorStateSystem, protagonist StoryProtagonist) StoryDirectorActorStateSystem {
	system = normalizeActorStateSystem(system)
	protagonist = normalizeStoryProtagonist(protagonist)
	if protagonist.Name == "" {
		return system
	}
	for index := range system.InitialActors {
		if system.InitialActors[index].ID == DefaultActorID {
			system.InitialActors[index].Name = protagonist.Name
			break
		}
	}
	return system
}

// StoryProtagonistContext returns the bounded, model-visible story foundation
// for a selected protagonist. The wrapper is English-only by product contract;
// user-authored names and profile content remain verbatim.
func StoryProtagonistContext(protagonist StoryProtagonist) string {
	protagonist = normalizeStoryProtagonist(protagonist)
	if protagonist.Mode == StoryProtagonistModeDefault {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("The player controls this story-owned protagonist. Treat this snapshot as canonical for identity and backstory.\n")
	builder.WriteString("Runtime Actor ID: `protagonist`\n")
	builder.WriteString("Name: ")
	builder.WriteString(protagonist.Name)
	if protagonist.Profile != "" {
		builder.WriteString("\nProfile:\n")
		builder.WriteString(protagonist.Profile)
	}
	return builder.String()
}
