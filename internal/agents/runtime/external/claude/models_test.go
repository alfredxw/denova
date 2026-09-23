package claude

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"denova/internal/agents/runtime/external"
	"denova/internal/hostruntime"
)

func TestModelDiscoveryPreservesRuntimeChoices(t *testing.T) {
	got, err := decodeModels(json.RawMessage(`{"models":[{"value":"account-default","displayName":"Recommended","supportedEffortLevels":["medium","new-effort"]},{"value":"new-model[large]","displayName":"New model"}]}`))
	want := external.Models{DefaultID: "account-default", Items: []external.Model{
		{ID: "account-default", DisplayName: "Recommended", Efforts: []string{"medium", "new-effort"}},
		{ID: "new-model[large]", DisplayName: "New model", Efforts: []string{}},
	}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("models=%+v err=%v", got, err)
	}
	if _, err := decodeModels(json.RawMessage(`{"models":[]}`)); err == nil {
		t.Fatal("empty discovery must not fabricate a static catalog")
	}
}

func TestInstalledClaudeModelDiscovery(t *testing.T) {
	executable := os.Getenv("DENOVA_TEST_CLAUDE_EXE")
	if executable == "" {
		t.Skip("set DENOVA_TEST_CLAUDE_EXE to validate the installed runtime")
	}
	client, err := Connect(t.Context(), ProcessOptions{Launch: hostruntime.ClaudeLaunch{Executable: executable}})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	models, err := client.Models(t.Context())
	if err != nil || len(models.Items) == 0 || models.DefaultID != models.Items[0].ID {
		t.Fatalf("models=%+v err=%v", models, err)
	}
}
