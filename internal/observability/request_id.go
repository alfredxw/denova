package observability

import (
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

var fallbackRequestIDSequence atomic.Uint64

// NewRequestID returns a time-sortable UUIDv7. A process-scoped fallback ID is
// still returned if the operating-system random source fails, allowing the
// failure itself to remain traceable; callers should log the non-nil error.
func NewRequestID() (string, error) {
	id, err := uuid.NewV7()
	if err == nil {
		return id.String(), nil
	}
	fallback := "fallback-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 36) +
		"-" + strconv.Itoa(os.Getpid()) +
		"-" + strconv.FormatUint(fallbackRequestIDSequence.Add(1), 36)
	return fallback, fmt.Errorf("generate UUIDv7 request ID: %w", err)
}
