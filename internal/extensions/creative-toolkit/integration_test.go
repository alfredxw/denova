package creativetoolkit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"denova/extensionassets"
	"denova/internal/platform"
	"denova/internal/project"
)

// The extension is tested as a consumer of the public host API, without
// importing platform test helpers or exposing production internals.
func TestCreativeToolkitInstallationAndPermissions(t *testing.T) {
	root := t.TempDir()
	registry := project.NewRegistry(root)
	workspace := filepath.Join(root, "projects", "Author")
	if err := os.MkdirAll(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := registry.Add(workspace, project.TypeGeneral, "Author")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.EnsureStore(record); err != nil {
		t.Fatal(err)
	}
	manager := platform.New(root, registry)
	t.Cleanup(func() {
		if err := manager.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})

	source := t.TempDir()
	if err := os.CopyFS(source, os.DirFS("package")); err != nil {
		t.Fatal(err)
	}
	runtimeSource, err := extensionassets.Files().ReadFile("sdk/runtime.mjs")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "runtime.mjs"), runtimeSource, 0600); err != nil {
		t.Fatal(err)
	}
	candidate, err := manager.PreviewDirectory(platform.Plugin, source)
	if err != nil {
		t.Fatal(err)
	}
	release, err := manager.Install(candidate.ID, candidate.Manifest.Permissions.Required)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := manager.ActivatePlugin(context.Background(), platform.ActivatePlugin{
		PluginID: release.Manifest.ID, ReleaseID: release.Ref.ReleaseID,
		Scope:       platform.Scope{Kind: "session", ProjectID: record.ID, SessionID: "writing"},
		OpenOptions: platform.OpenOptions{ParentOrigin: "http://127.0.0.1:15173"},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	for _, check := range []struct {
		tool    string
		input   map[string]string
		status  int
		content string
	}{
		{tool: "count-characters", input: map[string]string{"text": "A🌷中"}, status: http.StatusOK, content: "Character count: 3"},
		{tool: "save-note", input: map[string]string{"requestId": "note-1", "text": "A letter"}, status: http.StatusForbidden},
	} {
		t.Run(check.tool, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"input": check.input})
			if err != nil {
				t.Fatal(err)
			}
			request, err := http.NewRequest(http.MethodPost, runtime.Connection.BaseURL+"/tools/"+release.Manifest.ID+"/"+check.tool+"/invoke", bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer "+runtime.Connection.Token)
			request.Header.Set("Content-Type", "application/json")
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			data, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != check.status {
				t.Fatalf("tool returned %d, want %d: %s", response.StatusCode, check.status, data)
			}
			if check.status == http.StatusOK {
				var result platform.ToolResult
				if err := json.Unmarshal(data, &result); err != nil || result.Content != check.content {
					t.Fatalf("unexpected tool result: %s (%v)", data, err)
				}
			}
		})
	}
}
