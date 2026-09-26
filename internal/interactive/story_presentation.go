package interactive

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// StoryPresentationSettings controls future selection and local visibility.
// Disabling a layer never erases a committed turn's presentation.
type StoryPresentationSettings struct {
	Background bool `json:"background"`
	Characters bool `json:"characters"`
}

func NormalizeStoryPresentationSettings(settings *StoryPresentationSettings) *StoryPresentationSettings {
	if settings == nil {
		return &StoryPresentationSettings{Background: true, Characters: true}
	}
	copy := *settings
	return &copy
}

// PresentationMaterial pins a Lore association to its concrete project-relative
// file. History must not resolve the item's current cover or current materials.
type PresentationMaterial struct {
	ItemID  string `json:"item_id"`
	AssetID string `json:"asset_id"`
	Path    string `json:"path"`
	Name    string `json:"name"`
}

// TurnPresentation is a full snapshot, committed atomically with the turn.
type TurnPresentation struct {
	Background *PresentationMaterial  `json:"background,omitempty"`
	Characters []PresentationMaterial `json:"characters,omitempty"`
}

func (p *TurnPresentation) Clone() *TurnPresentation {
	if p == nil {
		return nil
	}
	copy := *p
	if p.Background != nil {
		background := *p.Background
		copy.Background = &background
	}
	copy.Characters = append([]PresentationMaterial(nil), p.Characters...)
	return &copy
}

// PresentationPatchSchema documents the model-facing patch. Decoding uses raw
// JSON to distinguish omission, explicit null, and malformed individual slots.
type PresentationPatchSchema struct {
	Background *PresentationReferenceSchema  `json:"background,omitempty" jsonschema_description:"Replace the background with an enabled Lore image reference. null clears it; omission preserves it."`
	Characters []PresentationReferenceSchema `json:"characters,omitempty" jsonschema_description:"Patch characters by Lore item_id. Omitted characters and an empty array preserve the cast. Existing characters keep their position; new ones append. At most 16 characters can be on stage."`
}

type PresentationReferenceSchema struct {
	ItemID  string  `json:"item_id" jsonschema_description:"Exact enabled Lore item ID, not an Actor ID."`
	AssetID *string `json:"asset_id" jsonschema_description:"Exact associated image asset ID. For a character only, explicit null removes that character. Missing or invalid IDs preserve the previous slot."`
}

type PresentationReceipt struct {
	Applied int      `json:"applied"`
	Ignored int      `json:"ignored"`
	Reasons []string `json:"reasons,omitempty"`
}

// ApplyPresentationPatch accepts independent visual changes without affecting
// turn readiness. resolve must validate enabled Lore ownership and a readable
// image file; it must never accept an arbitrary model-provided path.
func ApplyPresentationPatch(base *TurnPresentation, raw json.RawMessage, settings *StoryPresentationSettings, resolve func(string, string) (PresentationMaterial, error)) (*TurnPresentation, *PresentationReceipt) {
	result := base.Clone()
	if result == nil {
		result = &TurnPresentation{}
	}
	if len(raw) == 0 {
		return result, nil
	}
	receipt := &PresentationReceipt{}
	ignore := func(reason string) {
		receipt.Ignored++
		if len(receipt.Reasons) < 8 {
			receipt.Reasons = append(receipt.Reasons, reason)
		}
	}
	var patch map[string]json.RawMessage
	if err := json.Unmarshal(raw, &patch); err != nil || patch == nil {
		ignore("presentation must be an object; previous stage retained")
		return result, receipt
	}
	settings = NormalizeStoryPresentationSettings(settings)
	apply := func(raw json.RawMessage, index int) {
		background := index < 0
		label := "background"
		if !background {
			label = fmt.Sprintf("characters[%d]", index)
		}
		if (background && !settings.Background) || (!background && !settings.Characters) {
			ignore(label + ": layer disabled")
			return
		}
		if background && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			result.Background = nil
			receipt.Applied++
			return
		}
		var ref struct {
			ItemID  string          `json:"item_id"`
			AssetID json.RawMessage `json:"asset_id"`
		}
		if err := json.Unmarshal(raw, &ref); err != nil || ref.ItemID == "" || len(ref.AssetID) == 0 {
			ignore(label + ": item_id and asset_id are required")
			return
		}
		position := -1
		for i, character := range result.Characters {
			if character.ItemID == ref.ItemID {
				position = i
				break
			}
		}
		if !background && bytes.Equal(bytes.TrimSpace(ref.AssetID), []byte("null")) {
			if position >= 0 {
				result.Characters = append(result.Characters[:position], result.Characters[position+1:]...)
			}
			receipt.Applied++
			return
		}
		var assetID string
		if err := json.Unmarshal(ref.AssetID, &assetID); err != nil || assetID == "" {
			ignore(label + ": invalid asset_id")
			return
		}
		material, err := resolve(ref.ItemID, assetID)
		if err != nil {
			ignore(label + ": " + err.Error())
			return
		}
		if background {
			result.Background = &material
		} else if position >= 0 {
			result.Characters[position] = material
		} else if len(result.Characters) < 16 {
			result.Characters = append(result.Characters, material)
		} else {
			ignore(label + ": stage supports at most 16 characters")
			return
		}
		receipt.Applied++
	}
	if background, exists := patch["background"]; exists {
		apply(background, -1)
	}
	if rawCharacters, exists := patch["characters"]; exists {
		var characters []json.RawMessage
		if err := json.Unmarshal(rawCharacters, &characters); err != nil || bytes.Equal(bytes.TrimSpace(rawCharacters), []byte("null")) {
			ignore("characters must be an array; previous cast retained")
		} else {
			for i, character := range characters {
				apply(character, i)
			}
		}
	}
	return result, receipt
}
