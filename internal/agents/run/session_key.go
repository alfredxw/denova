package agentrun

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"strings"

	"denova/config"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

const sessionKeyPrefix = "denova-"

// SessionKeyForBinding derives an opaque, stable provider cache-routing key
// from one durable runtime identity. Raw workspace and product identifiers are
// never sent to a model provider.
func SessionKeyForBinding(binding runstate.BindingRef) string {
	parts := []string{"runtime", binding.Kind, binding.Profile, binding.Key}
	labelNames := make([]string, 0, len(binding.Labels))
	for name := range binding.Labels {
		labelNames = append(labelNames, name)
	}
	sort.Strings(labelNames)
	for _, name := range labelNames {
		parts = append(parts, name, binding.Labels[name])
	}
	return deriveSessionKey(parts...)
}

// StandaloneSessionKey derives a stable cache-routing key for model-backed
// jobs that do not run inside a durable conversation harness.
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
