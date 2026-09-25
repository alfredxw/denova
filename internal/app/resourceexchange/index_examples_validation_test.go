package resourceexchange

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

	"denova/internal/platform"
	"denova/internal/project"
)

func TestIndexExamplesValidation(t *testing.T) {
	dir := os.Getenv("DENOVA_INDEX_EXAMPLES_DIR")
	if dir == "" {
		t.Skip("Set DENOVA_INDEX_EXAMPLES_DIR to validate the independent index checkout")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			ctx := context.Background()
			s := testService(t)
			workspace := filepath.Join(s.root, "projects", "sample")
			if err := os.MkdirAll(workspace, 0700); err != nil {
				t.Fatal(err)
			}
			book, err := s.registry.Add(workspace, project.TypeBook, "Samples")
			if err != nil {
				t.Fatal(err)
			}
			files, err := readFiles(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := archiveBytes(files)
			if err != nil {
				t.Fatal(err)
			}
			preview, err := s.Preview(ctx, Source{Kind: "file", Filename: entry.Name() + ".zip"}, raw)
			if err != nil {
				t.Fatal(err)
			}
			candidate := preview.Candidates[0]
			selected := []string{}
			grants := map[string][]string{}
			for _, resource := range candidate.Resources {
				selected = append(selected, resource.ID)
				if resource.Extension != nil {
					grants[resource.ID] = resource.Extension.Manifest.Permissions.Required
				}
			}
			plan, err := s.Plan(ctx, PlanRequest{PreviewID: preview.ID, CandidateID: candidate.ID, Resources: selected, ProjectID: book.ID, Grants: grants})
			if err != nil {
				t.Fatal(err)
			}
			installed, err := s.Apply(ctx, plan.ID)
			if err != nil {
				t.Fatal(err)
			}
			refs := []LocalRef{}
			for _, binding := range installed.Bindings {
				refs = append(refs, binding.Local)
			}
			exported, err := s.Export(ctx, ExportRequest{Package: installed.Package, Resources: refs})
			if err != nil {
				t.Fatal(err)
			}
			next := testService(t)
			if _, err := next.Preview(ctx, Source{Kind: "file", Filename: "roundtrip.zip"}, exported); err != nil {
				t.Fatal(err)
			}
			if entry.Name() == "text-statistics" {
				runtime, err := s.platform.ActivatePlugin(ctx, platform.ActivatePlugin{PluginID: "index.text-statistics", ReleaseID: candidate.Resources[0].Extension.Digest, Scope: platform.Scope{Kind: "project", ProjectID: book.ID}, OpenOptions: platform.OpenOptions{ParentOrigin: "http://127.0.0.1:15173"}})
				if err != nil {
					t.Fatal(err)
				}
				defer s.platform.Stop(ctx, runtime.ID)
				request, err := http.NewRequest("POST", runtime.Connection.BaseURL+"/tools/index.text-statistics/statistics/invoke", bytes.NewBufferString(`{"input":{"text":"Hello world.\n\nGood night!"}}`))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Authorization", "Bearer "+runtime.Connection.Token)
				request.Header.Set("Content-Type", "application/json")
				client := &http.Client{Timeout: 3 * time.Second}
				response, err := client.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != 200 {
					t.Fatalf("tool: %d %s", response.StatusCode, body)
				}
				var result struct {
					Data map[string]int `json:"data"`
				}
				if err := json.Unmarshal(body, &result); err != nil {
					t.Fatal(err)
				}
				if result.Data["wordSegments"] != 4 || result.Data["paragraphs"] != 2 {
					t.Fatalf("Unexpected statistics: %s", body)
				}
			}
			t.Logf("Verified preview, installation and export for %d resources", len(installed.Bindings))
		})
	}
}
