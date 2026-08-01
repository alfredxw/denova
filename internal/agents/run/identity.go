package agentrun

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// NewID returns an opaque process-local identity with a semantic prefix. The
// timestamp aids diagnostics; random entropy prevents collisions across
// concurrently created runs and commands.
func NewID(prefix string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), "-")
	if prefix == "" {
		prefix = "id"
	}
	var entropy [6]byte
	if _, err := rand.Read(entropy[:]); err == nil {
		return prefix + "-" + time.Now().UTC().Format("20060102T150405.000000000") + "-" + hex.EncodeToString(entropy[:])
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}
