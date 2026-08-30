package config

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestResolveComfyUIConnectionDraftBeforeWorkflowSelection(t *testing.T) {
	cfg := &Config{ImageAPIProfiles: []ImageAPIProfileSettings{{
		ID: "comfy", Provider: ImageProviderComfyUI, BaseURL: "http://127.0.0.1:8188",
	}}}
	draft := ImageAPIProfileSettings{
		ID: "comfy", Provider: ImageProviderComfyUI,
	}
	resolved, err := ResolveImageAPIProfileConnectionDraft(cfg, draft)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Protocol != ImageProtocolComfyUI || resolved.BaseURL != "http://127.0.0.1:8188" ||
		resolved.ComfyUI.WorkflowMode != ComfyUIWorkflowRemote {
		t.Fatalf("connection draft = %#v", resolved)
	}
	if _, err := ResolveImageAPIProfileDraft(cfg, draft); !errors.Is(err, ErrComfyUIWorkflowMissing) {
		t.Fatalf("generation draft error = %v, want %v", err, ErrComfyUIWorkflowMissing)
	}
}

func TestResolveImageProfilesReuseOneEndpoint(t *testing.T) {
	cfg := &Config{
		ImageAPIEndpoints: []ImageAPIEndpointSettings{{
			ID: "shared", Provider: ImageProviderOpenAI, Protocol: ImageProtocolOpenAI,
			APIKey: "shared-key", BaseURL: "https://images.example/v1",
		}},
		ImageAPIProfiles: []ImageAPIProfileSettings{
			{ID: "cover", EndpointID: "shared", Model: "cover-model"},
			{ID: "illustration", EndpointID: "shared", Model: "illustration-model"},
		},
	}

	cover, err := ResolveImageAPIProfile(cfg, "cover")
	if err != nil {
		t.Fatal(err)
	}
	illustration, err := ResolveImageAPIProfile(cfg, "illustration")
	if err != nil {
		t.Fatal(err)
	}
	for name, resolved := range map[string]ResolvedImageAPIProfile{"cover": cover, "illustration": illustration} {
		if resolved.APIKey != "shared-key" || resolved.BaseURL != "https://images.example/v1" {
			t.Fatalf("%s route = %#v", name, resolved)
		}
	}
	if cover.Model != "cover-model" || illustration.Model != "illustration-model" {
		t.Fatalf("resolved models cover=%#v illustration=%#v", cover, illustration)
	}
}

func TestResolveImageProfileDraftUsesEditedEndpoint(t *testing.T) {
	cfg := &Config{
		ImageAPIEndpoints: []ImageAPIEndpointSettings{{ID: "shared", Provider: ImageProviderOpenAI, Protocol: ImageProtocolOpenAI, BaseURL: "https://old.example/v1", APIKey: "old-key"}},
		ImageAPIProfiles:  []ImageAPIProfileSettings{{ID: "cover", EndpointID: "shared", Model: "cover-model"}},
	}
	resolved, err := ResolveImageAPIProfileEndpointDraft(cfg,
		ImageAPIEndpointSettings{ID: "shared", Provider: ImageProviderOpenAI, Protocol: ImageProtocolOpenAI, BaseURL: "https://new.example/v1", APIKey: "new-key"},
		ImageAPIProfileSettings{ID: "cover", EndpointID: "shared", Model: "cover-model"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.BaseURL != "https://new.example/v1" || resolved.APIKey != "new-key" {
		t.Fatalf("resolved draft = %#v", resolved)
	}
}

func TestResolveDiscoveredComfyUIWorkflowPreservesBindings(t *testing.T) {
	cfg := &Config{
		DefaultImageAPIProfileID: "comfy",
		ImageAPIProfiles: []ImageAPIProfileSettings{{
			ID: "comfy", Provider: ImageProviderComfyUI,
			ComfyUI: &ComfyUIProfileSettings{
				WorkflowMode: ComfyUIWorkflowRemote, Workflow: `{"1":{"class_type":"Example","inputs":{"value":1}}}`,
				WorkflowName: "Portrait", WorkflowID: "workflow-id", WorkflowPath: "folder/portrait.json",
				WorkflowModified: 100, WorkflowJobID: "job-id", WorkflowJobTime: 110,
				Bindings: &ComfyUIBindings{
					Prompt: &ComfyUIInputBinding{NodeID: " 1 ", InputName: " text "},
					Width:  &ComfyUIInputBinding{NodeID: "1", InputName: "value"},
				},
			},
		}},
	}
	resolved, err := ResolveImageAPIProfile(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ComfyUI.WorkflowMode != ComfyUIWorkflowRemote || resolved.ComfyUI.WorkflowID != "workflow-id" || resolved.ComfyUI.Bindings == nil {
		t.Fatalf("resolved ComfyUI settings = %#v", resolved.ComfyUI)
	}
	if got := resolved.ComfyUI.Bindings.Prompt; got == nil || got.NodeID != "1" || got.InputName != "text" {
		t.Fatalf("normalized prompt binding = %#v", got)
	}
}

func TestDiscoveredComfyUIWorkflowSettingsRoundTrip(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "config.toml")
	wantWorkflow := `{"1":{"class_type":"Example","inputs":{"seed":9007199254740993}}}`
	settings := Settings{ImageAPIProfiles: []ImageAPIProfileSettings{{
		ID: "comfy", Provider: ImageProviderComfyUI,
		ComfyUI: &ComfyUIProfileSettings{
			WorkflowMode: ComfyUIWorkflowRemote, Workflow: wantWorkflow,
			WorkflowName: "Saved", WorkflowID: "workflow-id", WorkflowPath: "folder/saved.json",
			Bindings: &ComfyUIBindings{
				Prompt: &ComfyUIInputBinding{NodeID: "2", InputName: "text"},
				Count:  &ComfyUIInputBinding{NodeID: "4", InputName: "batch_size"},
			},
		},
	}}}
	if err := WriteSettingsFile(settingsPath, settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadSettingsFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.ImageAPIProfiles) != 1 || loaded.ImageAPIProfiles[0].ComfyUI == nil {
		t.Fatalf("loaded image profiles = %#v", loaded.ImageAPIProfiles)
	}
	comfy := loaded.ImageAPIProfiles[0].ComfyUI
	if comfy.Workflow != wantWorkflow || comfy.WorkflowID != "workflow-id" || comfy.Bindings == nil {
		t.Fatalf("loaded ComfyUI settings = %#v", comfy)
	}
	if comfy.Bindings.Prompt == nil || comfy.Bindings.Prompt.NodeID != "2" || comfy.Bindings.Count == nil || comfy.Bindings.Count.InputName != "batch_size" {
		t.Fatalf("loaded ComfyUI bindings = %#v", comfy.Bindings)
	}
}
