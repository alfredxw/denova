package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

	imagepreset "denova/internal/image/preset"
	"denova/internal/interactive"
	"denova/internal/interactive/teller"
	"denova/internal/platform"
	"denova/internal/style"
)

type portableNarrativeContent struct {
	StyleRefs     []string             `json:"styleRefs"`
	StyleRules    []teller.StyleRule   `json:"styleRules"`
	ContextPolicy teller.ContextPolicy `json:"contextPolicy"`
	Slots         []teller.PromptSlot  `json:"slots"`
}

type portableImageContent struct {
	Prompt string             `json:"prompt"`
	Slots  []imagepreset.Slot `json:"slots"`
}

// Portable content carries no author-local style references. Native inline
// style rules preserve the same global and scene-specific selection semantics.
func portableStoryNarrative(dataDir string, item teller.Definition) (portableNarrativeContent, error) {
	rules := make([]teller.StyleRule, 0, len(item.StyleRules)+1)
	if len(item.StyleRefs) > 0 {
		rules = append(rules, teller.StyleRule{Scene: "global", StyleRefs: item.StyleRefs})
	}
	rules = append(rules, item.StyleRules...)
	for index := range rules {
		rules[index].StyleContents = append([]string(nil), rules[index].StyleContents...)
		for _, ref := range rules[index].StyleRefs {
			file, err := style.NewLibrary(dataDir).Read(ref)
			if err != nil {
				return portableNarrativeContent{}, platformStoryError("INVALID_ARGUMENT", "A narrative style reference is unavailable")
			}
			rules[index].StyleContents = append(rules[index].StyleContents, file.Content)
		}
		rules[index].StyleRefs = nil
	}
	return portableNarrativeContent{StyleRules: teller.NormalizeStyleRules(rules), ContextPolicy: item.ContextPolicy, Slots: item.Slots}, nil
}

func decodePortableStoryContent(raw json.RawMessage, target any) error {
	if len(raw) == 0 || len(raw) > platform.MaxDefinitionBytes || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return platformStoryError("INVALID_ARGUMENT", "Portable preset content requires non-null JSON; the complete request must fit within 1 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return platformStoryError("INVALID_ARGUMENT", "Invalid portable preset content: "+err.Error())
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return platformStoryError("INVALID_ARGUMENT", "Portable preset content must contain one JSON value")
	}
	return nil
}

func portableStoryPresetDigest(value platform.StoryPreset) (string, error) {
	// Metadata is portable author content too. Runtime revisions, IDs and paths
	// are intentionally absent from the digest and cannot overwrite local IDs.
	data, err := json.Marshal(struct {
		Kind, Name, Description string
		Content                 any
	}{value.Kind, value.Name, value.Description, value.Content})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "imported-" + hex.EncodeToString(digest[:]), nil
}

