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
		case request.URL.Path == "/object_info":
			writeComfyUITestJSON(t, response, comfyUITestObjectInfo())
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
	roles := make(map[string]string, len(snapshot.Parameters))
	values := make(map[string]string, len(snapshot.Parameters))
	for _, parameter := range snapshot.Parameters {
		key := parameter.NodeID + "." + parameter.InputName
		roles[key] = parameter.Role
		values[key] = parameter.Value
	}
	for key, want := range map[string]string{
		"2.text": config.ComfyUIParameterRolePrompt, "3.text": config.ComfyUIParameterRoleNegativePrompt,
		"4.width": config.ComfyUIParameterRoleWidth, "4.height": config.ComfyUIParameterRoleHeight,
		"4.batch_size": config.ComfyUIParameterRoleBatchSize, "5.seed": config.ComfyUIParameterRoleSeed,
	} {
		if roles[key] != want {
			t.Errorf("parameter %s role = %q, want %q", key, roles[key], want)
		}
	}
	if values["5.seed"] != "9007199254740993" {
		t.Fatalf("large seed value = %q", values["5.seed"])
	}
	if roles["5.cfg"] != config.ComfyUIParameterRoleParameter {
		t.Fatalf("cfg role = %q", roles["5.cfg"])
	}
}

func TestPrepareDiscoveredComfyUIWorkflowAppliesBindings(t *testing.T) {
	raw, err := json.Marshal(comfyUITestWorkflow())
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := analyzeComfyUIParameters(mustDecodeComfyUITestWorkflow(t, raw), decodeComfyUITestObjectInfo(t))
	if err != nil {
		t.Fatal(err)
	}
	for index := range parameters {
		if parameters[index].NodeID == "3" && parameters[index].InputName == "text" {
			parameters[index].Value = `"soft shadows"`
		}
		if parameters[index].NodeID == "5" && parameters[index].InputName == "cfg" {
			parameters[index].Value = "6.25"
		}
	}
	workflow, err := prepareUploadedComfyUIWorkflow(config.ComfyUIProfileSettings{
		WorkflowMode: config.ComfyUIWorkflowRemote,
		Workflow:     string(raw),
		Parameters:   parameters,
	}, GenerateRequest{Prompt: "runtime prompt", N: 3, Size: "768x1024"})
	if err != nil {
		t.Fatal(err)
	}
	wantValues := map[string]string{
		"2.text": `"runtime prompt"`, "3.text": `"soft shadows"`,
		"4.width": "768", "4.height": "1024", "4.batch_size": "3", "5.cfg": "6.25",
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

func comfyUITestObjectInfo() map[string]any {
	return map[string]any{
		"CheckpointLoaderSimple": map[string]any{"input": map[string]any{"required": map[string]any{
			"ckpt_name": []any{[]string{"model.safetensors", "other.safetensors"}},
		}}},
		"CLIPTextEncode": map[string]any{"input": map[string]any{"required": map[string]any{
			"text": []any{"STRING", map[string]any{"multiline": true}},
		}}},
		"EmptyLatentImage": map[string]any{"input": map[string]any{"required": map[string]any{
			"width":      []any{"INT", map[string]any{"min": 64, "max": 4096, "step": 8}},
			"height":     []any{"INT", map[string]any{"min": 64, "max": 4096, "step": 8}},
			"batch_size": []any{"INT", map[string]any{"min": 1, "max": 64}},
		}}},
		"KSampler": map[string]any{"input": map[string]any{"required": map[string]any{
			"seed": []any{"INT"}, "steps": []any{"INT", map[string]any{"min": 1, "max": 100}},
			"cfg":          []any{"FLOAT", map[string]any{"min": 0, "max": 30, "step": 0.1}},
			"sampler_name": []any{[]string{"euler", "dpmpp_2m"}},
		}}},
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

func mustDecodeComfyUITestWorkflow(t *testing.T, raw []byte) comfyUIWorkflow {
	t.Helper()
	workflow, err := decodeComfyUIWorkflowJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	return workflow
}

func decodeComfyUITestObjectInfo(t *testing.T) map[string]comfyUINodeInfo {
	t.Helper()
	raw, err := json.Marshal(comfyUITestObjectInfo())
	if err != nil {
		t.Fatal(err)
	}
	var info map[string]comfyUINodeInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	return info
}
