package localfs

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireLeaseSerializesWaitersAndHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resource.lock")
	release, err := AcquireLease(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := AcquireLease(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled waiter error = %v, want %v", err, context.DeadlineExceeded)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatalf("idempotent release: %v", err)
	}
	reopened, err := AcquireLease(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened(); err != nil {
		t.Fatal(err)
	}
}
