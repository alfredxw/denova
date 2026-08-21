package config

import "strings"

func normalizeImageAPIProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", ImageProviderOpenAI:
		return ImageProviderOpenAI
	case ImageProviderXAI, ImageProviderComfyUI, ImageProviderVolcengine, ImageProviderGoogle, ImageProviderCustom:
		return strings.ToLower(strings.TrimSpace(provider))
	default:
		return ""
	}
}

func normalizeImageAPIProtocol(protocol string) string {
	return strings.ToLower(strings.TrimSpace(protocol))
}

func isSupportedImageAPIProtocol(protocol string) bool {
	switch normalizeImageAPIProtocol(protocol) {
	case ImageProtocolOpenAI, ImageProtocolXAI, ImageProtocolComfyUI, ImageProtocolArk, ImageProtocolGemini:
		return true
	default:
		return false
	}
}

func imageDefaultsForProvider(provider string) (imageProviderDefaults, bool) {
	switch normalizeImageAPIProvider(provider) {
	case ImageProviderOpenAI:
		return imageProviderDefaults{Protocol: ImageProtocolOpenAI, BaseURL: DefaultImageAPIBaseURL, Model: DefaultImageAPIModel, Quality: "auto", OutputFormat: "png"}, true
	case ImageProviderXAI:
		return imageProviderDefaults{Protocol: ImageProtocolXAI, BaseURL: "https://api.x.ai/v1", Model: "grok-imagine-image-2.0", Resolution: "1k", Quality: "medium"}, true
	case ImageProviderComfyUI:
		return imageProviderDefaults{Protocol: ImageProtocolComfyUI, BaseURL: "http://127.0.0.1:8188", Size: "1024x1024"}, true
	case ImageProviderVolcengine:
		return imageProviderDefaults{Protocol: ImageProtocolArk, BaseURL: "https://ark.cn-beijing.volces.com/api/v3", Model: "doubao-seedream-5-0-260128", Resolution: "2K", OutputFormat: "png"}, true
	case ImageProviderGoogle:
		return imageProviderDefaults{Protocol: ImageProtocolGemini, BaseURL: "https://generativelanguage.googleapis.com/v1", Model: "gemini-3.1-flash-image", Resolution: "1K"}, true
	case ImageProviderCustom:
		return imageProviderDefaults{Protocol: ImageProtocolOpenAI}, true
	default:
		return imageProviderDefaults{}, false
	}
}

func imageProviderRequiresAPIKey(provider string) bool {
	switch normalizeImageAPIProvider(provider) {
	case ImageProviderOpenAI, ImageProviderXAI, ImageProviderVolcengine, ImageProviderGoogle:
		return true
	default:
		return false
	}
}

func normalizeComfyUIProfile(settings *ComfyUIProfileSettings) ComfyUIProfileSettings {
	if settings == nil {
		return ComfyUIProfileSettings{WorkflowMode: ComfyUIWorkflowBuiltin}
	}
	out := ComfyUIProfileSettings{
		WorkflowMode: strings.ToLower(strings.TrimSpace(settings.WorkflowMode)),
		Workflow:     strings.TrimSpace(settings.Workflow),
		WorkflowName: strings.TrimSpace(settings.WorkflowName),
	}
	if out.WorkflowMode != ComfyUIWorkflowAPI {
		out.WorkflowMode = ComfyUIWorkflowBuiltin
		out.Workflow = ""
		out.WorkflowName = ""
	}
	return out
}

func normalizeImageAPISize(size string) string {
	size = strings.TrimSpace(size)
	if strings.EqualFold(size, "auto") {
		return ""
	}
	return size
}

func normalizeImageAPIAspectRatio(ratio string) string {
	ratio = strings.ToLower(strings.TrimSpace(ratio))
	if ratio == "auto" {
		return ""
	}
	return ratio
}

func normalizeImageAPIResolution(resolution string) string { return strings.TrimSpace(resolution) }

func normalizeImageAPIQuality(quality string) string {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "", "auto":
		return ""
	case "standard", "hd", "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(quality))
	default:
		return ""
	}
}

func normalizeImageAPIOutputFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "auto":
		return ""
	case "jpg":
		return "jpeg"
	case "jpeg", "png", "webp":
		return strings.ToLower(strings.TrimSpace(format))
	default:
		return ""
	}
}

func hasImageAPIProfileDraftFields(profile ImageAPIProfileSettings) bool {
	return strings.TrimSpace(profile.Name) != "" ||
		strings.TrimSpace(profile.Provider) != "" ||
		strings.TrimSpace(profile.Protocol) != "" ||
		profile.APIKey != "" ||
		strings.TrimSpace(profile.BaseURL) != "" ||
		strings.TrimSpace(profile.Model) != "" ||
		strings.TrimSpace(profile.PromptGuide) != "" ||
		len(profile.Headers) > 0 ||
		strings.TrimSpace(profile.DefaultSize) != "" ||
		strings.TrimSpace(profile.DefaultAspectRatio) != "" ||
		strings.TrimSpace(profile.DefaultResolution) != "" ||
		strings.TrimSpace(profile.DefaultQuality) != "" ||
		strings.TrimSpace(profile.DefaultOutputFormat) != "" ||
		profile.ComfyUI != nil
}

func sanitizeImageHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneImageHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		out[key] = value
	}
	return out
}
