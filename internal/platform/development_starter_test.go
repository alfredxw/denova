package platform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateDevelopmentWithoutTemplate(t *testing.T) {
	for _, kind := range []Kind{Plugin, Game} {
		t.Run(string(kind), func(t *testing.T) {
			m, projectID := testManager(t)
			development, err := m.CreateDevelopment(CreateDevelopment{
				Kind: kind, ProjectID: projectID, RelativePath: ".",
				ID: "test.starter", Name: LocalizedText{Chinese: "开发项目", English: "Development project"},
			})
			if err != nil {
				t.Fatal(err)
			}
			candidate, err := m.CheckDevelopment(development.ID)
			if err != nil {
				t.Fatal(err)
			}
			if candidate.Manifest.ID != "test.starter" || len(candidate.Manifest.ModelSlots) != 0 || candidate.Manifest.Settings != nil || candidate.Manifest.Definitions != nil {
				t.Fatalf("starter includes product-specific configuration: %#v", candidate.Manifest)
			}
			_, directory, err := m.Development(development.ID)
			if err != nil {
				t.Fatal(err)
			}
			if guide, err := os.ReadFile(filepath.Join(directory, "DEVELOPMENT.md")); err != nil || len(guide) == 0 {
				t.Fatalf("missing development guide: %v", err)
			}
			release := testInstall(t, m, candidate)
			if kind == Plugin {
				runtime, err := m.ActivatePlugin(context.Background(), ActivatePlugin{PluginID: release.Manifest.ID, ReleaseID: release.Ref.ReleaseID, Scope: Scope{Kind: "project", ProjectID: projectID}, OpenOptions: OpenOptions{ParentOrigin: "http://127.0.0.1:15173"}})
				if err != nil {
					t.Fatal(err)
				}
				status, data := testRequest(t, runtime.Connection, "POST", "/tools/test.starter/echo/invoke", "", map[string]any{"input": map[string]string{"text": "Starter 🧩"}})
				var result ToolResult
				if status != 200 || json.Unmarshal(data, &result) != nil || result.Content != "Starter 🧩" {
					t.Fatalf("starter tool failed: %d %s", status, data)
				}
			}
		})
	}
}
