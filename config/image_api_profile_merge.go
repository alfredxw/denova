package config

import (
	"net/url"
	"strings"
)

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
			if hasImageAPIProfileDraftFields(profile) {
				out = append(out, profile)
			}
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
		profile.Name = strings.TrimSpace(profile.Name)
		if profile.Provider != "" {
			provider := normalizeImageAPIProvider(profile.Provider)
			if provider == "" {
				provider = strings.ToLower(strings.TrimSpace(profile.Provider))
			}
			profile.Provider = provider
		}
		profile.Protocol = normalizeImageAPIProtocol(profile.Protocol)
		profile.BaseURL = strings.TrimSpace(profile.BaseURL)
		profile.Model = strings.TrimSpace(profile.Model)
		profile.Headers = sanitizeImageHeaders(profile.Headers)
		profile.DefaultSize = normalizeImageAPISize(profile.DefaultSize)
		profile.DefaultAspectRatio = normalizeImageAPIAspectRatio(profile.DefaultAspectRatio)
		profile.DefaultResolution = normalizeImageAPIResolution(profile.DefaultResolution)
		profile.DefaultQuality = normalizeImageAPIQuality(profile.DefaultQuality)
		profile.DefaultOutputFormat = normalizeImageAPIOutputFormat(profile.DefaultOutputFormat)
		if profile.ComfyUI != nil {
			normalized := normalizeComfyUIProfile(profile.ComfyUI)
			profile.ComfyUI = &normalized
		}
		profile.ID = imageAPIProfileID(profile)
		if profile.ID == "" {
			if hasImageAPIProfileDraftFields(profile) {
				out = append(out, profile)
			}
			continue
		}
		if profile.Model == "" && profile.ID != DefaultImageAPIProfileID && profile.Provider != ImageProviderComfyUI {
			profile.Model = profile.ID
		}
		out = append(out, profile)
	}
	return out
}

func mergeImageAPIProfile(parent, child ImageAPIProfileSettings) ImageAPIProfileSettings {
	out := parent
	previousProtocol := normalizeImageAPIProtocol(out.Protocol)
	if previousProtocol == "" {
		if defaults, ok := imageDefaultsForProvider(normalizeImageAPIProvider(out.Provider)); ok {
			previousProtocol = defaults.Protocol
		}
	}
	previousScope := imageAPICredentialScope(out)
	if id := imageAPIProfileID(child); id != "" {
		out.ID = id
	}
	out.Name = strings.TrimSpace(child.Name)
	if child.Provider != "" {
		provider := normalizeImageAPIProvider(child.Provider)
		if provider == "" {
			provider = strings.ToLower(strings.TrimSpace(child.Provider))
		}
		providerChanged := normalizeImageAPIProvider(out.Provider) != provider
		out.Provider = provider
		if providerChanged {
			out.Protocol = ""
			out.BaseURL = ""
			out.Model = ""
			out.ComfyUI = nil
			clearImageProtocolDefaults(&out)
		}
	}
	if child.Protocol != "" {
		protocol := normalizeImageAPIProtocol(child.Protocol)
		if previousProtocol != "" && protocol != previousProtocol {
			out.Model = ""
			out.ComfyUI = nil
			clearImageProtocolDefaults(&out)
		}
		out.Protocol = protocol
	}
	if child.BaseURL != "" {
		out.BaseURL = strings.TrimSpace(child.BaseURL)
	}
	if previousScope != imageAPICredentialScope(out) {
		out.APIKey = ""
		out.Headers = nil
	}
	if child.APIKey != "" {
		out.APIKey = child.APIKey
	}
	if child.Model != "" {
		out.Model = strings.TrimSpace(child.Model)
	}
	if child.Headers != nil {
		out.Headers = cloneImageHeaders(child.Headers)
	}
	if child.DefaultSize != "" {
		out.DefaultSize = normalizeImageAPISize(child.DefaultSize)
	}
	if child.DefaultAspectRatio != "" {
		out.DefaultAspectRatio = normalizeImageAPIAspectRatio(child.DefaultAspectRatio)
	}
	if child.DefaultResolution != "" {
		out.DefaultResolution = normalizeImageAPIResolution(child.DefaultResolution)
	}
	if child.DefaultQuality != "" {
		out.DefaultQuality = normalizeImageAPIQuality(child.DefaultQuality)
	}
	if child.DefaultOutputFormat != "" {
		out.DefaultOutputFormat = normalizeImageAPIOutputFormat(child.DefaultOutputFormat)
	}
	if child.ComfyUI != nil {
		value := normalizeComfyUIProfile(child.ComfyUI)
		out.ComfyUI = &value
	}
	return out
}

func clearImageProtocolDefaults(profile *ImageAPIProfileSettings) {
	profile.DefaultSize = ""
	profile.DefaultAspectRatio = ""
	profile.DefaultResolution = ""
	profile.DefaultQuality = ""
	profile.DefaultOutputFormat = ""
}

func imageAPICredentialScope(profile ImageAPIProfileSettings) string {
	baseURL := strings.TrimSpace(profile.BaseURL)
	if baseURL == "" {
		if defaults, ok := imageDefaultsForProvider(normalizeImageAPIProvider(profile.Provider)); ok {
			baseURL = defaults.BaseURL
		}
	}
	if parsed, err := url.Parse(baseURL); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return "origin:" + strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host)
	}
	return "endpoint:" + strings.ToLower(strings.TrimRight(baseURL, "/"))
}

func imageAPIProfileID(profile ImageAPIProfileSettings) string {
	if id := normalizeImageAPIProfileID(profile.ID); id != "" {
		return id
	}
	return strings.TrimSpace(profile.Model)
}

func normalizeImageAPIProfileID(id string) string { return strings.TrimSpace(id) }
