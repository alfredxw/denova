package config

import (
	"encoding/json"
	"strconv"
	"strings"
)

func DefaultImageAPIEndpoint() ImageAPIEndpointSettings {
	return ImageAPIEndpointSettings{
		ID: DefaultImageAPIProfileID, Name: "Default image endpoint",
		Provider: DefaultImageAPIProvider, Protocol: DefaultImageAPIProtocol,
		BaseURL: DefaultImageAPIBaseURL,
	}
}

func imageAPIEndpointID(endpoint ImageAPIEndpointSettings) string {
	return strings.TrimSpace(endpoint.ID)
}

func mergeImageAPIEndpoints(parent, child []ImageAPIEndpointSettings) []ImageAPIEndpointSettings {
	if len(child) == 0 {
		return parent
	}
	out := make([]ImageAPIEndpointSettings, 0, len(parent)+len(child))
	index := make(map[string]int, len(parent)+len(child))
	for _, endpoint := range parent {
		id := imageAPIEndpointID(endpoint)
		if id == "" {
			continue
		}
		endpoint.ID = id
		index[id] = len(out)
		out = append(out, endpoint)
	}
	for _, endpoint := range child {
		id := imageAPIEndpointID(endpoint)
		if id == "" {
			continue
		}
		endpoint.ID = id
		if current, ok := index[id]; ok {
			out[current] = mergeImageAPIEndpoint(out[current], endpoint)
			continue
		}
		index[id] = len(out)
		out = append(out, endpoint)
	}
	return out
}

func mergeImageAPIEndpoint(parent, child ImageAPIEndpointSettings) ImageAPIEndpointSettings {
	merged := mergeImageAPIProfile(imageProfileFromEndpoint(parent), imageProfileFromEndpoint(child))
	return imageEndpointFromProfile(merged, firstNonEmpty(imageAPIEndpointID(child), imageAPIEndpointID(parent)))
}

func sanitizeImageAPIEndpoints(endpoints []ImageAPIEndpointSettings) []ImageAPIEndpointSettings {
	out := make([]ImageAPIEndpointSettings, 0, len(endpoints))
	seen := make(map[string]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		id := imageAPIEndpointID(endpoint)
		if id == "" {
			continue
		}
		endpoint.ID = id
		endpoint.Name = strings.TrimSpace(endpoint.Name)
		endpoint.Provider = normalizeImageAPIProvider(endpoint.Provider)
		endpoint.Protocol = normalizeImageAPIProtocol(endpoint.Protocol)
		endpoint.BaseURL = strings.TrimSpace(endpoint.BaseURL)
		endpoint.Headers = sanitizeImageHeaders(endpoint.Headers)
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, endpoint)
	}
	return out
}

func imageProfileFromEndpoint(endpoint ImageAPIEndpointSettings) ImageAPIProfileSettings {
	return ImageAPIProfileSettings{
		ID: endpoint.ID, Name: endpoint.Name, Provider: endpoint.Provider,
		Protocol: endpoint.Protocol, APIKey: endpoint.APIKey,
		BaseURL: endpoint.BaseURL, Headers: cloneImageHeaders(endpoint.Headers),
	}
}

func imageEndpointFromProfile(profile ImageAPIProfileSettings, id string) ImageAPIEndpointSettings {
	return ImageAPIEndpointSettings{
		ID: id, Name: strings.TrimSpace(profile.Name), Provider: normalizeImageAPIProvider(profile.Provider),
		Protocol: normalizeImageAPIProtocol(profile.Protocol), APIKey: profile.APIKey,
		BaseURL: strings.TrimSpace(profile.BaseURL), Headers: cloneImageHeaders(profile.Headers),
	}
}

func imageProfileWithEndpoint(cfg *Config, profile ImageAPIProfileSettings) ImageAPIProfileSettings {
	endpointID := strings.TrimSpace(profile.EndpointID)
	if endpointID == "" {
		return profile
	}
	endpoint, ok := findImageAPIEndpoint(cfg, endpointID)
	if !ok && endpointID == DefaultImageAPIProfileID {
		endpoint = DefaultImageAPIEndpoint()
		ok = true
	}
	if !ok {
		return profile
	}
	connection := imageProfileFromEndpoint(endpoint)
	connection.ID = profile.ID
	connection.Name = profile.Name
	connection.EndpointID = endpointID
	connection.Model = profile.Model
	connection.PromptGuide = profile.PromptGuide
	connection.DefaultSize = profile.DefaultSize
	connection.DefaultAspectRatio = profile.DefaultAspectRatio
	connection.DefaultResolution = profile.DefaultResolution
	connection.DefaultQuality = profile.DefaultQuality
	connection.DefaultOutputFormat = profile.DefaultOutputFormat
	connection.ComfyUI = profile.ComfyUI
	return connection
}

func findImageAPIEndpoint(cfg *Config, id string) (ImageAPIEndpointSettings, bool) {
	if cfg == nil {
		return ImageAPIEndpointSettings{}, false
	}
	for _, endpoint := range cfg.ImageAPIEndpoints {
		if imageAPIEndpointID(endpoint) == id {
			return endpoint, true
		}
	}
	return ImageAPIEndpointSettings{}, false
}

