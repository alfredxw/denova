package agent

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateIdempotencyKeyExposesProviderNeutralAdmissionBoundary(t *testing.T) {
	t.Parallel()

	if err := ValidateIdempotencyKey(" caller-owned-retry "); err != nil {
		t.Fatalf("valid idempotency key: %v", err)
	}
	for _, key := range []string{"", " \t\n", strings.Repeat("x", 4<<10+1)} {
		if err := ValidateIdempotencyKey(key); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("ValidateIdempotencyKey(%d bytes) error = %v, want ErrInvalidInput", len(key), err)
		}
	}
}
