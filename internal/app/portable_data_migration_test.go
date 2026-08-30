package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPortableDataMigrationBacksUpReleaseIndexesAndWritesRelativeReceipt(t *testing.T) {
	dataRoot := t.TempDir()
	sources := map[string]string{
		"books.json":                         `{"books":[{"name":"Portable"}]}`,
		"projects.json":                      `{"version":3,"projects":[]}`,
		filepath.Join("book_meta", "a.json"): `{"path":"C:\\old\\book","title":"Portable"}`,
		filepath.Join("project-state", "Portable", "migration.json"): `{"version":3,"source":"C:\\old\\book","copied":[]}`,
	}
	for relative, content := range sources {
		path := filepath.Join(dataRoot, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	backups, err := preparePortableDataMigration(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 4 {
		t.Fatalf("backup count = %d, want 4: %#v", len(backups), backups)
	}
	for _, relative := range backups {
		if filepath.IsAbs(relative) || strings.Contains(relative, `\`) {
			t.Fatalf("backup receipt path is not portable: %q", relative)
		}
		if _, err := os.Stat(filepath.Join(dataRoot, filepath.FromSlash(relative))); err != nil {
			t.Fatalf("backup %q is unavailable: %v", relative, err)
		}
	}
	if err := completePortableDataMigration(dataRoot, backups, 2); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(portableDataMigrationReceiptPath(dataRoot))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), dataRoot) {
		t.Fatalf("migration receipt retained the runtime data root: %s", raw)
	}
	var receipt portableDataMigrationReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Version != portableDataMigrationVersion || receipt.ProjectCount != 2 || len(receipt.Backups) != 4 {
		t.Fatalf("migration receipt = %#v", receipt)
	}
}

func TestPortableDataMigrationRetryIsIdempotent(t *testing.T) {
	dataRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataRoot, "books.json"), []byte(`{"books":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := preparePortableDataMigration(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := preparePortableDataMigration(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(first, "\n") != strings.Join(second, "\n") {
		t.Fatalf("retry backups changed: first=%#v second=%#v", first, second)
	}
	if err := completePortableDataMigration(dataRoot, first, 0); err != nil {
		t.Fatal(err)
	}
	rawBefore, err := os.ReadFile(portableDataMigrationReceiptPath(dataRoot))
	if err != nil {
		t.Fatal(err)
	}
	if err := completePortableDataMigration(dataRoot, nil, 99); err != nil {
		t.Fatal(err)
	}
	rawAfter, err := os.ReadFile(portableDataMigrationReceiptPath(dataRoot))
	if err != nil {
		t.Fatal(err)
	}
	if string(rawBefore) != string(rawAfter) {
		t.Fatal("completed migration receipt was rewritten")
	}
}
