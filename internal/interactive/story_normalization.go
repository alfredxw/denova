package interactive

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	imagepreset "denova/internal/image/preset"
)

func defaultStoryTitle(stories []StorySummary) string {
	if len(stories) == 0 {
		return defaultFirstStoryTitle
	}
	next := len(stories) + 1
	for _, story := range stories {
		title := strings.TrimSpace(story.Title)
		if !strings.HasPrefix(title, "故事线") {
			continue
		}
		rawNumber := strings.TrimSpace(strings.TrimPrefix(title, "故事线"))
		if rawNumber == "" {
			continue
		}
		number, err := strconv.Atoi(rawNumber)
		if err == nil && number >= next {
			next = number + 1
		}
	}
	if next < 2 {
		next = 2
	}
	return fmt.Sprintf("故事线 %d", next)
}

func normalizeStoryReplyTargetChars(value int) int {
	if value <= 0 {
		return DefaultStoryReplyTargetChars
	}
	return value
}

func normalizeStoryChoiceCount(value int) int {
	if value == 0 {
		return DefaultStoryChoiceCount
	}
	return value
}

func validateStoryChoiceCount(value int) error {
	if value < MinStoryChoiceCount || value > MaxStoryChoiceCount {
		return fmt.Errorf("互动故事行动建议数量必须在 %d 到 %d 之间", MinStoryChoiceCount, MaxStoryChoiceCount)
	}
	return nil
}

func normalizeStorySummary(story StorySummary) StorySummary {
	story.StoryDirectorID = normalizedStoryDirectorID(story.StoryDirectorID)
	story.PlanningMode = normalizeStoryPlanningMode(story.PlanningMode)
	story.ReplyTargetChars = normalizeStoryReplyTargetChars(story.ReplyTargetChars)
	story.ChoiceCount = normalizeStoryChoiceCount(story.ChoiceCount)
	story.Protagonist = normalizeStoryProtagonist(story.Protagonist)
	story.Opening = normalizeStoryOpeningConfig(story.Opening)
	story.ImageSettings = normalizeStoryImageSettings(story.ImageSettings)
	story.CheckSettings = normalizeStoryCheckSettings(story.CheckSettings)
	story.ModuleRefs = cloneStoryDirectorModuleRefs(story.ModuleRefs)
	if story.StateSchemaPolicy == nil {
		story.StateSchemaPolicy = fixedStoryStateSchemaPolicy()
	} else {
		story.StateSchemaPolicy = cloneStoryStateSchemaPolicy(story.StateSchemaPolicy)
	}
	return story
}

func normalizeStoryMeta(meta StoryMeta) StoryMeta {
	legacyFixedSchema := meta.StateSchemaPolicy == nil
	meta.StoryDirectorID = normalizedStoryDirectorID(meta.StoryDirectorID)
	meta.PlanningMode = normalizeStoryPlanningMode(meta.PlanningMode)
	meta.ReplyTargetChars = normalizeStoryReplyTargetChars(meta.ReplyTargetChars)
	meta.ChoiceCount = normalizeStoryChoiceCount(meta.ChoiceCount)
	meta.Protagonist = normalizeStoryProtagonist(meta.Protagonist)
	meta.Opening = normalizeStoryOpeningConfig(meta.Opening)
	meta.ImageSettings = normalizeStoryImageSettings(meta.ImageSettings)
	meta.CheckSettings = normalizeStoryCheckSettings(meta.CheckSettings)
	meta.ActorStateSchema = normalizeActorStateSchemaSnapshot(meta.ActorStateSchema)
	meta.ModuleRefs = cloneStoryDirectorModuleRefs(meta.ModuleRefs)
	if legacyFixedSchema {
		meta.StateSchemaPolicy = fixedStoryStateSchemaPolicy()
	} else {
		meta.StateSchemaPolicy = cloneStoryStateSchemaPolicy(meta.StateSchemaPolicy)
	}
	normalizeFixedStoryStateSchemaInitialization(&meta)
	return meta
}

func storyContainsTurn(events []StoryEventRecord) bool {
	for _, event := range events {
		if event.Envelope.Type == StoryEventTypeTurn {
			return true
		}
	}
	return false
}

func cloneStoryDirectorModuleRefs(refs *StoryDirectorModuleRefs) *StoryDirectorModuleRefs {
	if refs == nil {
		return nil
	}
	cloned := NormalizeStoryDirectorModuleRefs(*refs)
	cloned.EventPackageIDs = append([]string(nil), cloned.EventPackageIDs...)
	return &cloned
}

func normalizedStoryDirectorID(id string) string {
	if id = NormalizeStoryDirectorID(id); id != "" {
		return id
	}
	return DefaultStoryDirectorID
}

func normalizeStoryOpeningConfig(config StoryOpeningConfig) StoryOpeningConfig {
	mode := strings.TrimSpace(config.Mode)
	switch mode {
	case StoryOpeningModePreset, StoryOpeningModeCustom:
	default:
		mode = StoryOpeningModeAI
	}
	normalized := StoryOpeningConfig{
		Mode:       mode,
		PresetID:   strings.TrimSpace(config.PresetID),
		PresetText: truncateStoryOpeningText(config.PresetText),
		CustomText: truncateStoryOpeningText(config.CustomText),
	}
	if mode != StoryOpeningModePreset {
		normalized.PresetID = ""
		normalized.PresetText = ""
	}
	if mode != StoryOpeningModeCustom {
		normalized.CustomText = ""
	}
	return normalized
}

func truncateStoryOpeningText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxStoryOpeningTextRunes {
		return text
	}
	return string(runes[:maxStoryOpeningTextRunes])
}

func normalizeStoryImageSettings(settings StoryImageSettings) StoryImageSettings {
	rawMode := strings.TrimSpace(settings.Mode)
	mode := StoryImageModeManual
	interval := settings.IntervalTurns
	switch rawMode {
	case "every_turn":
		mode = StoryImageModeInterval
		interval = 1
	case StoryImageModeInterval:
		mode = StoryImageModeInterval
	default:
		mode = StoryImageModeManual
	}
	if interval <= 0 {
		interval = 3
	}
	if interval > 50 {
		interval = 50
	}
	return StoryImageSettings{
		Mode:          mode,
		IntervalTurns: interval,
		PresetID:      normalizeStoryImagePresetID(settings.PresetID),
	}
}

func normalizeStoryImagePresetID(id string) string {
	id = imagepreset.NormalizeID(id)
	if id == "" {
		return imagepreset.DefaultID
	}
	return id
}

func newID(prefix string) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + strconv.FormatInt(time.Now().UnixNano(), 36) + hex.EncodeToString(b[:])
}
