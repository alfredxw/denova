//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"testing"
	"time"
)

const (
	fileJournalHelperEnv    = "DENOVA_FILE_JOURNAL_LOCK_HELPER"
	fileJournalRootEnv      = "DENOVA_FILE_JOURNAL_LOCK_ROOT"
	fileJournalReadyPathEnv = "DENOVA_FILE_JOURNAL_LOCK_READY"
	fileJournalLockTestKey  = "cross-process-lock"
)

func TestFileJournalLockSerializesAnotherProcess(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileJournalStore(root)
	if err != nil {
		t.Fatalf("new file journal store: %v", err)
	}
	opened, err := store.OpenJournal(context.Background(), fileJournalLockTestKey)
	if err != nil {
		t.Fatalf("open file journal: %v", err)
	}
	journal := opened.(*fileJournal)
	released := false
	defer func() {
		if !released {
			_ = journal.Close()
		}
	}()

	readyPath := filepath.Join(root, "child-ready")
	command := exec.Command(os.Args[0], "-test.run=^TestFileJournalLockHelperProcess$", "-test.count=1")
	command.Env = append(os.Environ(),
		fileJournalHelperEnv+"=1",
		fileJournalRootEnv+"="+root,
		fileJournalReadyPathEnv+"="+readyPath,
	)
	if err := command.Start(); err != nil {
		t.Fatalf("start lock helper: %v", err)
	}
	finished := false
	waited := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				waited <- fmt.Errorf("wait for lock helper panicked: %v\n%s", recovered, debug.Stack())
			}
		}()
		waited <- command.Wait()
	}()
	defer func() {
		if !finished {
			_ = command.Process.Kill()
			<-waited
		}
	}()

	waitForJournalHelperReady(t, readyPath)
	select {
	case err := <-waited:
		finished = true
		t.Fatalf("helper bypassed held file lock: %v", err)
	case <-time.After(40 * time.Millisecond):
	}
	if err := journal.Close(); err != nil {
		t.Fatalf("release parent binding lease: %v", err)
	}
	released = true
	select {
	case err := <-waited:
		finished = true
		if err != nil {
			t.Fatalf("helper failed after lock release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("helper did not finish after lock release")
	}

	reopened, err := store.OpenJournal(context.Background(), fileJournalLockTestKey)
	if err != nil {
		t.Fatalf("reopen file journal: %v", err)
	}
	defer reopened.Close()
	events, err := reopened.Load(context.Background())
	if err != nil {
		t.Fatalf("load helper append: %v", err)
	}
	if len(events) != 1 || events[0].Cursor != 1 {
		t.Fatalf("helper events = %#v, want one committed event", events)
	}
}

func TestFileJournalLockHelperProcess(t *testing.T) {
	if os.Getenv(fileJournalHelperEnv) != "1" {
		t.Skip("subprocess helper")
	}
	store, err := NewFileJournalStore(os.Getenv(fileJournalRootEnv))
	if err != nil {
		t.Fatalf("new helper store: %v", err)
	}
	if err := os.WriteFile(os.Getenv(fileJournalReadyPathEnv), []byte("ready"), 0o600); err != nil {
		t.Fatalf("signal helper ready: %v", err)
	}
	opened, err := store.OpenJournal(context.Background(), fileJournalLockTestKey)
	if err != nil {
		t.Fatalf("open helper journal: %v", err)
	}
	defer opened.Close()
	journal := opened.(*fileJournal)
	appendTestJournalEvent(t, journal, 0, "child")
}

func waitForJournalHelperReady(t *testing.T, path string) {
	t.Helper()
	deadline := time.NewTimer(500 * time.Millisecond)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat helper readiness: %v", err)
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("lock helper did not become ready")
		}
	}
}
