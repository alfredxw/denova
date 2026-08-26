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
		ComfyUI: &ComfyUIProfileSettings{WorkflowMode: ComfyUIWorkflowRemote},
	}
	resolved, err := ResolveImageAPIProfileConnectionDraft(cfg, draft)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Protocol != ImageProtocolComfyUI || resolved.BaseURL != "http://127.0.0.1:8188" {
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
				Parameters: []ComfyUIParameterSettings{{
					NodeID: "1", InputName: "value", Label: " Example ", Type: "int", Role: ComfyUIParameterRoleWidth,
					Value: "512", Options: []string{"installation-local"},
				}},
			},
		}},
	}
	resolved, err := ResolveImageAPIProfile(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ComfyUI.WorkflowMode != ComfyUIWorkflowRemote || resolved.ComfyUI.WorkflowID != "workflow-id" || len(resolved.ComfyUI.Parameters) != 1 {
		t.Fatalf("resolved ComfyUI settings = %#v", resolved.ComfyUI)
	}
	parameter := resolved.ComfyUI.Parameters[0]
	if parameter.Label != "Example" || parameter.Type != "INT" || parameter.Role != ComfyUIParameterRoleWidth || parameter.Value != "512" {
		t.Fatalf("normalized parameter = %#v", parameter)
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
			Parameters: []ComfyUIParameterSettings{{
				NodeID: "1", InputName: "seed", Type: "INT", Role: ComfyUIParameterRoleSeed,
				Value: "9007199254740993", Options: []string{"not-persisted"},
			}},
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
	if comfy.Workflow != wantWorkflow || comfy.WorkflowID != "workflow-id" || len(comfy.Parameters) != 1 {
		t.Fatalf("loaded ComfyUI settings = %#v", comfy)
	}
	if comfy.Parameters[0].Value != "9007199254740993" || len(comfy.Parameters[0].Options) != 0 {
		t.Fatalf("loaded ComfyUI parameter = %#v", comfy.Parameters[0])
	}
}
