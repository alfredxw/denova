package agent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

var publicIDFallback atomic.Uint64

// RunIDRequest is the application-owned context available when Agent admits a
// new Run. Agent validates and persists the returned identity but assigns no
// product meaning to its format.
type RunIDRequest struct {
	Session SessionKey
}

// RunIDGenerator lets an embedding application define its Run identity policy.
// Implementations must return a non-empty, stable ID within the configured
// Agent limits; they should not perform remote I/O.
type RunIDGenerator func(RunIDRequest) (string, error)

func newPublicID(prefix string) string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return prefix + "-" + hex.EncodeToString(value[:])
	}
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UTC().UnixNano(), publicIDFallback.Add(1))
}
