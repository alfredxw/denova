package config

import (
	"errors"
	"fmt"
	"strings"
)

const (
	DefaultImageAPIProfileID = "default"
	DefaultImageAPIProvider  = "openai"
	DefaultImageAPIBaseURL   = "https://api.openai.com/v1"
	DefaultImageAPIModel     = "gpt-image-1"
	DefaultImageAPISize      = "1024x1024"
	DefaultImageAPIQuality   = "auto"
	DefaultImageAPIFormat    = "png"
)

var (
	ErrImageAPIProfileNotFound = errors.New("图片 API 配置不存在")
	ErrImageAPIKeyMissing      = errors.New("图片 API Key 未配置")
	ErrImageAPIModelMissing    = errors.New("图片模型未配置")
)

type ImageAPIProfileSettings struct {
	ID                  string `toml:"id,omitempty" json:"id,omitempty"`
	Name                string `toml:"name,omitempty" json:"name,omitempty"`
	Provider            string `toml:"provider,omitempty" json:"provider,omitempty"`
	OpenAIAPIKey        string `toml:"openai_api_key,omitempty" json:"openai_api_key,omitempty"`
	OpenAIBaseURL       string `toml:"openai_base_url,omitempty" json:"openai_base_url,omitempty"`
	OpenAIModel         string `toml:"openai_model,omitempty" json:"openai_model,omitempty"`
	DefaultSize         string `toml:"default_size,omitempty" json:"default_size,omitempty"`
	DefaultQuality      string `toml:"default_quality,omitempty" json:"default_quality,omitempty"`
	DefaultOutputFormat string `toml:"default_output_format,omitempty" json:"default_output_format,omitempty"`
}

type ResolvedImageAPIProfile struct {
	ProfileID     string
	Name          string
	Provider      string
	OpenAIAPIKey  string
	OpenAIBaseURL string
	OpenAIModel   string
	Size          string
	Quality       string
	OutputFormat  string
}

func ResolveImageAPIProfile(cfg *Config, requestedID string) (ResolvedImageAPIProfile, error) {
	if cfg == nil {
		return ResolvedImageAPIProfile{}, ErrImageAPIProfileNotFound
	}
	profiles := map[string]ImageAPIProfileSettings{
		DefaultImageAPIProfileID: legacyImageAPIProfile(cfg),
	}
	for _, profile := range cfg.ImageAPIProfiles {
		id := imageAPIProfileID(profile)
		if id == "" {
			continue
		}
		base := profiles[id]
		profile.ID = id
		profiles[id] = mergeImageAPIProfile(base, profile)
	}

	profileID := normalizeImageAPIProfileID(requestedID)
	if profileID == "" {
		profileID = normalizeImageAPIProfileID(cfg.DefaultImageAPIProfileID)
	}
	if profileID == "" {
		profileID = DefaultImageAPIProfileID
	}
	profile, ok := profiles[profileID]
	if !ok {
		return ResolvedImageAPIProfile{}, fmt.Errorf("%w: %s", ErrImageAPIProfileNotFound, profileID)
	}
	if profile.Provider == "" {
		profile.Provider = DefaultImageAPIProvider
	}
	if profile.OpenAIAPIKey == "" {
		profile.OpenAIAPIKey = cfg.ImageAPIKey
	}
	if profile.OpenAIBaseURL == "" {
		profile.OpenAIBaseURL = cfg.ImageAPIBaseURL
	}
	if profile.OpenAIBaseURL == "" {
		profile.OpenAIBaseURL = DefaultImageAPIBaseURL
	}
	if profile.OpenAIModel == "" {
		profile.OpenAIModel = cfg.ImageAPIModel
	}
	if profile.OpenAIModel == "" {
		profile.OpenAIModel = DefaultImageAPIModel
	}
	if profile.DefaultSize == "" {
		profile.DefaultSize = DefaultImageAPISize
	}
	if profile.DefaultQuality == "" {
		profile.DefaultQuality = DefaultImageAPIQuality
	}
	if profile.DefaultOutputFormat == "" {
		profile.DefaultOutputFormat = DefaultImageAPIFormat
	}
	if strings.EqualFold(profile.Provider, DefaultImageAPIProvider) && strings.TrimSpace(profile.OpenAIAPIKey) == "" {
		return ResolvedImageAPIProfile{}, ErrImageAPIKeyMissing
	}
	if strings.EqualFold(profile.Provider, DefaultImageAPIProvider) && strings.TrimSpace(profile.OpenAIModel) == "" {
		return ResolvedImageAPIProfile{}, ErrImageAPIModelMissing
	}
	return ResolvedImageAPIProfile{
		ProfileID:     profileID,
		Name:          strings.TrimSpace(profile.Name),
		Provider:      normalizeImageAPIProvider(profile.Provider),
		OpenAIAPIKey:  strings.TrimSpace(profile.OpenAIAPIKey),
		OpenAIBaseURL: strings.TrimSpace(profile.OpenAIBaseURL),
		OpenAIModel:   strings.TrimSpace(profile.OpenAIModel),
		Size:          normalizeImageAPISize(profile.DefaultSize),
		Quality:       normalizeImageAPIQuality(profile.DefaultQuality),
		OutputFormat:  normalizeImageAPIOutputFormat(profile.DefaultOutputFormat),
	}, nil
}

