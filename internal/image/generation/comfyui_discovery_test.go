package generation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"denova/config"
)

func TestComfyUIWorkflowDiscoveryAndLoad(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/userdata" && request.URL.Query().Get("dir") == "workflows":
			writeComfyUITestJSON(t, response, []map[string]any{
				{"path": "ready.json", "modified": 100},
				{"path": "stale.json", "modified": 300},
				{"path": "never-run.json", "modified": 100},
				{"path": "invalid.json", "modified": 100},
			})
		case strings.HasSuffix(request.URL.Path, "/ready.json"):
			writeComfyUITestJSON(t, response, map[string]any{"id": "ready-workflow"})
		case strings.HasSuffix(request.URL.Path, "/stale.json"):
			writeComfyUITestJSON(t, response, map[string]any{"id": "stale-workflow"})
		case strings.HasSuffix(request.URL.Path, "/never-run.json"):
			writeComfyUITestJSON(t, response, map[string]any{"id": "never-run-workflow"})
		case strings.HasSuffix(request.URL.Path, "/invalid.json"):
			writeComfyUITestJSON(t, response, map[string]any{})
		case request.URL.Path == "/api/jobs" && request.URL.Query().Get("workflow_id") == "ready-workflow":
			writeComfyUITestJSON(t, response, comfyUIJobsResponseForTest([]map[string]any{{
				"id": "ready-job", "status": "completed", "create_time": 200, "workflow_id": "ready-workflow",
			}}))
		case request.URL.Path == "/api/jobs" && request.URL.Query().Get("workflow_id") == "stale-workflow":
			writeComfyUITestJSON(t, response, comfyUIJobsResponseForTest([]map[string]any{
				{"id": "stale-job", "status": "completed", "create_time": 200, "workflow_id": "stale-workflow"},
			}))
		case request.URL.Path == "/api/jobs" && request.URL.Query().Get("workflow_id") == "never-run-workflow":
			writeComfyUITestJSON(t, response, comfyUIJobsResponseForTest(nil))
		case request.URL.Path == "/api/jobs/ready-job":
			writeComfyUITestJSON(t, response, map[string]any{"workflow": map[string]any{"prompt": comfyUITestWorkflow()}})
		default:
			http.Error(response, `{"error":"not found"}`, http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	profile := config.ResolvedImageAPIProfile{BaseURL: server.URL}
	adapter := NewComfyUIAdapter(server.Client())
	catalog, err := adapter.DiscoverWorkflows(context.Background(), profile)
	if err != nil {
		t.Fatal(err)
	}
	statuses := make(map[string]string, len(catalog.Workflows))
	for _, workflow := range catalog.Workflows {
		statuses[workflow.Path] = workflow.Status
	}
	wantStatuses := map[string]string{
		"invalid.json": ComfyUIWorkflowStatusInvalid, "never-run.json": ComfyUIWorkflowStatusNotRun,
		"ready.json": ComfyUIWorkflowStatusReady, "stale.json": ComfyUIWorkflowStatusStale,
	}
	if !reflect.DeepEqual(statuses, wantStatuses) {
		t.Fatalf("workflow statuses = %#v, want %#v", statuses, wantStatuses)
	}

	snapshot, err := adapter.LoadWorkflow(context.Background(), profile, "ready.json")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.WorkflowID != "ready-workflow" || snapshot.JobID != "ready-job" || snapshot.Status != ComfyUIWorkflowStatusReady {
		t.Fatalf("workflow snapshot metadata = %#v", snapshot.ComfyUIWorkflowSummary)
	}
	if snapshot.Bindings == nil {
		t.Fatal("workflow bindings are missing")
	}
	for name, binding := range map[string]*config.ComfyUIInputBinding{
		"prompt": snapshot.Bindings.Prompt, "count": snapshot.Bindings.Count,
		"width": snapshot.Bindings.Width, "height": snapshot.Bindings.Height,
	} {
		if binding == nil {
			t.Fatalf("%s binding is missing", name)
		}
	}
	if snapshot.Bindings.Prompt.NodeID != "2" || snapshot.Bindings.Prompt.InputName != "text" ||
		snapshot.Bindings.Count.NodeID != "4" || snapshot.Bindings.Count.InputName != "batch_size" {
		t.Fatalf("workflow bindings = %#v", snapshot.Bindings)
	}
	if len(snapshot.Candidates.Prompt) != 1 || len(snapshot.Candidates.Count) != 1 ||
		len(snapshot.Candidates.Width) != 1 || len(snapshot.Candidates.Height) != 1 {
		t.Fatalf("binding candidates = %#v", snapshot.Candidates)
	}
}

func TestPrepareDiscoveredComfyUIWorkflowAppliesBindings(t *testing.T) {
	raw, err := json.Marshal(comfyUITestWorkflow())
	if err != nil {
		t.Fatal(err)
	}
	bindings, _ := analyzeComfyUIBindings(mustDecodeComfyUITestWorkflow(t, raw))
	workflow, err := prepareUploadedComfyUIWorkflow(config.ComfyUIProfileSettings{
		WorkflowMode: config.ComfyUIWorkflowRemote,
		Workflow:     string(raw),
		Bindings:     bindings,
	}, GenerateRequest{Prompt: "runtime prompt", N: 3, Size: "768x1024"})
	if err != nil {
		t.Fatal(err)
	}
	wantValues := map[string]string{
		"2.text": `"runtime prompt"`, "3.text": `"saved negative"`,
		"4.width": "768", "4.height": "1024", "4.batch_size": "3", "5.cfg": "7",
	}
	for key, want := range wantValues {
		parts := strings.Split(key, ".")
		node := workflow[parts[0]].(map[string]any)
		value := node["inputs"].(map[string]any)[parts[1]]
		encoded, encodeErr := json.Marshal(value)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		if string(encoded) != want {
			t.Errorf("workflow %s = %s, want %s", key, encoded, want)
		}
	}
	seed := workflow["5"].(map[string]any)["inputs"].(map[string]any)["seed"]
	if seed == json.Number("9007199254740993") {
		t.Fatal("runtime seed was not regenerated")
	}
}

func TestAnalyzeComfyUIBindingsLeavesAmbiguousPromptForManualSelection(t *testing.T) {
	workflow := mustDecodeComfyUITestWorkflow(t, mustMarshalComfyUITestJSON(t, comfyUITestWorkflow()))
	workflow["6"] = map[string]any{
		"class_type": "CLIPTextEncode", "inputs": map[string]any{"text": "second positive", "clip": []any{"1", 1}},
	}
	workflow["5"].(map[string]any)["inputs"].(map[string]any)["positive"] = []any{"7", 0}
	workflow["7"] = map[string]any{
		"class_type": "ConditioningCombine", "inputs": map[string]any{"conditioning_1": []any{"2", 0}, "conditioning_2": []any{"6", 0}},
	}

	bindings, candidates := analyzeComfyUIBindings(workflow)
	if bindings == nil || bindings.Prompt != nil {
		t.Fatalf("ambiguous prompt binding = %#v", bindings)
	}
	if len(candidates.Prompt) != 2 {
		t.Fatalf("prompt candidates = %#v", candidates.Prompt)
	}
}

func comfyUITestWorkflow() map[string]any {
	return map[string]any{
		"1": map[string]any{"class_type": "CheckpointLoaderSimple", "inputs": map[string]any{"ckpt_name": "model.safetensors"}},
		"2": map[string]any{"class_type": "CLIPTextEncode", "_meta": map[string]any{"title": "Positive Prompt"}, "inputs": map[string]any{"text": "saved positive", "clip": []any{"1", 1}}},
		"3": map[string]any{"class_type": "CLIPTextEncode", "_meta": map[string]any{"title": "Negative Prompt"}, "inputs": map[string]any{"text": "saved negative", "clip": []any{"1", 1}}},
		"4": map[string]any{"class_type": "EmptyLatentImage", "inputs": map[string]any{"width": 512, "height": 512, "batch_size": 1}},
		"5": map[string]any{"class_type": "KSampler", "inputs": map[string]any{
			"seed": json.Number("9007199254740993"), "steps": 20, "cfg": 7.0, "sampler_name": "euler",
			"positive": []any{"2", 0}, "negative": []any{"3", 0}, "latent_image": []any{"4", 0},
		}},
	}
}

func comfyUIJobsResponseForTest(jobs []map[string]any) map[string]any {
	return map[string]any{"jobs": jobs, "pagination": map[string]any{"has_more": false}}
}

func writeComfyUITestJSON(t *testing.T, response http.ResponseWriter, value any) {
	t.Helper()
	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func mustMarshalComfyUITestJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mustDecodeComfyUITestWorkflow(t *testing.T, raw []byte) comfyUIWorkflow {
	t.Helper()
	workflow, err := decodeComfyUIWorkflowJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	return workflow
}
