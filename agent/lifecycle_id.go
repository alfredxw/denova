package agent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

var publicIDFallback atomic.Uint64

func newPublicID(prefix string) string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return prefix + "-" + hex.EncodeToString(value[:])
	}
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UTC().UnixNano(), publicIDFallback.Add(1))
}
