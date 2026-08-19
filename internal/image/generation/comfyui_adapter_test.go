package generation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"denova/config"
)

func TestPrepareUploadedComfyUIWorkflowInjectsPromptAndOptions(t *testing.T) {
	raw := `{
		"3":{"class_type":"KSamplerAdvanced","inputs":{"seed":1,"positive":["6",0]}},
		"5":{"class_type":"EmptyLatentImage","inputs":{"width":512,"height":512,"batch_size":1}},
		"6":{"class_type":"CLIPTextEncode","inputs":{"text":"old"}}
	}`
	workflow, err := prepareUploadedComfyUIWorkflow(raw, GenerateRequest{Prompt: "new prompt", N: 3, Size: "1280x720"})
	if err != nil {
		t.Fatal(err)
	}
	promptInputs := workflow["6"].(map[string]any)["inputs"].(map[string]any)
	if promptInputs["text"] != "new prompt" {
		t.Fatalf("prompt was not injected: %#v", promptInputs)
	}
	latentInputs := workflow["5"].(map[string]any)["inputs"].(map[string]any)
	if latentInputs["width"] != 1280 || latentInputs["height"] != 720 || latentInputs["batch_size"] != 3 {
		t.Fatalf("latent options were not injected: %#v", latentInputs)
	}
	samplerInputs := workflow["3"].(map[string]any)["inputs"].(map[string]any)
	if samplerInputs["seed"] == float64(1) {
		t.Fatalf("sampler seed was not refreshed: %#v", samplerInputs)
	}
}

func TestPrepareUploadedComfyUIWorkflowRejectsUIFormat(t *testing.T) {
	_, err := prepareUploadedComfyUIWorkflow(`{"last_node_id":9,"nodes":[]}`, GenerateRequest{Prompt: "prompt", N: 1})
	if err == nil {
		t.Fatal("UI-format workflow should not be accepted as API format")
	}
}

func TestComfyUIAdapterRunsWorkflowAndDownloadsOutput(t *testing.T) {
	var submitted comfyUIPromptRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/prompt":
			if err := json.NewDecoder(request.Body).Decode(&submitted); err != nil {
				t.Errorf("decode prompt request: %v", err)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"prompt_id":"prompt-1","node_errors":{}}`))
		case "/history/prompt-1":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"prompt-1":{"outputs":{"9":{"images":[{"filename":"result.png","subfolder":"","type":"output"}]}},"status":{"status_str":"success","completed":true}}}`))
		case "/view":
			if request.URL.Query().Get("filename") != "result.png" || request.URL.Query().Get("type") != "output" {
				t.Errorf("unexpected view query: %s", request.URL.RawQuery)
			}
			writer.Header().Set("Content-Type", "image/png")
			_, _ = writer.Write([]byte("\x89PNG\r\n\x1a\n"))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	result, err := NewComfyUIAdapter(server.Client()).Generate(context.Background(), config.ResolvedImageAPIProfile{
		ProfileID: "local", Provider: config.ImageProviderComfyUI, Protocol: config.ImageProtocolComfyUI,
		BaseURL: server.URL, Model: "sdxl.safetensors", ComfyUI: config.ComfyUIProfileSettings{WorkflowMode: config.ComfyUIWorkflowBuiltin},
	}, GenerateRequest{Prompt: "test prompt", N: 1, Size: "1024x1024"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Images) != 1 || result.Images[0].Extension != "png" || result.Images[0].MIMEType != "image/png" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if submitted.ClientID == "" || submitted.Prompt["6"].(map[string]any)["inputs"].(map[string]any)["text"] != "test prompt" {
		t.Fatalf("unexpected submitted workflow: %#v", submitted)
	}
}
