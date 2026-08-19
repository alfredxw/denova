package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	DefaultImageAPIProfileID = "default"

	ImageProviderOpenAI     = "openai"
	ImageProviderXAI        = "xai"
	ImageProviderComfyUI    = "comfyui"
	ImageProviderVolcengine = "volcengine"
	ImageProviderGoogle     = "google"
	ImageProviderCustom     = "custom"

	ImageProtocolOpenAI  = "openai-images"
	ImageProtocolXAI     = "xai-images"
	ImageProtocolComfyUI = "comfyui-workflow"
	ImageProtocolArk     = "ark-images"
	ImageProtocolGemini  = "gemini-images"

	DefaultImageAPIProvider = ImageProviderOpenAI
	DefaultImageAPIProtocol = ImageProtocolOpenAI
	DefaultImageAPIBaseURL  = "https://api.openai.com/v1"
	DefaultImageAPIModel    = "gpt-image-2"

	ComfyUIWorkflowBuiltin = "builtin"
	ComfyUIWorkflowAPI     = "api"
)

var (
	ErrImageAPIProfileNotFound = errors.New("image model profile not found")
	ErrImageAPIKeyMissing      = errors.New("image model API key is missing")
	ErrImageAPIModelMissing    = errors.New("image model is missing")
	ErrComfyUIWorkflowMissing  = errors.New("ComfyUI API-format workflow is missing")
	ErrImageProviderInvalid    = errors.New("image provider is invalid")
	ErrImageProtocolInvalid    = errors.New("image protocol is invalid")
)

// ComfyUIProfileSettings owns the workflow source used by the ComfyUI
// protocol. Builtin mode uses Denova's core-node text-to-image graph; API mode
// executes an uploaded ComfyUI API-format graph.
type ComfyUIProfileSettings struct {
	WorkflowMode string `toml:"workflow_mode,omitempty" json:"workflow_mode,omitempty"`
	Workflow     string `toml:"workflow,omitempty" json:"workflow,omitempty"`
	WorkflowName string `toml:"workflow_name,omitempty" json:"workflow_name,omitempty"`
}

// ImageAPIProfileSettings is provider-neutral persistent configuration. A
// provider supplies ergonomic defaults while Protocol selects the wire format.
// Custom profiles can combine their own endpoint with any installed protocol.
type ImageAPIProfileSettings struct {
	ID       string `toml:"id,omitempty" json:"id,omitempty"`
	Name     string `toml:"name,omitempty" json:"name,omitempty"`
	Provider string `toml:"provider,omitempty" json:"provider,omitempty"`
	Protocol string `toml:"protocol,omitempty" json:"protocol,omitempty"`
	APIKey   string `toml:"api_key,omitempty" json:"api_key,omitempty"`
	BaseURL  string `toml:"base_url,omitempty" json:"base_url,omitempty"`
	Model    string `toml:"model,omitempty" json:"model,omitempty"`
	// LegacyOpenAI* are presence-aware decode aliases for image profiles saved
	// before provider/protocol adapters. Migration clears them before writes.
	LegacyOpenAIAPIKey  *string                 `toml:"openai_api_key,omitempty" json:"openai_api_key,omitempty"`
	LegacyOpenAIBaseURL *string                 `toml:"openai_base_url,omitempty" json:"openai_base_url,omitempty"`
	LegacyOpenAIModel   *string                 `toml:"openai_model,omitempty" json:"openai_model,omitempty"`
	Headers             map[string]string       `toml:"headers,omitempty" json:"headers,omitempty"`
	DefaultSize         string                  `toml:"default_size,omitempty" json:"default_size,omitempty"`
	DefaultAspectRatio  string                  `toml:"default_aspect_ratio,omitempty" json:"default_aspect_ratio,omitempty"`
	DefaultResolution   string                  `toml:"default_resolution,omitempty" json:"default_resolution,omitempty"`
	DefaultQuality      string                  `toml:"default_quality,omitempty" json:"default_quality,omitempty"`
	DefaultOutputFormat string                  `toml:"default_output_format,omitempty" json:"default_output_format,omitempty"`
	ComfyUI             *ComfyUIProfileSettings `toml:"comfyui,omitempty" json:"comfyui,omitempty"`
}

