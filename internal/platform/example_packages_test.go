package platform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCompleteExamplePackages(t *testing.T) {
	m, projectID := testManager(t)
	for directory, kind := range map[string]Kind{"http-tool": Plugin, "galgame": Game} {
		t.Run(directory, func(t *testing.T) {
			root := t.TempDir()
			if err := os.CopyFS(root, os.DirFS(filepath.Join("templates", directory))); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"client.mjs", "runtime.mjs"} {
				data, err := os.ReadFile(filepath.Join("templates", "common", name))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			candidate, err := m.PreviewDirectory(kind, root)
			if err != nil {
				t.Fatal(err)
			}
			release := testInstall(t, m, candidate)
			if kind == Plugin {
				runtime, err := m.ActivatePlugin(context.Background(), ActivatePlugin{PluginID: release.Manifest.ID, ReleaseID: release.Ref.ReleaseID, Scope: Scope{Kind: "session", ProjectID: projectID, SessionID: "writing"}, OpenOptions: OpenOptions{ParentOrigin: "http://127.0.0.1:15173"}})
				if err != nil {
					t.Fatal(err)
				}
				status, data := testRequest(t, runtime.Connection, "POST", "/tools/example.text-tools/count-characters/invoke", "", map[string]any{"input": map[string]string{"text": "A🌷中"}})
				var result ToolResult
				if status != 200 || json.Unmarshal(data, &result) != nil || result.Content != "Character count: 3" {
					t.Fatalf("Example backend failed: %d %s", status, data)
				}
				status, data = testRequest(t, runtime.Connection, "POST", "/tools/example.text-tools/save-note/invoke", "", map[string]any{"input": map[string]string{"requestId": "note-1", "text": "A letter"}})
				if status != 403 {
					t.Fatalf("Example write bypassed optional permission: %d %s", status, data)
				}
			}
		})
	}
}
