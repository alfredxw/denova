// Package prompting projects image-model configuration into model-visible
// prompt-authoring guidance. The guide never participates in provider calls.
package prompting

import (
	"fmt"
	"strings"

	"denova/config"
)

// SelectedGuide returns a provenance-labeled instruction for the active image
// model. An invalid image profile is left for generation-time validation.
func SelectedGuide(cfg *config.Config) string {
	profile, err := config.ResolveImageAPIProfile(cfg, "")
	if err != nil || strings.TrimSpace(profile.PromptGuide) == "" {
		return ""
	}
	return fmt.Sprintf("## Selected Image Model Prompt Guide\n\nSource: image model profile %q.\nPurpose: author the final `prompt` in the syntax preferred by this model. Follow the guide when constructing `prompt`; do not copy this heading or explanation into `prompt`.\n\n%s", profile.ProfileID, profile.PromptGuide)
}

// ToolPromptContext returns the configured image-preset rules at the point
// where an Agent authors the final prompt.
func ToolPromptContext(cfg *config.Config) string {
	if cfg == nil || strings.TrimSpace(cfg.ImagePresetToolPrompt) == "" {
		return ""
	}
	return "## Active Image Preset Rules\n\nSource: selected image preset tool prompt.\nPurpose: constrain the final `prompt` for this image request. Apply these rules while authoring `prompt`; do not copy this heading or explanation into `prompt`.\n\n" + strings.TrimSpace(cfg.ImagePresetToolPrompt)
}

// Append joins optional model-visible prompt-authoring fragments without
// changing the final prompt passed to an image provider.
func Append(base string, fragments ...string) string {
	parts := []string{strings.TrimSpace(base)}
	for _, fragment := range fragments {
		if fragment = strings.TrimSpace(fragment); fragment != "" {
			parts = append(parts, fragment)
		}
	}
	return strings.Join(parts, "\n\n")
}