type ResolvedImageAPIProfile struct {
	ProfileID    string
	Name         string
	Provider     string
	Protocol     string
	APIKey       string
	BaseURL      string
	Model        string
	Headers      map[string]string
	Size         string
	AspectRatio  string
	Resolution   string
	Quality      string
	OutputFormat string
	ComfyUI      ComfyUIProfileSettings
}

type imageProviderDefaults struct {
	Protocol     string
	BaseURL      string
	Model        string
	Size         string
	Resolution   string
	Quality      string
	OutputFormat string
}

func DefaultImageAPIProfile() ImageAPIProfileSettings {
	return ImageAPIProfileSettings{
		ID:                  DefaultImageAPIProfileID,
		Name:                "Default image model",
		Provider:            DefaultImageAPIProvider,
		Protocol:            DefaultImageAPIProtocol,
		BaseURL:             DefaultImageAPIBaseURL,
		Model:               DefaultImageAPIModel,
		DefaultQuality:      "auto",
		DefaultOutputFormat: "png",
	}
}

// ApplyImageAPIEnvironment applies process-local overrides to the default
// image profile without leaking environment secrets into persisted settings.
func ApplyImageAPIEnvironment(cfg *Config) {
	if cfg == nil {
		return
	}
	provider := strings.TrimSpace(os.Getenv("DENOVA_IMAGE_PROVIDER"))
	protocol := strings.TrimSpace(os.Getenv("DENOVA_IMAGE_PROTOCOL"))
	allowLegacyOpenAI := (provider == "" || normalizeImageAPIProvider(provider) == ImageProviderOpenAI) &&
		(protocol == "" || normalizeImageAPIProtocol(protocol) == ImageProtocolOpenAI)
	apiKey, legacyKey := imageEnvironmentValue("DENOVA_IMAGE_API_KEY", "OPENAI_IMAGE_API_KEY", allowLegacyOpenAI)
	baseURL, legacyBaseURL := imageEnvironmentValue("DENOVA_IMAGE_BASE_URL", "OPENAI_IMAGE_BASE_URL", allowLegacyOpenAI)
	model, legacyModel := imageEnvironmentValue("DENOVA_IMAGE_MODEL", "OPENAI_IMAGE_MODEL", allowLegacyOpenAI)
	if legacyKey || legacyBaseURL || legacyModel {
		if provider == "" {
			provider = ImageProviderOpenAI
		}
		if protocol == "" {
			protocol = ImageProtocolOpenAI
		}
	}
	override := ImageAPIProfileSettings{
		ID:       DefaultImageAPIProfileID,
		Provider: provider,
		Protocol: protocol,
		APIKey:   apiKey,
		BaseURL:  strings.TrimSpace(baseURL),
		Model:    strings.TrimSpace(model),
	}
	if !hasImageAPIProfileDraftFields(override) {
		return
	}
	found := false
	for index, profile := range cfg.ImageAPIProfiles {
		if imageAPIProfileID(profile) != DefaultImageAPIProfileID {
			continue
		}
		cfg.ImageAPIProfiles[index] = mergeImageAPIProfile(profile, override)
		found = true
		break
	}
	if !found {
		cfg.ImageAPIProfiles = append(cfg.ImageAPIProfiles, mergeImageAPIProfile(DefaultImageAPIProfile(), override))
	}
}

func imageEnvironmentValue(current, legacy string, allowLegacy bool) (string, bool) {
	if value, exists := os.LookupEnv(current); exists {
		return value, false
	}
	if allowLegacy {
		if value, exists := os.LookupEnv(legacy); exists {
			return value, true
		}
	}
	return "", false
}

