package config

import (
	"strconv"
	"strings"
)

const legacyDefaultImageAPIModel = "gpt-image-1"

// migrateLegacyImageSettings converts both released image schemas into the
// canonical provider/protocol profile model. Canonical fields always win.
func migrateLegacyImageSettings(settings Settings) (Settings, bool) {
	hasLegacyTopLevel := hasLegacyTopLevelImageSettings(settings)
	migrated := hasLegacyTopLevel
	for index, profile := range settings.ImageAPIProfiles {
		var changed bool
		settings.ImageAPIProfiles[index], changed = migrateLegacyImageAPIProfile(profile)
		if changed && !hasLegacyTopLevel && imageAPIProfileID(settings.ImageAPIProfiles[index]) == DefaultImageAPIProfileID &&
			strings.TrimSpace(settings.ImageAPIProfiles[index].Model) == "" {
			settings.ImageAPIProfiles[index].Model = legacyDefaultImageAPIModel
		}
		migrated = migrated || changed
	}
	if hasLegacyTopLevel {
		settings.ImageAPIProfiles = applyLegacyTopLevelImageSettings(settings.ImageAPIProfiles, settings)
	}
	settings.LegacyImageAPIKey = nil
	settings.LegacyImageAPIBaseURL = nil
	settings.LegacyImageAPIModel = nil
	return settings, migrated
}

func migrateLegacyImageAPIProfile(profile ImageAPIProfileSettings) (ImageAPIProfileSettings, bool) {
	if profile.LegacyOpenAIAPIKey == nil && profile.LegacyOpenAIBaseURL == nil && profile.LegacyOpenAIModel == nil {
		return profile, false
	}
	if !imageProfileUsesOpenAIRouting(profile) {
		profile.LegacyOpenAIAPIKey = nil
		profile.LegacyOpenAIBaseURL = nil
		profile.LegacyOpenAIModel = nil
		return profile, true
	}
	if strings.TrimSpace(profile.Provider) == "" {
		profile.Provider = ImageProviderOpenAI
	}
	if strings.TrimSpace(profile.Protocol) == "" {
		profile.Protocol = ImageProtocolOpenAI
	}
	if profile.APIKey == "" && profile.LegacyOpenAIAPIKey != nil {
		profile.APIKey = *profile.LegacyOpenAIAPIKey
	}
	if strings.TrimSpace(profile.BaseURL) == "" && profile.LegacyOpenAIBaseURL != nil {
		profile.BaseURL = *profile.LegacyOpenAIBaseURL
	}
	if strings.TrimSpace(profile.Model) == "" && profile.LegacyOpenAIModel != nil {
		profile.Model = *profile.LegacyOpenAIModel
	}
	if strings.TrimSpace(profile.Model) == "" {
		switch id := normalizeImageAPIProfileID(profile.ID); id {
		case "", DefaultImageAPIProfileID:
		default:
			profile.Model = id
		}
	}
	profile.LegacyOpenAIAPIKey = nil
	profile.LegacyOpenAIBaseURL = nil
	profile.LegacyOpenAIModel = nil
	return profile, true
}

func applyLegacyTopLevelImageSettings(profiles []ImageAPIProfileSettings, settings Settings) []ImageAPIProfileSettings {
	legacyKey := legacyImageSettingValue(settings.LegacyImageAPIKey)
	legacyBaseURL := legacyImageSettingValue(settings.LegacyImageAPIBaseURL)
	legacyModel := legacyImageSettingValue(settings.LegacyImageAPIModel)
	hasCompatibleDefault := false
	for index, profile := range profiles {
		id := imageAPIProfileID(profile)
		if id == "" || !imageProfileUsesOpenAIRouting(profile) {
			continue
		}
		if strings.TrimSpace(profile.Provider) == "" {
			profile.Provider = ImageProviderOpenAI
		}
		if strings.TrimSpace(profile.Protocol) == "" {
			profile.Protocol = ImageProtocolOpenAI
		}
		if profile.APIKey == "" {
			profile.APIKey = legacyKey
		}
		if strings.TrimSpace(profile.BaseURL) == "" {
			profile.BaseURL = legacyBaseURL
		}
		if strings.TrimSpace(profile.Model) == "" {
			if id == DefaultImageAPIProfileID {
				profile.Model = firstNonEmpty(legacyModel, legacyDefaultImageAPIModel)
			} else {
				profile.Model = id
			}
		}
		profiles[index] = profile
		if id == DefaultImageAPIProfileID {
			hasCompatibleDefault = true
		}
	}
	if hasCompatibleDefault {
		return profiles
	}

	profileID := DefaultImageAPIProfileID
	for _, profile := range profiles {
		if imageAPIProfileID(profile) == profileID {
			profileID = uniqueMigratedImageProfileID("legacy-openai-image", profiles)
			break
		}
	}
	return append(profiles, ImageAPIProfileSettings{
		ID:                  profileID,
		Provider:            ImageProviderOpenAI,
		Protocol:            ImageProtocolOpenAI,
		APIKey:              legacyKey,
		BaseURL:             firstNonEmpty(legacyBaseURL, DefaultImageAPIBaseURL),
		Model:               firstNonEmpty(legacyModel, legacyDefaultImageAPIModel),
		DefaultQuality:      "auto",
		DefaultOutputFormat: "png",
	})
}

func imageProfileUsesOpenAIRouting(profile ImageAPIProfileSettings) bool {
	provider := strings.ToLower(strings.TrimSpace(profile.Provider))
	protocol := normalizeImageAPIProtocol(profile.Protocol)
	return (provider == "" || provider == ImageProviderOpenAI) &&
		(protocol == "" || protocol == ImageProtocolOpenAI)
}

func hasLegacyTopLevelImageSettings(settings Settings) bool {
	return settings.LegacyImageAPIKey != nil || settings.LegacyImageAPIBaseURL != nil || settings.LegacyImageAPIModel != nil
}

func hasLegacyImageSettings(settings Settings) bool {
	if hasLegacyTopLevelImageSettings(settings) {
		return true
	}
	for _, profile := range settings.ImageAPIProfiles {
		if profile.LegacyOpenAIAPIKey != nil || profile.LegacyOpenAIBaseURL != nil || profile.LegacyOpenAIModel != nil {
			return true
		}
	}
	return false
}

func legacyImageSettingValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func uniqueMigratedImageProfileID(base string, profiles []ImageAPIProfileSettings) string {
	used := make(map[string]struct{}, len(profiles))
	for _, profile := range profiles {
		if id := imageAPIProfileID(profile); id != "" {
			used[id] = struct{}{}
		}
	}
	if _, exists := used[base]; !exists {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := base + "-" + strconv.Itoa(suffix)
		if _, exists := used[candidate]; !exists {
			return candidate
		}
	}
}
