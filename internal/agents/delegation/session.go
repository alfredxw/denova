package delegation

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

const (
	ParentSessionAttribute = agent.ParentSessionAttribute
	ChildAgentAttribute    = "agent"
	routeChunksAttribute   = "parent_route_chunks"
	routeDigestAttribute   = "parent_route_sha256"
	routeChunkPrefix       = "parent_route_"
	routeChunkBytes        = 48 << 10
)

// ParentAttributes freezes the exact public parent Session identity in a
// child key. The encoded canonical key remains privacy-safe provider-side:
// Denova's cache-key adapter hashes the public identity before model use.
func ParentAttributes(parent agent.SessionKey) (map[string]string, error) {
	return agent.ChildSessionAttributes(parent)
}

// WithParentRoute freezes the accepted parent product descriptor into child
// Session identity. Values are chunked below the public Session attribute
// bound; the digest makes missing/reordered/corrupted chunks fail closed.
// Nothing here is model-visible and Denova's provider cache adapter hashes the
// canonical child key before sending a cache identity to providers.
func WithParentRoute(attributes map[string]string, route *agent.HostData) (map[string]string, error) {
	if route == nil {
		return nil, errors.New("delegated task parent route is required")
	}
	encoded, err := json.Marshal(route)
	if err != nil {
		return nil, fmt.Errorf("encode delegated task parent route: %w", err)
	}
	if len(encoded) == 0 || !json.Valid(encoded) {
		return nil, errors.New("delegated task parent route is invalid")
	}
	chunks := (len(encoded) + routeChunkBytes - 1) / routeChunkBytes
	if chunks <= 0 || chunks+len(attributes)+2 > agentsession.MaxAttributes {
		return nil, fmt.Errorf("delegated task parent route exceeds Session identity capacity")
	}
	result := make(map[string]string, len(attributes)+chunks+2)
	for name, value := range attributes {
		result[name] = value
	}
	digest := sha256.Sum256(encoded)
	result[routeChunksAttribute] = fmt.Sprintf("%d", chunks)
	result[routeDigestAttribute] = hex.EncodeToString(digest[:])
	for index := 0; index < chunks; index++ {
		start := index * routeChunkBytes
		end := min(start+routeChunkBytes, len(encoded))
		result[fmt.Sprintf("%s%02d", routeChunkPrefix, index)] = base64.RawURLEncoding.EncodeToString(encoded[start:end])
	}
	return result, nil
}

// ParentRoute reconstructs the immutable product descriptor needed to build
// every cycle of a delegated child, including Steer and cold recovery.
func ParentRoute(child agent.SessionKey) (*agent.HostData, error) {
	count, err := parseRouteChunkCount(child.Attributes[routeChunksAttribute])
	if err != nil {
		return nil, err
	}
	encoded := make([]byte, 0, count*routeChunkBytes)
	for index := 0; index < count; index++ {
		chunk, decodeErr := base64.RawURLEncoding.DecodeString(child.Attributes[fmt.Sprintf("%s%02d", routeChunkPrefix, index)])
		if decodeErr != nil || len(chunk) == 0 {
			return nil, errors.New("delegated task parent route is incomplete")
		}
		encoded = append(encoded, chunk...)
	}
	digest := sha256.Sum256(encoded)
	if hex.EncodeToString(digest[:]) != child.Attributes[routeDigestAttribute] {
		return nil, errors.New("delegated task parent route digest does not match")
	}
	var route agent.HostData
	if err := json.Unmarshal(encoded, &route); err != nil || strings.TrimSpace(route.Type) == "" || route.Version == 0 || !json.Valid(route.Data) {
		return nil, errors.New("delegated task parent route is invalid")
	}
	return &route, nil
}

func parseRouteChunkCount(value string) (int, error) {
	var count int
	if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &count); err != nil || count <= 0 || count > agentsession.MaxAttributes-4 {
		return 0, errors.New("delegated task parent route chunk count is invalid")
	}
	return count, nil
}

// ParentSession decodes the exact parent key from a delegated child Session.
func ParentSession(child agent.SessionKey) (agent.SessionKey, error) {
	if !strings.HasPrefix(child.Namespace, "task.") {
		return agent.SessionKey{}, errors.New("Session is not a delegated task")
	}
	parent, err := agent.ParentSessionKey(child)
	if err != nil {
		return agent.SessionKey{}, fmt.Errorf("decode delegated parent Session: %w", err)
	}
	return parent, nil
}

func ChildName(child agent.SessionKey) (string, error) {
	name := strings.TrimSpace(child.Attributes[ChildAgentAttribute])
	if name == "" || child.Namespace != "task."+name {
		return "", errors.New("delegated task Agent identity is invalid")
	}
	return name, nil
}
