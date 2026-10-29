package interactive

import (
	"encoding/json"
	"errors"
	"fmt"
)

var ErrNarrativeStyleOverrides = errors.New("invalid story narrative style overrides")

// StoryNarrativeStyleOverrides belongs to one story and one selected style.
// An empty Slots map resets the story to the shared style. It never edits the library.
type StoryNarrativeStyleOverrides struct {
	StyleID string                           `json:"style_id"`
	Slots   map[string]NarrativeSlotOverride `json:"slots"`
}

type NarrativeSlotOverride struct {
	Enabled *bool   `json:"enabled,omitempty"`
	Content *string `json:"content,omitempty"`
}

// Bound additional story context without truncating user-authored rules.
const storyNarrativeOverridesMaxBytes = 512 << 10

func cloneNarrativeStyleOverrides(value *StoryNarrativeStyleOverrides) *StoryNarrativeStyleOverrides {
	if value == nil {
		return nil
	}
	result := &StoryNarrativeStyleOverrides{StyleID: value.StyleID, Slots: make(map[string]NarrativeSlotOverride, len(value.Slots))}
	for id, slot := range value.Slots {
		copy := NarrativeSlotOverride{}
		if slot.Enabled != nil {
			enabled := *slot.Enabled
			copy.Enabled = &enabled
		}
		if slot.Content != nil {
			content := *slot.Content
			copy.Content = &content
		}
		result.Slots[id] = copy
	}
	return result
}

func validateNarrativeStyleOverrides(value *StoryNarrativeStyleOverrides, styleID string) error {
	if value.StyleID != styleID {
		return fmt.Errorf("%w: selected style changed", ErrNarrativeStyleOverrides)
	}
	data, err := json.Marshal(value)
	if err != nil || len(data) > storyNarrativeOverridesMaxBytes {
		return fmt.Errorf("%w: rules exceed 512 KiB", ErrNarrativeStyleOverrides)
	}
	return nil
}
