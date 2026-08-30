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
	// MaxImagePromptGuideBytes bounds one model-authored context fragment while
	// leaving enough room for detailed examples and model-specific syntax.
	MaxImagePromptGuideBytes = 64 * 1024

	ComfyUIWorkflowAPI    = "api"
	ComfyUIWorkflowRemote = "remote"
)

var (
	ErrImageAPIProfileNotFound = errors.New("image model profile not found")
	ErrImageAPIKeyMissing      = errors.New("image model API key is missing")
	ErrImageAPIModelMissing    = errors.New("image model is missing")
	ErrComfyUIWorkflowMissing  = errors.New("ComfyUI API-format workflow is missing")
	ErrImageProviderInvalid    = errors.New("image provider is invalid")
	ErrImageProtocolInvalid    = errors.New("image protocol is invalid")
)

// ComfyUIInputBinding points one provider-neutral image input at a writable
// ComfyUI API-graph input. The workflow snapshot remains the source of every
// static value; bindings contain no duplicated workflow configuration.
type ComfyUIInputBinding struct {
	NodeID    string `toml:"node_id" json:"node_id"`
	InputName string `toml:"input_name" json:"input_name"`
}

// ComfyUIBindings is the complete public control surface Denova projects onto
// a discovered workflow. Width and height form the optional size capability;
// random seed handling remains an adapter policy rather than a user binding.
type ComfyUIBindings struct {
	Prompt *ComfyUIInputBinding `toml:"prompt,omitempty" json:"prompt,omitempty"`
	Count  *ComfyUIInputBinding `toml:"count,omitempty" json:"count,omitempty"`
	Width  *ComfyUIInputBinding `toml:"width,omitempty" json:"width,omitempty"`
	Height *ComfyUIInputBinding `toml:"height,omitempty" json:"height,omitempty"`
}

// ComfyUIProfileSettings owns the executable graph used by the ComfyUI
// protocol. API mode executes an imported API-format graph, while remote mode
// executes a cached snapshot discovered from a saved ComfyUI workflow.
type ComfyUIProfileSettings struct {
	WorkflowMode     string           `toml:"workflow_mode,omitempty" json:"workflow_mode,omitempty"`
	Workflow         string           `toml:"workflow,omitempty" json:"workflow,omitempty"`
	WorkflowName     string           `toml:"workflow_name,omitempty" json:"workflow_name,omitempty"`
	WorkflowID       string           `toml:"workflow_id,omitempty" json:"workflow_id,omitempty"`
	WorkflowPath     string           `toml:"workflow_path,omitempty" json:"workflow_path,omitempty"`
	WorkflowModified int64            `toml:"workflow_modified,omitempty" json:"workflow_modified,omitempty"`
	WorkflowJobID    string           `toml:"workflow_job_id,omitempty" json:"workflow_job_id,omitempty"`
	WorkflowJobTime  int64            `toml:"workflow_job_time,omitempty" json:"workflow_job_time,omitempty"`
	Bindings         *ComfyUIBindings `toml:"bindings,omitempty" json:"bindings,omitempty"`
}