func mergeImageAPIProfiles(parent, child []ImageAPIProfileSettings) []ImageAPIProfileSettings {
	if len(child) == 0 {
		return parent
	}
	out := make([]ImageAPIProfileSettings, 0, len(parent)+len(child))
	index := make(map[string]int, len(parent)+len(child))
	for _, profile := range parent {
		id := imageAPIProfileID(profile)
		if id == "" {
			continue
		}
		profile.ID = id
		index[id] = len(out)
		out = append(out, profile)
	}
	for _, profile := range child {
		id := imageAPIProfileID(profile)
		if id == "" {
			continue
		}
		profile.ID = id
		if i, ok := index[id]; ok {
			out[i] = mergeImageAPIProfile(out[i], profile)
		} else {
			index[id] = len(out)
			out = append(out, profile)
		}
	}
	return out
}

func sanitizeImageAPIProfiles(profiles []ImageAPIProfileSettings) []ImageAPIProfileSettings {
	if len(profiles) == 0 {
		return profiles
	}
	out := make([]ImageAPIProfileSettings, 0, len(profiles))
	for _, profile := range profiles {
		profile.OpenAIModel = strings.TrimSpace(profile.OpenAIModel)
		profile.ID = imageAPIProfileID(profile)
		if profile.ID == "" {
			continue
		}
		if profile.OpenAIModel == "" && profile.ID != DefaultImageAPIProfileID {
			profile.OpenAIModel = profile.ID
		}
		profile.Name = strings.TrimSpace(profile.Name)
		profile.Provider = normalizeImageAPIProvider(profile.Provider)
		profile.OpenAIBaseURL = strings.TrimSpace(profile.OpenAIBaseURL)
		profile.DefaultSize = normalizeImageAPISize(profile.DefaultSize)
		profile.DefaultQuality = normalizeImageAPIQuality(profile.DefaultQuality)
		profile.DefaultOutputFormat = normalizeImageAPIOutputFormat(profile.DefaultOutputFormat)
		out = append(out, profile)
	}
	return out
}

func mergeImageAPIProfile(parent, child ImageAPIProfileSettings) ImageAPIProfileSettings {
	out := parent
	if id := imageAPIProfileID(child); id != "" {
		out.ID = id
	}
	if child.Name != "" {
		out.Name = strings.TrimSpace(child.Name)
	}
	if child.Provider != "" {
		out.Provider = normalizeImageAPIProvider(child.Provider)
	}
	if child.OpenAIAPIKey != "" {
		out.OpenAIAPIKey = child.OpenAIAPIKey
	}
	if child.OpenAIBaseURL != "" {
		out.OpenAIBaseURL = strings.TrimSpace(child.OpenAIBaseURL)
	}
	if child.OpenAIModel != "" {
		out.OpenAIModel = strings.TrimSpace(child.OpenAIModel)
	}
	if child.DefaultSize != "" {
		out.DefaultSize = normalizeImageAPISize(child.DefaultSize)
	}
	if child.DefaultQuality != "" {
		out.DefaultQuality = normalizeImageAPIQuality(child.DefaultQuality)
	}
	if child.DefaultOutputFormat != "" {
		out.DefaultOutputFormat = normalizeImageAPIOutputFormat(child.DefaultOutputFormat)
	}
	return out
}

func legacyImageAPIProfile(cfg *Config) ImageAPIProfileSettings {
	return ImageAPIProfileSettings{
		ID:                  DefaultImageAPIProfileID,
		Name:                "默认图片 API",
		Provider:            DefaultImageAPIProvider,
		OpenAIAPIKey:        cfg.ImageAPIKey,
		OpenAIBaseURL:       firstNonEmpty(cfg.ImageAPIBaseURL, DefaultImageAPIBaseURL),
		OpenAIModel:         firstNonEmpty(cfg.ImageAPIModel, DefaultImageAPIModel),
		DefaultSize:         DefaultImageAPISize,
		DefaultQuality:      DefaultImageAPIQuality,
		DefaultOutputFormat: DefaultImageAPIFormat,
	}
}

func normalizeImageAPIProfileID(id string) string {
	return strings.TrimSpace(id)
}

func imageAPIProfileID(profile ImageAPIProfileSettings) string {
	if id := normalizeImageAPIProfileID(profile.ID); id != "" {
		return id
	}
	return strings.TrimSpace(profile.OpenAIModel)
}

func normalizeImageAPIProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", DefaultImageAPIProvider:
		return DefaultImageAPIProvider
	default:
		return ""
	}
}

func normalizeImageAPISize(size string) string {
	switch strings.TrimSpace(size) {
	case "", "auto":
		return ""
	case "256x256", "512x512", "1024x1024", "1536x1024", "1024x1536", "1792x1024", "1024x1792":
		return strings.TrimSpace(size)
	default:
		return ""
	}
}

func normalizeImageAPIQuality(quality string) string {
	switch strings.TrimSpace(quality) {
	case "", "auto":
		return ""
	case "standard", "hd", "low", "medium", "high":
		return strings.TrimSpace(quality)
	default:
		return ""
	}
}

func normalizeImageAPIOutputFormat(format string) string {
	switch strings.TrimSpace(format) {
	case "", "png":
		return ""
	case "jpeg", "webp":
		return strings.TrimSpace(format)
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
