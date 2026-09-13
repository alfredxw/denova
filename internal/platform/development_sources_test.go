package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevelopmentSourcesRetainInvalidDrafts(t *testing.T) {
	m, projectID := testManager(t)
	created := testSource(t, m, projectID, ".", "static", "test.source", Game)
	_, directory, err := m.Development(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	items, err := m.DevelopmentSources()
	if err != nil || len(items) != 1 || items[0].Manifest == nil || items[0].Manifest.ID != "test.source" || items[0].Development != created {
		t.Fatalf("source projection: %#v %v", items, err)
	}
	for _, content := range []string{`{"id":`, strings.Repeat(" ", MaxDefinitionBytes+1)} {
		if err := os.WriteFile(filepath.Join(directory, "denova.game.json"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		items, err := m.DevelopmentSources()
		if err != nil || len(items) != 1 || items[0].Manifest != nil || items[0].MessageKey != "platform.sourceInvalid" || items[0].Development != created {
			t.Fatalf("invalid source disappeared or changed identity: %#v %v", items, err)
		}
	}
}
