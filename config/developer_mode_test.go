package config

import "testing"

func TestSettingsFromConfigPreservesDeveloperMode(t *testing.T) {
	settings := settingsFromConfig(&Config{Labs: ResolvedLabs{DeveloperMode: true}})
	if settings.Labs.DeveloperMode == nil || !*settings.Labs.DeveloperMode {
		t.Fatalf("developer mode was lost while converting config to settings: %#v", settings.Labs)
	}
}
