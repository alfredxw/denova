package continuallearning

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"denova/config"
)

func TestStateUpdateToolAppliesOneValidatedChangeSet(t *testing.T) {
	service := newStateToolTestService(t)
	ctx := context.Background()
	current, err := service.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := service.StateUpdateTool()
	if err != nil {
		t.Fatal(err)
	}
	arguments, _ := json.Marshal(map[string]any{
		"base_revision": current.Revision,
		"summary":       "Add a reusable response preference",
		"changes": []map[string]any{{
			"path": "prompts/general.md", "content": "Lead with the result.",
		}},
	})
	result, err := definition.Tool.Run(ctx, string(arguments))
	if err != nil || result.IsError() {
		t.Fatalf("update result=%#v err=%v", result, err)
	}
	after, err := service.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision == current.Revision || len(after.Files) != 1 || after.Files[0].Content != "Lead with the result." {
		t.Fatalf("updated State = %#v", after)
	}
}

func TestStateUpdateToolReturnsAllInputDiagnosticsWithoutWriting(t *testing.T) {
	service := newStateToolTestService(t)
	ctx := context.Background()
	current, err := service.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := service.StateUpdateTool()
	if err != nil {
		t.Fatal(err)
	}
	result, err := definition.Tool.Run(ctx, `{
		"base_revision":"",
		"changes":[
			{"path":"../bad","delete":true,"content":"x"},
			{"path":"../bad"}
		]
	}`)
	if err != nil || !result.IsError() {
		t.Fatalf("invalid update result=%#v err=%v", result, err)
	}
	var details struct {
		Diagnostics []struct {
			Code string `json:"code"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(result.Details, &details); err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, diagnostic := range details.Diagnostics {
		codes[diagnostic.Code] = true
	}
	for _, code := range []string{"base_revision_missing", "change_content_with_delete", "change_content_missing", "invalid_change_path"} {
		if !codes[code] {
			t.Fatalf("missing diagnostic %q in %#v", code, details.Diagnostics)
		}
	}
	after, err := service.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != current.Revision {
		t.Fatalf("invalid update changed State: before=%s after=%s", current.Revision, after.Revision)
	}
}

func newStateToolTestService(t *testing.T) *Service {
	t.Helper()
	cfg := config.Config{DenovaDir: filepath.Join(t.TempDir(), ".denova")}
	cfg.Labs.DeveloperMode = true
	return NewService(testHost{runtime: Runtime{Config: cfg}})
}
