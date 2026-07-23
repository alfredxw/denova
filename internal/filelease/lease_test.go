package filelease

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestAcquireSerializesProcessWaitersAndHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resource.lock")
	release, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Acquire(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waiter error = %v, want %v", err, context.Canceled)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatalf("idempotent release: %v", err)
	}
	reopened, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened(); err != nil {
		t.Fatal(err)
	}
}
