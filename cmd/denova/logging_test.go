package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneLogDirectoryBoundsManagedFiles(t *testing.T) {
	dir := t.TempDir()
	currentPath := filepath.Join(dir, logFileName)
	writeTestLog(t, currentPath, "current", time.Now())
	writeTestLog(t, filepath.Join(dir, "notes.txt"), "keep", time.Now().Add(-time.Hour))
	for index := range 4 {
		writeTestLog(
			t,
			filepath.Join(dir, time.Now().AddDate(0, 0, -index).Format("2006-01-02")+".log"),
			"1234",
			time.Now().Add(-time.Duration(index)*time.Hour),
		)
	}

	if err := pruneLogDirectory(dir, 8, 2, currentPath); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var logCount int
	for _, entry := range entries {
		if isLogFile(entry.Name()) {
			logCount++
		}
	}
	if logCount != 3 { // The current file plus two retained historical logs.
		t.Fatalf("log file count = %d, want 3", logCount)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Fatalf("unrelated file was removed: %v", err)
	}
}

func TestSetupLoggingWritesToBoundedCurrentFile(t *testing.T) {
	dir := t.TempDir()
	path, output, closeLog := setupLogging(dir)
	if path != filepath.Join(dir, logFileName) {
		t.Fatalf("log path = %q", path)
	}
	if _, err := output.Write([]byte("entry\n")); err != nil {
		t.Fatal(err)
	}
	closeLog()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "entry\n" {
		t.Fatalf("log contents = %q", contents)
	}
}

func writeTestLog(t *testing.T, path, contents string, modTime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}
