package structural

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// CommandID derives a deterministic caller-owned idempotency key for a manual
// structural command from its semantic scope.
func CommandID(prefix string, parts ...string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(strings.TrimSpace(prefix)))
	for _, part := range parts {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(strings.TrimSpace(part)))
	}
	return strings.TrimSpace(prefix) + "-" + hex.EncodeToString(hash.Sum(nil))
}