func clearImageProfileEndpoint(profile ImageAPIProfileSettings) ImageAPIProfileSettings {
	profile.Provider = ""
	profile.Protocol = ""
	profile.APIKey = ""
	profile.BaseURL = ""
	profile.LegacyOpenAIAPIKey = nil
	profile.LegacyOpenAIBaseURL = nil
	profile.LegacyOpenAIModel = nil
	profile.Headers = nil
	return profile
}

func migrateImageAPIEndpointSettings(settings Settings) (Settings, bool) {
	settings, legacyMigrated := migrateLegacyImageSettings(settings)
	migrated := legacyMigrated || hasEmbeddedImageAPIEndpointSettings(settings)
	endpoints := sanitizeImageAPIEndpoints(settings.ImageAPIEndpoints)
	profiles := make([]ImageAPIProfileSettings, 0, len(settings.ImageAPIProfiles))
	for _, profile := range settings.ImageAPIProfiles {
		id := imageAPIProfileID(profile)
		if id == "" {
			if strings.TrimSpace(profile.EndpointID) == "" && imageProfileHasRouting(profile) {
				candidate := imageEndpointFromProfile(profile, "")
				profile.EndpointID = reusableImageAPIEndpointID(candidate, endpoints)
				if profile.EndpointID == "" {
					profile.EndpointID = uniqueImageAPIEndpointID("image-endpoint", endpoints)
					candidate.ID = profile.EndpointID
					candidate.Name = profile.Name
					endpoints = append(endpoints, candidate)
				}
			}
			profiles = append(profiles, clearImageProfileEndpoint(profile))
			continue
		}
		endpointID := strings.TrimSpace(profile.EndpointID)
		if endpointID == "" {
			candidate := imageEndpointFromProfile(profile, "")
			if id == DefaultImageAPIProfileID {
				candidate = mergeImageAPIEndpoint(DefaultImageAPIEndpoint(), candidate)
				candidate.ID = DefaultImageAPIProfileID
			}
			endpointID = reusableImageAPIEndpointID(candidate, endpoints)
			if endpointID == "" {
				base := id
				if base != DefaultImageAPIProfileID {
					base += "-endpoint"
				}
				endpointID = uniqueImageAPIEndpointID(base, endpoints)
				candidate.ID = endpointID
				candidate.Name = firstNonEmpty(candidate.Name, profile.Name)
				endpoints = append(endpoints, candidate)
			}
		}
		profile.ID = id
		profile.EndpointID = endpointID
		profiles = append(profiles, clearImageProfileEndpoint(profile))
	}
	settings.ImageAPIEndpoints = sanitizeImageAPIEndpoints(endpoints)
	settings.ImageAPIProfiles = profiles
	return settings, migrated
}

func imageProfileHasRouting(profile ImageAPIProfileSettings) bool {
	return profile.Provider != "" || profile.Protocol != "" || profile.APIKey != "" ||
		profile.BaseURL != "" || profile.Headers != nil
}

func hasEmbeddedImageAPIEndpointSettings(settings Settings) bool {
	for _, profile := range settings.ImageAPIProfiles {
		if profile.Provider != "" || profile.Protocol != "" || profile.APIKey != "" ||
			profile.BaseURL != "" || profile.Headers != nil {
			return true
		}
		if strings.TrimSpace(profile.EndpointID) == "" &&
			(strings.TrimSpace(profile.ID) != "" || strings.TrimSpace(profile.Model) != "" || profile.ComfyUI != nil) {
			return true
		}
	}
	return false
}

func reusableImageAPIEndpointID(candidate ImageAPIEndpointSettings, endpoints []ImageAPIEndpointSettings) string {
	candidateRoute := imageAPIEndpointRouteSignature(candidate)
	for _, endpoint := range endpoints {
		if imageAPIEndpointRouteSignature(endpoint) != candidateRoute {
			continue
		}
		if candidate.APIKey == "" || endpoint.APIKey == "" || candidate.APIKey == endpoint.APIKey {
			return imageAPIEndpointID(endpoint)
		}
	}
	return ""
}

func imageAPIEndpointRouteSignature(endpoint ImageAPIEndpointSettings) string {
	stable := struct {
		Provider string
		Protocol string
		BaseURL  string
		Headers  map[string]string
	}{
		Provider: normalizeImageAPIProvider(endpoint.Provider), Protocol: normalizeImageAPIProtocol(endpoint.Protocol),
		BaseURL: strings.TrimRight(strings.TrimSpace(endpoint.BaseURL), "/"), Headers: endpoint.Headers,
	}
	encoded, _ := json.Marshal(stable)
	return string(encoded)
}

func uniqueImageAPIEndpointID(base string, endpoints []ImageAPIEndpointSettings) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "image-endpoint"
	}
	used := make(map[string]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		used[imageAPIEndpointID(endpoint)] = struct{}{}
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
