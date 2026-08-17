package providers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/alfredxw/denova/agent"
)

// ModelIdentity returns the stable, credential-free identity used by durable
// Agent Sessions. It describes request behavior, not a concrete client or
// secret. Endpoint credentials in URL userinfo, query parameters, and known
// authentication headers are deliberately excluded.
func ModelIdentity(config ModelConfig) (agent.CapabilityIdentity, error) {
	clone, err := config.Clone()
	if err != nil {
		return agent.CapabilityIdentity{}, err
	}
	baseURL, err := identityBaseURL(clone.BaseURL)
	if err != nil {
		return agent.CapabilityIdentity{}, err
	}
	payload := struct {
		Provider          ProviderID
		Protocol          ProtocolID
		Model             string
		BaseURL           string
		Headers           map[string]string
		ProtocolOptions   json.RawMessage
		SessionKeyMapping *SessionKeyMapping
		Temperature       *float32
		MaxOutputTokens   *int
		ThinkingLevel     ThinkingLevel
		OutputFormat      *OutputFormat
	}{
		Provider:          clone.Provider,
		Protocol:          clone.Protocol,
		Model:             clone.Model,
		BaseURL:           baseURL,
		Headers:           identityHeaders(clone.Headers),
		ProtocolOptions:   clone.ProtocolOptions,
		SessionKeyMapping: clone.SessionKeyMapping,
		Temperature:       clone.Temperature,
		MaxOutputTokens:   clone.MaxOutputTokens,
		ThinkingLevel:     clone.ThinkingLevel,
		OutputFormat:      clone.OutputFormat,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return agent.CapabilityIdentity{}, fmt.Errorf("marshal model capability identity: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return agent.CapabilityIdentity{
		Kind:       "model.provider",
		Version:    1,
		ConfigHash: hex.EncodeToString(digest[:]),
	}, nil
}

func identityBaseURL(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse model base URL for identity: %w", err)
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String(), nil
}

func identityHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	result := make(map[string]string, len(headers))
	for name, value := range headers {
		canonicalName := http.CanonicalHeaderKey(strings.TrimSpace(name))
		if canonicalName == "" {
			continue
		}
		if identitySecretHeader(canonicalName) {
			result[canonicalName] = "<credential>"
			continue
		}
		result[canonicalName] = value
	}
	return result
}

func identitySecretHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "proxy-authorization", "cookie", "set-cookie",
		"api-key", "x-api-key", "x-auth-token", "x-access-token":
		return true
	default:
		return false
	}
}
