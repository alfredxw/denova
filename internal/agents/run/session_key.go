package agentrun

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"denova/config"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

const sessionKeyPrefix = "denova-"

// SessionKeyForAgentSession derives an opaque, stable provider cache-routing
// key from one public Agent Session identity. Raw workspace and product
// identifiers are never sent to a model provider.
func SessionKeyForAgentSession(key agent.SessionKey) (string, error) {
	canonical, err := agentsession.CanonicalKey(key)
	if err != nil {
		return "", fmt.Errorf("derive provider cache key: %w", err)
	}
	return deriveSessionKey("agent-session", canonical), nil
}

// StandaloneSessionKey derives a stable cache-routing key for model-backed
// jobs that do not run inside a durable conversation execution.
func StandaloneSessionKey(cfg *config.Config, agentKind, source string) string {
	projectScope := ""
	if cfg != nil {
		projectScope = strings.TrimSpace(cfg.ProjectID)
		if projectScope == "" {
			projectScope = strings.TrimSpace(cfg.Workspace)
		}
	}
	return deriveSessionKey("standalone", projectScope, strings.TrimSpace(agentKind), strings.TrimSpace(source))
}

func deriveSessionKey(parts ...string) string {
	payload := make([]byte, 0, 256)
	for _, part := range parts {
		payload = binary.BigEndian.AppendUint32(payload, uint32(len(part)))
		payload = append(payload, part...)
	}
	digest := sha256.Sum256(payload)
	return sessionKeyPrefix + hex.EncodeToString(digest[:16])
}