// ImageAPIProfileSettings is provider-neutral persistent configuration. A
// provider supplies ergonomic defaults while Protocol selects the wire format.
// Custom profiles can combine their own endpoint with any installed protocol.
type ImageAPIProfileSettings struct {
	ID         string `toml:"id,omitempty" json:"id,omitempty"`
	Name       string `toml:"name,omitempty" json:"name,omitempty"`
	EndpointID string `toml:"endpoint_id,omitempty" json:"endpoint_id,omitempty"`
	Model      string `toml:"model,omitempty" json:"model,omitempty"`
	// Provider, Protocol, APIKey, BaseURL and Headers are decode-only fields
	// from the released profile schema. Migration moves them to a shared image
	// endpoint before settings are persisted again.
	Provider string `toml:"provider,omitempty" json:"provider,omitempty"`
	Protocol string `toml:"protocol,omitempty" json:"protocol,omitempty"`
	APIKey   string `toml:"api_key,omitempty" json:"api_key,omitempty"`
	BaseURL  string `toml:"base_url,omitempty" json:"base_url,omitempty"`
	// PromptGuide teaches prompt-authoring Agents the selected model's native
	// syntax. It is model context only and is never sent to the image provider.
	PromptGuide string `toml:"prompt_guide,omitempty" json:"prompt_guide,omitempty"`
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

// ImageAPIEndpointSettings owns one image API route and its credentials. Image
// profiles keep model, output, prompt-guide, and workflow settings only.
type ImageAPIEndpointSettings struct {
	ID       string            `toml:"id,omitempty" json:"id,omitempty"`
	Name     string            `toml:"name,omitempty" json:"name,omitempty"`
	Provider string            `toml:"provider,omitempty" json:"provider,omitempty"`
	Protocol string            `toml:"protocol,omitempty" json:"protocol,omitempty"`
	APIKey   string            `toml:"api_key,omitempty" json:"api_key,omitempty"`
	BaseURL  string            `toml:"base_url,omitempty" json:"base_url,omitempty"`
	Headers  map[string]string `toml:"headers,omitempty" json:"headers,omitempty"`
}

type ResolvedImageAPIProfile struct {
	ProfileID    string
	Name         string
	Provider     string
	Protocol     string
	APIKey       string
	BaseURL      string
	Model        string
	PromptGuide  string
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

type imageProfileResolutionPurpose int

const (
	imageProfileForGeneration imageProfileResolutionPurpose = iota
	imageProfileForConnection
)

func DefaultImageAPIProfile() ImageAPIProfileSettings {
	return ImageAPIProfileSettings{
		ID:                  DefaultImageAPIProfileID,
		Name:                "Default image model",
		EndpointID:          DefaultImageAPIProfileID,
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
	endpointOverride := ImageAPIEndpointSettings{
		ID:       DefaultImageAPIProfileID,
		Provider: provider,
		Protocol: protocol,
		APIKey:   apiKey,
		BaseURL:  strings.TrimSpace(baseURL),
	}
	model = strings.TrimSpace(model)
	if endpointOverride.Provider == "" && endpointOverride.Protocol == "" && endpointOverride.APIKey == "" && endpointOverride.BaseURL == "" && model == "" {
		return
	}
	foundEndpoint := false
	for index, endpoint := range cfg.ImageAPIEndpoints {
		if imageAPIEndpointID(endpoint) != DefaultImageAPIProfileID {
			continue
		}
		cfg.ImageAPIEndpoints[index] = mergeImageAPIEndpoint(endpoint, endpointOverride)
		foundEndpoint = true
		break
	}
	if !foundEndpoint {
		cfg.ImageAPIEndpoints = append(cfg.ImageAPIEndpoints, mergeImageAPIEndpoint(DefaultImageAPIEndpoint(), endpointOverride))
	}
	if model == "" {
		return
	}
	foundProfile := false
	for index, profile := range cfg.ImageAPIProfiles {
		if imageAPIProfileID(profile) != DefaultImageAPIProfileID {
			continue
		}
		cfg.ImageAPIProfiles[index] = mergeImageAPIProfile(profile, ImageAPIProfileSettings{ID: DefaultImageAPIProfileID, Model: model})
		foundProfile = true
		break
	}
	if !foundProfile {
		cfg.ImageAPIProfiles = append(cfg.ImageAPIProfiles, mergeImageAPIProfile(DefaultImageAPIProfile(), ImageAPIProfileSettings{ID: DefaultImageAPIProfileID, Model: model}))
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
		DefaultImageAPIProfileID: imageProfileWithEndpoint(cfg, DefaultImageAPIProfile()),
	}
	for _, profile := range cfg.ImageAPIProfiles {
		id := imageAPIProfileID(profile)
		if id == "" {
			continue
		}
		profile = imageProfileWithEndpoint(cfg, profile)
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
	return resolveImageAPIProfileForPurpose(profileID, profile, imageProfileForGeneration)
}

func resolveImageAPIProfileForPurpose(profileID string, profile ImageAPIProfileSettings, purpose imageProfileResolutionPurpose) (ResolvedImageAPIProfile, error) {
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
	if purpose == imageProfileForGeneration {
		if protocol == ImageProtocolComfyUI && comfy.Workflow == "" {
			return ResolvedImageAPIProfile{}, ErrComfyUIWorkflowMissing
		}
		if protocol != ImageProtocolComfyUI && model == "" {
			return ResolvedImageAPIProfile{}, ErrImageAPIModelMissing
		}
	}
	if imageProviderRequiresAPIKey(provider) && strings.TrimSpace(profile.APIKey) == "" {
		return ResolvedImageAPIProfile{}, ErrImageAPIKeyMissing
	}
	if baseURL == "" {
		return ResolvedImageAPIProfile{}, fmt.Errorf("image model base URL is missing")
	}
	promptGuide := strings.TrimSpace(profile.PromptGuide)
	if len([]byte(promptGuide)) > MaxImagePromptGuideBytes {
		return ResolvedImageAPIProfile{}, fmt.Errorf("image model prompt guide exceeds %d bytes", MaxImagePromptGuideBytes)
	}

	return ResolvedImageAPIProfile{
		ProfileID:    profileID,
		Name:         strings.TrimSpace(profile.Name),
		Provider:     provider,
		Protocol:     protocol,
		APIKey:       strings.TrimSpace(profile.APIKey),
		BaseURL:      baseURL,
		Model:        model,
		PromptGuide:  promptGuide,
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
	return resolveImageAPIProfileDraftForPurpose(cfg, draft, imageProfileForGeneration)
}

// ResolveImageAPIProfileEndpointDraft validates an unsaved image model with a
// shared endpoint. Stored secrets can be inherited only through the same
// endpoint ID and credential scope.
func ResolveImageAPIProfileEndpointDraft(cfg *Config, endpoint ImageAPIEndpointSettings, draft ImageAPIProfileSettings) (ResolvedImageAPIProfile, error) {
	return resolveImageAPIProfileEndpointDraftForPurpose(cfg, endpoint, draft, imageProfileForGeneration)
}

// ResolveImageAPIProfileConnectionDraft resolves endpoint credentials and
// protocol defaults without requiring a runnable model or workflow. Discovery
// services use it before the user has selected a remote workflow snapshot.
func ResolveImageAPIProfileConnectionDraft(cfg *Config, draft ImageAPIProfileSettings) (ResolvedImageAPIProfile, error) {
	return resolveImageAPIProfileDraftForPurpose(cfg, draft, imageProfileForConnection)
}

func ResolveImageAPIConnectionEndpointDraft(cfg *Config, endpoint ImageAPIEndpointSettings, draft ImageAPIProfileSettings) (ResolvedImageAPIProfile, error) {
	return resolveImageAPIProfileEndpointDraftForPurpose(cfg, endpoint, draft, imageProfileForConnection)
}

func resolveImageAPIProfileEndpointDraftForPurpose(cfg *Config, endpoint ImageAPIEndpointSettings, draft ImageAPIProfileSettings, purpose imageProfileResolutionPurpose) (ResolvedImageAPIProfile, error) {
	if cfg == nil {
		return ResolvedImageAPIProfile{}, fmt.Errorf("config is nil")
	}
	endpoint.ID = firstNonEmpty(imageAPIEndpointID(endpoint), strings.TrimSpace(draft.EndpointID), "__image_endpoint_draft__")
	if stored, ok := findImageAPIEndpoint(cfg, endpoint.ID); ok {
		endpoint = mergeImageAPIEndpoint(stored, endpoint)
	}
	draft.EndpointID = endpoint.ID
	temporary := *cfg
	temporary.ImageAPIEndpoints = append([]ImageAPIEndpointSettings(nil), cfg.ImageAPIEndpoints...)
	replaced := false
	for index, current := range temporary.ImageAPIEndpoints {
		if imageAPIEndpointID(current) == endpoint.ID {
			temporary.ImageAPIEndpoints[index] = endpoint
			replaced = true
			break
		}
	}
	if !replaced {
		temporary.ImageAPIEndpoints = append(temporary.ImageAPIEndpoints, endpoint)
	}
	return resolveImageAPIProfileDraftForPurpose(&temporary, draft, purpose)
}

func resolveImageAPIProfileDraftForPurpose(cfg *Config, draft ImageAPIProfileSettings, purpose imageProfileResolutionPurpose) (ResolvedImageAPIProfile, error) {
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
	base = imageProfileWithEndpoint(cfg, base)
	draft = imageProfileWithEndpoint(cfg, draft)
	merged := mergeImageAPIProfile(base, draft)
	if draft.APIKey == "" {
		merged.APIKey = ""
		if strings.TrimSpace(base.APIKey) != "" && imageAPICredentialScope(base) == imageAPICredentialScope(merged) {
			merged.APIKey = base.APIKey
		}
	}
	return resolveImageAPIProfileForPurpose(originalID, merged, purpose)
}