// ImportPreset uses native create-only library writes. A returned ID is bound
// through ordinary moduleRefs; existing native loader semantics are unchanged.
func (h Stories) ImportPreset(ctx context.Context, scope platform.Scope, input platform.StoryPresetImport) (platform.StoryPreset, error) {
	operation, err := h.host.AcquireStory(ctx, scope.ProjectID)
	if err != nil {
		return platform.StoryPreset{}, err
	}
	defer operation.Release()
	if _, err := h.host.InteractiveSnapshot(scope.StoryID, ""); err != nil {
		return platform.StoryPreset{}, err
	}
	if strings.TrimSpace(input.Name) == "" || len(input.Name) > 256 || len(input.Description) > 1024 {
		return platform.StoryPreset{}, platformStoryError("INVALID_ARGUMENT", "Preset name requires 1..256 bytes and description allows 1024 bytes")
	}
	dataDir := h.host.DataDir()
	value := platform.StoryPreset{Kind: input.Kind, Name: strings.TrimSpace(input.Name), Description: strings.TrimSpace(input.Description)}
	var get func(string) (platform.StoryPreset, error)
	var create func(string) (platform.StoryPreset, error)
	switch input.Kind {
	case "events":
		var content []interactive.EventCard
		if err := decodePortableStoryContent(input.Content, &content); err != nil {
			return value, err
		}
		item, err := interactive.PreparePortableEventPackage(interactive.EventPackageModule{ID: "portable-import", Name: value.Name, Description: value.Description, Events: content})
		if err != nil {
			return value, platformStoryError("INVALID_ARGUMENT", err.Error())
		}
		project := func(v interactive.EventPackageModule) platform.StoryPreset {
			return platform.StoryPreset{Kind: input.Kind, ID: v.ID, Name: v.Name, Description: v.Description, Revision: v.Revision, Content: v.Events}
		}
		value = project(item)
		library := interactive.NewEventPackageLibrary(dataDir)
		get = func(id string) (platform.StoryPreset, error) { v, e := library.Get(id); return project(v), e }
		create = func(id string) (platform.StoryPreset, error) {
			item.ID = id
			v, e := library.Create(item)
			return project(v), e
		}
	case "planning":
		var content []interactive.GamePlanningSection
		if err := decodePortableStoryContent(input.Content, &content); err != nil {
			return value, err
		}
		item, err := interactive.PreparePortablePlanningTemplate(interactive.GamePlanningTemplate{ID: "portable-import", Name: value.Name, Description: value.Description, Sections: content})
		if err != nil {
			return value, platformStoryError("INVALID_ARGUMENT", err.Error())
		}
		project := func(v interactive.GamePlanningTemplate) platform.StoryPreset {
			return platform.StoryPreset{Kind: input.Kind, ID: v.ID, Name: v.Name, Description: v.Description, Revision: v.Revision, Content: v.Sections}
		}
		value = project(item)
		library := interactive.NewGamePlanningTemplateLibrary(dataDir)
		get = func(id string) (platform.StoryPreset, error) { v, e := library.Get(id); return project(v), e }
		create = func(id string) (platform.StoryPreset, error) {
			item.ID = id
			v, e := library.Create(item)
			return project(v), e
		}
	case "narrative":
		var content portableNarrativeContent
		if err := decodePortableStoryContent(input.Content, &content); err != nil {
			return value, err
		}
		if len(content.StyleRefs) > 0 {
			return value, platformStoryError("INVALID_ARGUMENT", "Portable narrative styles must embed references as style_contents")
		}
		for _, rule := range content.StyleRules {
			if len(rule.StyleRefs) > 0 {
				return value, platformStoryError("INVALID_ARGUMENT", "Portable narrative styles must embed references as style_contents")
			}
		}
		item, err := teller.PreparePortable(teller.Definition{ID: "portable-import", Name: value.Name, Description: value.Description, StyleRules: content.StyleRules, ContextPolicy: content.ContextPolicy, Slots: content.Slots})
		if err != nil {
			return value, platformStoryError("INVALID_ARGUMENT", err.Error())
		}
		project := func(v teller.Definition) platform.StoryPreset {
			return platform.StoryPreset{Kind: input.Kind, ID: v.ID, Name: v.Name, Description: v.Description, Revision: v.Revision, Content: portableNarrativeContent{StyleRefs: v.StyleRefs, StyleRules: v.StyleRules, ContextPolicy: v.ContextPolicy, Slots: v.Slots}}
		}
		value = project(item)
		library := teller.NewLibrary(dataDir)
		get = func(id string) (platform.StoryPreset, error) { v, e := library.Get(id); return project(v), e }
		create = func(id string) (platform.StoryPreset, error) {
			item.ID = id
			v, e := library.Create(item)
			return project(v), e
		}
	case "image":
		var content portableImageContent
		if err := decodePortableStoryContent(input.Content, &content); err != nil {
			return value, err
		}
		item, err := imagepreset.PreparePortable(imagepreset.Preset{ID: "portable-import", Name: value.Name, Description: value.Description, Prompt: content.Prompt, Slots: content.Slots})
		if err != nil {
			return value, platformStoryError("INVALID_ARGUMENT", err.Error())
		}
		project := func(v imagepreset.Preset) platform.StoryPreset {
			return platform.StoryPreset{Kind: input.Kind, ID: v.ID, Name: v.Name, Description: v.Description, Revision: v.Revision, Content: portableImageContent{Prompt: v.Prompt, Slots: v.Slots}}
		}
		value = project(item)
		library := imagepreset.NewLibrary(dataDir)
		get = func(id string) (platform.StoryPreset, error) { v, e := library.Get(id); return project(v), e }
		create = func(id string) (platform.StoryPreset, error) {
			item.ID = id
			v, e := library.Create(item)
			return project(v), e
		}
	default:
		return value, platformStoryError("INVALID_ARGUMENT", "Import supports events, planning, narrative and image; freeze state and rules through Story configuration")
	}
	id, err := portableStoryPresetDigest(value)
	if err != nil {
		return value, err
	}
	verify := func(existing platform.StoryPreset) (platform.StoryPreset, error) {
		digest, e := portableStoryPresetDigest(existing)
		if e != nil {
			return existing, e
		}
		if digest != id {
			return existing, platformStoryError("DOCUMENT_CONFLICT", "Content-addressed preset was changed locally; restore or remove the conflicting custom copy")
		}
		return existing, nil
	}
	if existing, err := get(id); err == nil {
		return verify(existing)
	}
	created, err := create(id)
	if err != nil {
		// Another importer may have won the create-only race. No update or
		// overwrite is permitted, including when an existing file is invalid.
		if existing, getErr := get(id); getErr == nil {
			return verify(existing)
		}
		slog.WarnContext(ctx, "platform_story_preset_import_failed", "kind", input.Kind, "preset", id, "error", err)
		return value, fmt.Errorf("Create portable Story preset: %w", err)
	}
	slog.InfoContext(ctx, "platform_story_preset_imported", "kind", input.Kind, "preset", id, "story", scope.StoryID)
	return verify(created)
}
