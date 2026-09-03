package config

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/alfredxw/denova/agent/providers"
)

const DefaultModelEndpointID = "default"

func modelEndpointID(endpoint ModelEndpointSettings) string {
	return strings.TrimSpace(endpoint.ID)
}

func mergeModelEndpoints(parent, child []ModelEndpointSettings) []ModelEndpointSettings {
	if len(child) == 0 {
		return parent
	}
	out := make([]ModelEndpointSettings, 0, len(parent)+len(child))
	index := make(map[string]int, len(parent)+len(child))
	for _, endpoint := range parent {
		id := modelEndpointID(endpoint)
		if id == "" {
			continue
		}
		endpoint.ID = id
		index[id] = len(out)
		out = append(out, endpoint)
	}
	for _, endpoint := range child {
		id := modelEndpointID(endpoint)
		if id == "" {
			continue
		}
		endpoint.ID = id
		if current, ok := index[id]; ok {
			out[current] = mergeModelEndpoint(out[current], endpoint)
			continue
		}
		index[id] = len(out)
		out = append(out, endpoint)
	}
	return out
}

func mergeModelEndpoint(parent, child ModelEndpointSettings) ModelEndpointSettings {
	merged := mergeModelProfile(modelProfileFromEndpoint(parent), modelProfileFromEndpoint(child))
	return modelEndpointFromProfile(merged, firstNonEmpty(modelEndpointID(child), modelEndpointID(parent)))
}

func sanitizeModelEndpoints(endpoints []ModelEndpointSettings) []ModelEndpointSettings {
	out := make([]ModelEndpointSettings, 0, len(endpoints))
	seen := make(map[string]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		id := modelEndpointID(endpoint)
		if id == "" {
			continue
		}
		profile := normalizeModelProfileRouting(modelProfileFromEndpoint(endpoint))
		endpoint = modelEndpointFromProfile(profile, id)
		endpoint.Name = strings.TrimSpace(endpoint.Name)
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, endpoint)
	}
	return out
}

func modelProfileFromEndpoint(endpoint ModelEndpointSettings) ModelProfileSettings {
	return ModelProfileSettings{
		ID: endpoint.ID, Name: endpoint.Name, Provider: endpoint.Provider,
		Protocol: endpoint.Protocol, APIKey: endpoint.APIKey, BaseURL: endpoint.BaseURL,
		Headers:           cloneModelProfileHeaders(endpoint.Headers),
		ProtocolOptions:   cloneModelProfileOptions(endpoint.ProtocolOptions),
		SessionKeyMapping: cloneModelProfileSessionKeyMapping(endpoint.SessionKeyMapping),
	}
}

func modelEndpointFromProfile(profile ModelProfileSettings, id string) ModelEndpointSettings {
	return ModelEndpointSettings{
		ID: id, Name: strings.TrimSpace(profile.Name), Provider: strings.TrimSpace(profile.Provider),
		Protocol: strings.TrimSpace(profile.Protocol), APIKey: profile.APIKey,
		BaseURL: strings.TrimSpace(profile.BaseURL), Headers: cloneModelProfileHeaders(profile.Headers),
		ProtocolOptions:   cloneModelProfileOptions(profile.ProtocolOptions),
		SessionKeyMapping: cloneModelProfileSessionKeyMapping(profile.SessionKeyMapping),
	}
}

func modelProfileWithEndpoint(cfg *Config, profile ModelProfileSettings) ModelProfileSettings {
	endpointID := strings.TrimSpace(profile.EndpointID)
	if endpointID == "" {
		return profile
	}
	endpoint, ok := findModelEndpoint(cfg, endpointID)
	if !ok && endpointID == DefaultModelEndpointID {
		endpoint = legacyModelEndpoint(cfg)
		ok = true
	}
	if !ok {
		profile.Provider = "__missing_endpoint__"
		return profile
	}
	connection := modelProfileFromEndpoint(endpoint)
	connection.ID = profile.ID
	connection.Name = profile.Name
	connection.EndpointID = endpointID
	connection.Model = profile.Model
	connection.Temperature = profile.Temperature
	connection.ContextWindowTokens = profile.ContextWindowTokens
	connection.MaxTokens = profile.MaxTokens
	return connection
}

