package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReadSettingsFileMigratesLegacyImageSettingsAndCreatesBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	legacy := `image_api_key = "top-level-key"
image_api_base_url = "https://api.openai.com/v1"
image_api_model = "gpt-image-1"
default_image_api_profile_id = "portrait"

[[image_api_profiles]]
id = "portrait"
name = "Portrait"
provider = "openai"
openai_api_key = "profile-key"
openai_base_url = "https://images.example.test/v1"
openai_model = "portrait-v1"
default_size = "1024x1536"
`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	settings, err := ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if settings.DefaultImageAPIProfileID != "portrait" || len(settings.ImageAPIProfiles) != 2 {
		t.Fatalf("migrated image settings = %#v", settings)
	}
	profiles := imageProfilesByID(settings.ImageAPIProfiles)
	endpoints := imageEndpointsByID(settings.ImageAPIEndpoints)
	portrait := profiles["portrait"]
	portraitEndpoint := endpoints[portrait.EndpointID]
	if portraitEndpoint.Provider != ImageProviderOpenAI || portraitEndpoint.Protocol != ImageProtocolOpenAI ||
		portraitEndpoint.APIKey != "profile-key" || portraitEndpoint.BaseURL != "https://images.example.test/v1" || portrait.Model != "portrait-v1" {
		t.Fatalf("migrated legacy profile = %#v", portrait)
	}
	defaultProfile := profiles[DefaultImageAPIProfileID]
	defaultEndpoint := endpoints[defaultProfile.EndpointID]
	if defaultEndpoint.APIKey != "top-level-key" || defaultProfile.Model != legacyDefaultImageAPIModel {
		t.Fatalf("migrated top-level profile = %#v", defaultProfile)
	}
	if settings.LegacyImageAPIKey != nil || portrait.LegacyOpenAIAPIKey != nil {
		t.Fatalf("legacy aliases remained after migration: %#v %#v", settings, portrait)
	}

	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, legacyField := range []string{"image_api_key", "image_api_base_url", "image_api_model", "openai_api_key", "openai_base_url", "openai_model"} {
		if strings.Contains(string(persisted), legacyField) {
			t.Fatalf("legacy field %q remained in migrated config:\n%s", legacyField, persisted)
		}
	}
	backups, err := filepath.Glob(path + ".pre-model-endpoint-migration-*.bak")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("migration backups = %v", backups)
	}
	backup, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != legacy {
		t.Fatalf("migration backup changed original bytes:\n%s", backup)
	}
	info, err := os.Stat(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("migration backup mode = %o", info.Mode().Perm())
	}

	before := string(persisted)
	if _, err := ReadSettingsFile(path); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != before {
		t.Fatal("canonical image settings were rewritten on the second read")
	}
}

func TestLegacyImageMigrationPrefersCanonicalFields(t *testing.T) {
	legacyKey, legacyBaseURL, legacyModel := "legacy-key", "https://legacy.example/v1", "legacy-model"
	settings, migrated := migrateLegacyImageSettings(Settings{ImageAPIProfiles: []ImageAPIProfileSettings{{
		ID: "default", Provider: ImageProviderOpenAI, Protocol: ImageProtocolOpenAI,
		APIKey: "canonical-key", BaseURL: "https://canonical.example/v1", Model: "canonical-model",
		LegacyOpenAIAPIKey: &legacyKey, LegacyOpenAIBaseURL: &legacyBaseURL, LegacyOpenAIModel: &legacyModel,
	}}})
	if !migrated {
		t.Fatal("legacy aliases were not detected")
	}
	profile := settings.ImageAPIProfiles[0]
	if profile.APIKey != "canonical-key" || profile.BaseURL != "https://canonical.example/v1" || profile.Model != "canonical-model" {
		t.Fatalf("canonical fields did not win: %#v", profile)
	}
}

func TestLegacyTopLevelModelFeedsLegacyDefaultProfile(t *testing.T) {
	legacyKey, legacyModel := "profile-key", "top-level-model"
	settings, migrated := migrateLegacyImageSettings(Settings{
		LegacyImageAPIModel: &legacyModel,
		ImageAPIProfiles: []ImageAPIProfileSettings{{
			ID: DefaultImageAPIProfileID, LegacyOpenAIAPIKey: &legacyKey,
		}},
	})
	if !migrated {
		t.Fatal("legacy image settings were not detected")
	}
	profile := imageProfilesByID(settings.ImageAPIProfiles)[DefaultImageAPIProfileID]
	if profile.APIKey != legacyKey || profile.Model != legacyModel {
		t.Fatalf("legacy top-level fallback was not preserved: %#v", profile)
	}
}

