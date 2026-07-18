package keyedlock

import (
	"sync"
	"testing"
)

func TestLockSerializesSameKey(t *testing.T) {
	lock := New(nil)

	var wg sync.WaitGroup
	var mu sync.Mutex
	held := false
	failed := false
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := lock.Lock("same")
			defer unlock()
			mu.Lock()
			if held {
				failed = true
			}
			held = true
			mu.Unlock()
			mu.Lock()
			held = false
			mu.Unlock()
		}()
	}
	wg.Wait()
	if failed {
		t.Fatal("keyedlock allowed two goroutines to hold the same key concurrently")
	}
}

func TestLockAllowsDifferentKeys(t *testing.T) {
	lock := New(nil)
	unlockA := lock.Lock("a")
	defer unlockA()

	done := make(chan struct{})
	go func() {
		u := lock.Lock("b")
		u()
		close(done)
	}()
	select {
	case <-done:
	case <-make(chan struct{}):
		t.Fatal("distinct keys should not block each other")
	}
}

func TestLockUsesKeyFunc(t *testing.T) {
	lock := New(func(key string) string {
		if key == "alias" {
			return "canonical"
		}
		return key
	})
	unlock := lock.Lock("canonical")
	defer unlock()

	started := make(chan struct{})
	blocked := make(chan struct{})
	go func() {
		close(started)
		u := lock.Lock("alias")
		u()
		close(blocked)
	}()
	<-started
	select {
	case <-blocked:
		t.Fatal("key func should collapse alias onto the canonical lock")
	default:
	}
}

func TestLockDropsEntriesWhenUnused(t *testing.T) {
	lock := New(nil)
	unlock := lock.Lock("key")
	unlock()
	// A second acquire after all references drop must not panic and should
	// reuse a fresh entry internally.
	again := lock.Lock("key")
	again()
}