func findModelEndpoint(cfg *Config, id string) (ModelEndpointSettings, bool) {
	if cfg == nil {
		return ModelEndpointSettings{}, false
	}
	for _, endpoint := range cfg.ModelEndpoints {
		if modelEndpointID(endpoint) == id {
			return endpoint, true
		}
	}
	return ModelEndpointSettings{}, false
}

func legacyModelEndpoint(cfg *Config) ModelEndpointSettings {
	if cfg == nil {
		return ModelEndpointSettings{ID: DefaultModelEndpointID}
	}
	profile := legacyModelProfile(cfg)
	return modelEndpointFromProfile(profile, DefaultModelEndpointID)
}

func clearModelProfileEndpoint(profile ModelProfileSettings) ModelProfileSettings {
	profile.Provider = ""
	profile.Protocol = ""
	profile.APIKey = ""
	profile.BaseURL = ""
	profile.LegacyOpenAIAPIKey = ""
	profile.LegacyOpenAIBaseURL = ""
	profile.LegacyOpenAIModel = ""
	profile.Headers = nil
	profile.ProtocolOptions = nil
	profile.SessionKeyMapping = nil
	return profile
}

func migrateModelEndpointSettings(settings Settings) (Settings, bool) {
	migrated := hasEmbeddedModelEndpointSettings(settings)
	hasLegacyTopLevel := hasLegacyTopLevelModelSettings(settings)
	endpoints := sanitizeModelEndpoints(settings.ModelEndpoints)
	profiles := make([]ModelProfileSettings, 0, len(settings.ModelProfiles)+1)

	legacyDefault := migrateLegacyModelProfile(ModelProfileSettings{
		ID: DefaultModelEndpointID, Name: "Default model", APIKey: settings.OpenAIAPIKey,
		BaseURL: settings.OpenAIBaseURL, Model: settings.OpenAIModel,
		ContextWindowTokens: settings.OpenAIContextWindowTokens,
	})
	if hasLegacyTopLevel {
		endpoint := modelEndpointFromProfile(legacyDefault, DefaultModelEndpointID)
		endpoints = upsertMigratedModelEndpoint(endpoints, endpoint)
	}

	hasDefaultProfile := false
	for _, raw := range settings.ModelProfiles {
		profile := migrateLegacyModelProfile(raw)
		// Released profiles inherited the top-level OpenAI connection before the
		// endpoint split. Materialize that effective connection before grouping so
		// migration preserves behavior even when a profile overrides only its URL.
		// Generic embedded routing was never released and must not inherit secrets.
		if hasLegacyTopLevel && strings.TrimSpace(raw.EndpointID) == "" && !modelProfileHasRouting(raw) {
			if profile.APIKey == "" {
				profile.APIKey = legacyDefault.APIKey
			}
			if strings.TrimSpace(profile.BaseURL) == "" {
				profile.BaseURL = legacyDefault.BaseURL
			}
		}
		id := modelProfileID(profile)
		if id == "" {
			if strings.TrimSpace(profile.EndpointID) == "" && modelProfileHasRouting(profile) {
				candidate := modelEndpointFromProfile(profile, "")
				profile.EndpointID = reusableModelEndpointID(candidate, endpoints)
				if profile.EndpointID == "" {
					profile.EndpointID = uniqueModelEndpointID("endpoint", endpoints)
					candidate.ID = profile.EndpointID
					candidate.Name = profile.Name
					endpoints = append(endpoints, candidate)
				}
			}
			profiles = append(profiles, clearModelProfileEndpoint(profile))
			continue
		}
		if id == DefaultModelEndpointID {
			hasDefaultProfile = true
		}
		endpointID := strings.TrimSpace(profile.EndpointID)
		if endpointID == "" {
			candidate := modelEndpointFromProfile(profile, "")
			if id == DefaultModelEndpointID {
				candidate = mergeModelEndpoint(modelEndpointFromProfile(legacyDefault, DefaultModelEndpointID), candidate)
				candidate.ID = DefaultModelEndpointID
			}
			endpointID = reusableModelEndpointID(candidate, endpoints)
			if endpointID == "" {
				base := id
				if base != DefaultModelEndpointID {
					base += "-endpoint"
				}
				endpointID = uniqueModelEndpointID(base, endpoints)
				candidate.ID = endpointID
				candidate.Name = firstNonEmpty(candidate.Name, profile.Name)
				endpoints = append(endpoints, candidate)
			}
		}
		profile.EndpointID = endpointID
		profile.ID = id
		profiles = append(profiles, clearModelProfileEndpoint(profile))
	}
	if hasLegacyTopLevel && !hasDefaultProfile {
		legacyDefault.EndpointID = DefaultModelEndpointID
		profiles = append([]ModelProfileSettings{clearModelProfileEndpoint(legacyDefault)}, profiles...)
	}

	settings.OpenAIAPIKey = ""
	settings.OpenAIBaseURL = ""
	settings.OpenAIModel = ""
	settings.OpenAIContextWindowTokens = nil
	settings.ModelEndpoints = sanitizeModelEndpoints(endpoints)
	settings.ModelProfiles = profiles
	return settings, migrated
}