func TestReleasedImageProfilesPreserveInheritedAndExplicitCredentialsDuringEndpointMigration(t *testing.T) {
	defaultKey, defaultBaseURL := "default-key", "https://default.example/v1"
	sharedBaseURL, inheritedModel := "https://shared.example/v1", "inherited-image"
	explicitKey, explicitModel := "explicit-key", "explicit-image"
	settings, migrated := migrateImageAPIEndpointSettings(Settings{
		LegacyImageAPIKey:     &defaultKey,
		LegacyImageAPIBaseURL: &defaultBaseURL,
		ImageAPIProfiles: []ImageAPIProfileSettings{
			{ID: "inherited", LegacyOpenAIBaseURL: &sharedBaseURL, LegacyOpenAIModel: &inheritedModel},
			{ID: "explicit", LegacyOpenAIAPIKey: &explicitKey, LegacyOpenAIBaseURL: &sharedBaseURL, LegacyOpenAIModel: &explicitModel},
		},
	})
	if !migrated {
		t.Fatal("released image settings were not migrated")
	}
	profiles := imageProfilesByID(settings.ImageAPIProfiles)
	endpoints := imageEndpointsByID(settings.ImageAPIEndpoints)
	inheritedEndpoint := endpoints[profiles["inherited"].EndpointID]
	explicitEndpoint := endpoints[profiles["explicit"].EndpointID]
	if inheritedEndpoint.APIKey != defaultKey || explicitEndpoint.APIKey != explicitKey {
		t.Fatalf("migrated credentials inherited=%q explicit=%q", inheritedEndpoint.APIKey, explicitEndpoint.APIKey)
	}
	if profiles["inherited"].EndpointID == profiles["explicit"].EndpointID {
		t.Fatalf("profiles with different effective credentials shared endpoint %q", profiles["inherited"].EndpointID)
	}
}

func TestReleasedImageProfilesDoNotTreatMissingCredentialAsReusable(t *testing.T) {
	sharedBaseURL, firstModel := "https://shared.example/v1", "first-image"
	explicitKey, secondModel := "explicit-key", "second-image"
	settings, migrated := migrateImageAPIEndpointSettings(Settings{ImageAPIProfiles: []ImageAPIProfileSettings{
		{ID: "first", LegacyOpenAIBaseURL: &sharedBaseURL, LegacyOpenAIModel: &firstModel},
		{ID: "second", LegacyOpenAIAPIKey: &explicitKey, LegacyOpenAIBaseURL: &sharedBaseURL, LegacyOpenAIModel: &secondModel},
	}})
	if !migrated {
		t.Fatal("released image settings were not migrated")
	}
	profiles := imageProfilesByID(settings.ImageAPIProfiles)
	endpoints := imageEndpointsByID(settings.ImageAPIEndpoints)
	if endpoints[profiles["first"].EndpointID].APIKey != "" || endpoints[profiles["second"].EndpointID].APIKey != explicitKey {
		t.Fatalf("migrated endpoints = %#v", endpoints)
	}
	if profiles["first"].EndpointID == profiles["second"].EndpointID {
		t.Fatalf("missing and explicit credentials shared endpoint %q", profiles["first"].EndpointID)
	}
}

