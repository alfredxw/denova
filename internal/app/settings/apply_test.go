package settings

import (
	"testing"

	"denova/config"
)

func TestApplyLayeredRefreshesResolvedLabSettings(t *testing.T) {
	enabled, scheduled := true, true
	interval, cap := 48, 120
	cfg := config.Config{}
	ApplyLayered(&cfg, config.LayeredSettings{Effective: config.Settings{Labs: config.LabSettings{
		DeveloperMode:                  &enabled,
		ContinualLearningSchedule:      &scheduled,
		ContinualLearningIntervalHours: &interval,
		ContinualLearningTrajectoryCap: &cap,
	}}})

	if !cfg.Labs.DeveloperMode || !cfg.Labs.ContinualLearningSchedule ||
		cfg.Labs.ContinualLearningIntervalHours != interval || cfg.Labs.ContinualLearningTrajectoryCap != cap {
		t.Fatalf("runtime Labs were not refreshed: %#v", cfg.Labs)
	}
}

func TestApplyLayeredKeepsImageEnvironmentOverridesAfterEffectiveSettings(t *testing.T) {
	t.Setenv("DENOVA_IMAGE_PROVIDER", config.ImageProviderXAI)
	t.Setenv("DENOVA_IMAGE_PROTOCOL", "")
	t.Setenv("DENOVA_IMAGE_API_KEY", "environment-key")
	t.Setenv("DENOVA_IMAGE_BASE_URL", "")
	t.Setenv("DENOVA_IMAGE_MODEL", "grok-environment")

	effective := config.Settings{
		DefaultImageAPIProfileID: config.DefaultImageAPIProfileID,
		ImageAPIProfiles: []config.ImageAPIProfileSettings{{
			ID:       config.DefaultImageAPIProfileID,
			Provider: config.ImageProviderOpenAI,
			APIKey:   "persisted-key",
			Model:    "persisted-model",
		}},
	}
	cfg := config.Config{}
	ApplyLayered(&cfg, config.LayeredSettings{Effective: effective})

	resolved, err := config.ResolveImageAPIProfile(&cfg, "")
	if err != nil {
		t.Fatalf("ResolveImageAPIProfile() error = %v", err)
	}
	if resolved.Provider != config.ImageProviderXAI || resolved.Protocol != config.ImageProtocolXAI {
		t.Fatalf("resolved provider/protocol = %q/%q", resolved.Provider, resolved.Protocol)
	}
	if resolved.APIKey != "environment-key" || resolved.Model != "grok-environment" {
		t.Fatalf("resolved environment values = key %q model %q", resolved.APIKey, resolved.Model)
	}
	if effective.ImageAPIProfiles[0].Provider != config.ImageProviderOpenAI {
		t.Fatalf("ApplyLayered mutated effective settings: %#v", effective.ImageAPIProfiles[0])
	}
}