func ResolveImageAPIProfile(cfg *Config, requestedID string) (ResolvedImageAPIProfile, error) {
	if cfg == nil {
		return ResolvedImageAPIProfile{}, ErrImageAPIProfileNotFound
	}
	profiles := map[string]ImageAPIProfileSettings{
		DefaultImageAPIProfileID: DefaultImageAPIProfile(),
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
	return resolveImageAPIProfile(profileID, profile)
}

func resolveImageAPIProfile(profileID string, profile ImageAPIProfileSettings) (ResolvedImageAPIProfile, error) {
	provider := normalizeImageAPIProvider(profile.Provider)
	if provider == "" {
		return ResolvedImageAPIProfile{}, fmt.Errorf("%w: %s", ErrImageProviderInvalid, profile.Provider)
	}
	defaults, ok := imageDefaultsForProvider(provider)
	if !ok {
		return ResolvedImageAPIProfile{}, fmt.Errorf("%w: %s", ErrImageProviderInvalid, provider)
	}
	protocol := normalizeImageAPIProtocol(profile.Protocol)
	if protocol == "" {
		protocol = defaults.Protocol
	}
	if !isSupportedImageAPIProtocol(protocol) {
		return ResolvedImageAPIProfile{}, fmt.Errorf("%w: %s", ErrImageProtocolInvalid, protocol)
	}

	baseURL := firstNonEmpty(strings.TrimSpace(profile.BaseURL), defaults.BaseURL)
	model := firstNonEmpty(strings.TrimSpace(profile.Model), defaults.Model)
	comfy := normalizeComfyUIProfile(profile.ComfyUI)
	if protocol == ImageProtocolComfyUI && comfy.WorkflowMode == ComfyUIWorkflowBuiltin && model == "" {
		return ResolvedImageAPIProfile{}, ErrImageAPIModelMissing
	}
	if protocol == ImageProtocolComfyUI && comfy.WorkflowMode == ComfyUIWorkflowAPI && comfy.Workflow == "" {
		return ResolvedImageAPIProfile{}, ErrComfyUIWorkflowMissing
	}
	if protocol != ImageProtocolComfyUI && model == "" {
		return ResolvedImageAPIProfile{}, ErrImageAPIModelMissing
	}
	if imageProviderRequiresAPIKey(provider) && strings.TrimSpace(profile.APIKey) == "" {
		return ResolvedImageAPIProfile{}, ErrImageAPIKeyMissing
	}
	if baseURL == "" {
		return ResolvedImageAPIProfile{}, fmt.Errorf("image model base URL is missing")
	}

	return ResolvedImageAPIProfile{
		ProfileID:    profileID,
		Name:         strings.TrimSpace(profile.Name),
		Provider:     provider,
		Protocol:     protocol,
		APIKey:       strings.TrimSpace(profile.APIKey),
		BaseURL:      baseURL,
		Model:        model,
		Headers:      cloneImageHeaders(profile.Headers),
		Size:         firstNonEmpty(normalizeImageAPISize(profile.DefaultSize), defaults.Size),
		AspectRatio:  normalizeImageAPIAspectRatio(profile.DefaultAspectRatio),
		Resolution:   firstNonEmpty(normalizeImageAPIResolution(profile.DefaultResolution), defaults.Resolution),
		Quality:      firstNonEmpty(normalizeImageAPIQuality(profile.DefaultQuality), defaults.Quality),
		OutputFormat: firstNonEmpty(normalizeImageAPIOutputFormat(profile.DefaultOutputFormat), defaults.OutputFormat),
		ComfyUI:      comfy,
	}, nil
}

// ResolveImageAPIProfileDraft resolves an inline, potentially unsaved profile
// for connection validation. Stored credentials may follow edits on the same
// endpoint origin, but are never copied to a different network destination.
func ResolveImageAPIProfileDraft(cfg *Config, draft ImageAPIProfileSettings) (ResolvedImageAPIProfile, error) {
	if cfg == nil {
		return ResolvedImageAPIProfile{}, fmt.Errorf("config is nil")
	}
	originalID := imageAPIProfileID(draft)
	if originalID == "" {
		return ResolvedImageAPIProfile{}, fmt.Errorf("image model profile requires an id or model")
	}
	base := ImageAPIProfileSettings{}
	if originalID == DefaultImageAPIProfileID {
		base = DefaultImageAPIProfile()
	}
	for _, profile := range cfg.ImageAPIProfiles {
		if imageAPIProfileID(profile) == originalID {
			base = profile
			break
		}
	}
	merged := mergeImageAPIProfile(base, draft)
	if draft.APIKey == "" {
		merged.APIKey = ""
		if strings.TrimSpace(base.APIKey) != "" && imageAPICredentialScope(base) == imageAPICredentialScope(merged) {
			merged.APIKey = base.APIKey
		}
	}
	return resolveImageAPIProfile(originalID, merged)
}