func TestReadSettingsFilePreservesLegacyImplicitImageModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`image_api_key = "legacy-key"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	settings, err := ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	profile := imageProfilesByID(settings.ImageAPIProfiles)[DefaultImageAPIProfileID]
	if profile.Model != legacyDefaultImageAPIModel {
		t.Fatalf("implicit legacy image model = %q, want %q", profile.Model, legacyDefaultImageAPIModel)
	}
}

func TestLegacyTopLevelImageSettingsDoNotLeakIntoNewProvider(t *testing.T) {
	legacyKey := "legacy-openai-key"
	settings, migrated := migrateLegacyImageSettings(Settings{
		LegacyImageAPIKey: &legacyKey,
		ImageAPIProfiles: []ImageAPIProfileSettings{{
			ID: "default", Provider: ImageProviderXAI, Protocol: ImageProtocolXAI,
			APIKey: "xai-key", Model: "grok-imagine-image-2.0",
		}},
	})
	if !migrated {
		t.Fatal("legacy top-level settings were not detected")
	}
	profiles := imageProfilesByID(settings.ImageAPIProfiles)
	if profiles["default"].APIKey != "xai-key" {
		t.Fatalf("legacy OpenAI key changed the xAI profile: %#v", profiles["default"])
	}
	legacy := profiles["legacy-openai-image"]
	if legacy.Provider != ImageProviderOpenAI || legacy.APIKey != legacyKey || legacy.Model != legacyDefaultImageAPIModel {
		t.Fatalf("legacy OpenAI profile was not preserved separately: %#v", legacy)
	}
}

func TestApplyImageAPIEnvironmentAcceptsLegacyOpenAIAliases(t *testing.T) {
	unsetEnvironmentForTest(t, "DENOVA_IMAGE_PROVIDER", "DENOVA_IMAGE_PROTOCOL", "DENOVA_IMAGE_API_KEY", "DENOVA_IMAGE_BASE_URL", "DENOVA_IMAGE_MODEL")
	t.Setenv("OPENAI_IMAGE_API_KEY", "legacy-key")
	t.Setenv("OPENAI_IMAGE_BASE_URL", "https://legacy-images.example/v1")
	t.Setenv("OPENAI_IMAGE_MODEL", "legacy-image-model")

	cfg := &Config{ImageAPIProfiles: []ImageAPIProfileSettings{DefaultImageAPIProfile()}}
	ApplyImageAPIEnvironment(cfg)
	resolved, err := ResolveImageAPIProfile(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Provider != ImageProviderOpenAI || resolved.Protocol != ImageProtocolOpenAI ||
		resolved.APIKey != "legacy-key" || resolved.BaseURL != "https://legacy-images.example/v1" || resolved.Model != "legacy-image-model" {
		t.Fatalf("legacy image environment aliases = %#v", resolved)
	}
}

func TestCanonicalImageEnvironmentOverridesLegacyAliases(t *testing.T) {
	t.Setenv("DENOVA_IMAGE_PROVIDER", ImageProviderOpenAI)
	t.Setenv("DENOVA_IMAGE_PROTOCOL", ImageProtocolOpenAI)
	t.Setenv("DENOVA_IMAGE_API_KEY", "canonical-key")
	t.Setenv("DENOVA_IMAGE_BASE_URL", "https://canonical-images.example/v1")
	t.Setenv("DENOVA_IMAGE_MODEL", "canonical-image-model")
	t.Setenv("OPENAI_IMAGE_API_KEY", "legacy-key")
	t.Setenv("OPENAI_IMAGE_BASE_URL", "https://legacy-images.example/v1")
	t.Setenv("OPENAI_IMAGE_MODEL", "legacy-image-model")

	cfg := &Config{ImageAPIProfiles: []ImageAPIProfileSettings{DefaultImageAPIProfile()}}
	ApplyImageAPIEnvironment(cfg)
	resolved, err := ResolveImageAPIProfile(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.APIKey != "canonical-key" || resolved.BaseURL != "https://canonical-images.example/v1" || resolved.Model != "canonical-image-model" {
		t.Fatalf("canonical image environment did not win: %#v", resolved)
	}
}

func TestMutateSettingsFileBacksUpLegacyImageSettingsBeforePatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	legacy := []byte("image_api_key = \"legacy-key\"\ntheme = \"dark\"\n")
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := MutateSettingsFile(path, "", func(settings Settings) (Settings, error) {
		settings.Theme = "light"
		return settings, nil
	}); err != nil {
		t.Fatal(err)
	}
	settings, err := ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defaultProfile := imageProfilesByID(settings.ImageAPIProfiles)[DefaultImageAPIProfileID]
	if settings.Theme != "light" || imageEndpointsByID(settings.ImageAPIEndpoints)[defaultProfile.EndpointID].APIKey != "legacy-key" {
		t.Fatalf("mutated migrated settings = %#v", settings)
	}
	backups, err := filepath.Glob(path + ".pre-model-endpoint-migration-*.bak")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("migration backups = %v", backups)
	}
}

func TestSettingsFromConfigMigratesLegacyStartupImageFields(t *testing.T) {
	key, model := "startup-key", "startup-image"
	settings := settingsFromConfig(&Config{LegacyImageAPIKey: &key, LegacyImageAPIModel: &model})
	profiles := imageProfilesByID(settings.ImageAPIProfiles)
	profile := profiles[DefaultImageAPIProfileID]
	if imageEndpointsByID(settings.ImageAPIEndpoints)[profile.EndpointID].APIKey != key || profile.Model != model {
		t.Fatalf("legacy startup image settings = %#v", settings.ImageAPIProfiles)
	}
}

func imageProfilesByID(profiles []ImageAPIProfileSettings) map[string]ImageAPIProfileSettings {
	out := make(map[string]ImageAPIProfileSettings, len(profiles))
	for _, profile := range profiles {
		out[imageAPIProfileID(profile)] = profile
	}
	return out
}

func imageEndpointsByID(endpoints []ImageAPIEndpointSettings) map[string]ImageAPIEndpointSettings {
	out := make(map[string]ImageAPIEndpointSettings, len(endpoints))
	for _, endpoint := range endpoints {
		out[imageAPIEndpointID(endpoint)] = endpoint
	}
	return out
}

func unsetEnvironmentForTest(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		value, existed := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(name, value)
			} else {
				_ = os.Unsetenv(name)
			}
		})
	}
}
