// Package presetlayout defines the durable on-disk layout for Denova's
// user-wide preset catalogs. Resource IDs remain the persistent identity;
// these paths only locate each typed catalog inside the configured data root.
package presetlayout

import "path/filepath"

const DirectoryName = "presets"

func Root(dataRoot string) string {
	return filepath.Join(dataRoot, DirectoryName)
}

func NarrativeStyles(dataRoot string) string {
	return filepath.Join(Root(dataRoot), "narrative-styles")
}

func Image(dataRoot string) string {
	return filepath.Join(Root(dataRoot), "image")
}

func GamePlanning(dataRoot string) string {
	return filepath.Join(Root(dataRoot), "game-planning")
}

func EventPackages(dataRoot string) string {
	return filepath.Join(Root(dataRoot), "event-packages")
}

func RuleSystems(dataRoot string) string {
	return filepath.Join(Root(dataRoot), "rule-systems")
}

func ActorStates(dataRoot string) string {
	return filepath.Join(Root(dataRoot), "actor-states")
}

// LegacyGamePresets retains the v0.3.3 composition presets while stories lazily
// migrate to independent planning, narrative, event, rule, state, and image IDs.
func LegacyGamePresets(dataRoot string) string {
	return filepath.Join(Root(dataRoot), "legacy", "v0.3.3-game-presets")
}