func modelProfileHasRouting(profile ModelProfileSettings) bool {
	return profile.Provider != "" || profile.Protocol != "" || profile.APIKey != "" ||
		profile.BaseURL != "" || profile.Headers != nil || profile.ProtocolOptions != nil ||
		profile.SessionKeyMapping != nil
}

func hasLegacyTopLevelModelSettings(settings Settings) bool {
	return settings.OpenAIAPIKey != "" || strings.TrimSpace(settings.OpenAIBaseURL) != "" ||
		strings.TrimSpace(settings.OpenAIModel) != "" || settings.OpenAIContextWindowTokens != nil
}

func hasEmbeddedModelEndpointSettings(settings Settings) bool {
	if hasLegacyTopLevelModelSettings(settings) {
		return true
	}
	for _, profile := range settings.ModelProfiles {
		if profile.Provider != "" || profile.Protocol != "" || profile.APIKey != "" ||
			profile.BaseURL != "" || profile.LegacyOpenAIAPIKey != "" ||
			profile.LegacyOpenAIBaseURL != "" || profile.LegacyOpenAIModel != "" ||
			profile.Headers != nil || profile.ProtocolOptions != nil || profile.SessionKeyMapping != nil {
			return true
		}
		if strings.TrimSpace(profile.EndpointID) == "" &&
			(strings.TrimSpace(profile.ID) != "" || strings.TrimSpace(profile.Model) != "") {
			return true
		}
	}
	return false
}

func upsertMigratedModelEndpoint(endpoints []ModelEndpointSettings, endpoint ModelEndpointSettings) []ModelEndpointSettings {
	for index, current := range endpoints {
		if modelEndpointID(current) == modelEndpointID(endpoint) {
			endpoints[index] = mergeModelEndpoint(current, endpoint)
			return endpoints
		}
	}
	return append(endpoints, endpoint)
}

func reusableModelEndpointID(candidate ModelEndpointSettings, endpoints []ModelEndpointSettings) string {
	candidateRoute := modelEndpointRouteSignature(candidate)
	for _, endpoint := range endpoints {
		if modelEndpointRouteSignature(endpoint) != candidateRoute {
			continue
		}
		if candidate.APIKey == endpoint.APIKey {
			return modelEndpointID(endpoint)
		}
	}
	return ""
}

func modelEndpointRouteSignature(endpoint ModelEndpointSettings) string {
	profile := normalizeModelProfileRouting(modelProfileFromEndpoint(endpoint))
	stable := struct {
		Provider          string
		Protocol          string
		BaseURL           string
		Headers           map[string]string
		ProtocolOptions   map[string]any
		SessionKeyMapping *providers.SessionKeyMapping
	}{
		Provider: strings.TrimSpace(profile.Provider), Protocol: strings.TrimSpace(profile.Protocol),
		BaseURL: strings.TrimRight(strings.TrimSpace(profile.BaseURL), "/"),
		Headers: profile.Headers, ProtocolOptions: profile.ProtocolOptions,
		SessionKeyMapping: profile.SessionKeyMapping,
	}
	encoded, _ := json.Marshal(stable)
	return string(encoded)
}

func uniqueModelEndpointID(base string, endpoints []ModelEndpointSettings) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "endpoint"
	}
	used := make(map[string]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		used[modelEndpointID(endpoint)] = struct{}{}
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
