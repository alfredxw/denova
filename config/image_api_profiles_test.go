package config

import "testing"

func TestResolveImageAPIProfileUsesProviderDefaultsAndRequiresKey(t *testing.T) {
	_, err := ResolveImageAPIProfile(&Config{}, "")
	if err != ErrImageAPIKeyMissing {
		t.Fatalf("missing key error = %v, want %v", err, ErrImageAPIKeyMissing)
	}

	resolved, err := ResolveImageAPIProfile(&Config{ImageAPIProfiles: []ImageAPIProfileSettings{{
		ID: DefaultImageAPIProfileID, APIKey: "key",
	}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ProfileID != DefaultImageAPIProfileID || resolved.Provider != ImageProviderOpenAI || resolved.Protocol != ImageProtocolOpenAI {
		t.Fatalf("profile identity mismatch: %#v", resolved)
	}
	if resolved.BaseURL != DefaultImageAPIBaseURL || resolved.Model != DefaultImageAPIModel {
		t.Fatalf("defaults not applied: %#v", resolved)
	}
}

func TestResolveImageAPIProfileSelectsConfiguredProvider(t *testing.T) {
	cfg := &Config{
		DefaultImageAPIProfileID: "cover",
		ImageAPIProfiles: []ImageAPIProfileSettings{{
			ID: "cover", Name: "Cover", Provider: ImageProviderXAI, APIKey: "profile-key",
			DefaultAspectRatio: "16:9", DefaultResolution: "2k", DefaultQuality: "low",
		}},
	}
	resolved, err := ResolveImageAPIProfile(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ProfileID != "cover" || resolved.APIKey != "profile-key" || resolved.Model != "grok-imagine-image-2.0" {
		t.Fatalf("resolved profile mismatch: %#v", resolved)
	}
	if resolved.Protocol != ImageProtocolXAI || resolved.AspectRatio != "16:9" || resolved.Resolution != "2k" || resolved.Quality != "low" {
		t.Fatalf("resolved defaults mismatch: %#v", resolved)
	}
}

func TestResolveComfyUIBuiltInAndUploadedWorkflows(t *testing.T) {
	builtin, err := ResolveImageAPIProfile(&Config{ImageAPIProfiles: []ImageAPIProfileSettings{{
		ID: "local", Provider: ImageProviderComfyUI, Model: "sdxl.safetensors",
	}}}, "local")
	if err != nil {
		t.Fatal(err)
	}
	if builtin.APIKey != "" || builtin.Protocol != ImageProtocolComfyUI || builtin.ComfyUI.WorkflowMode != ComfyUIWorkflowBuiltin {
		t.Fatalf("built-in ComfyUI profile mismatch: %#v", builtin)
	}

	uploaded, err := ResolveImageAPIProfile(&Config{ImageAPIProfiles: []ImageAPIProfileSettings{{
		ID: "workflow", Provider: ImageProviderComfyUI,
		ComfyUI: &ComfyUIProfileSettings{WorkflowMode: ComfyUIWorkflowAPI, Workflow: `{}`},
	}}}, "workflow")
	if err != nil {
		t.Fatal(err)
	}
	if uploaded.Model != "" || uploaded.ComfyUI.WorkflowMode != ComfyUIWorkflowAPI {
		t.Fatalf("uploaded ComfyUI profile mismatch: %#v", uploaded)
	}
}

func TestMergeImageAPIProfilesByID(t *testing.T) {
	parent := Settings{ImageAPIProfiles: []ImageAPIProfileSettings{{
		ID: "cover", Provider: ImageProviderCustom, Protocol: ImageProtocolOpenAI,
		BaseURL: "https://parent.test/v1", Model: "image-v1", APIKey: "secret",
	}}}
	child := Settings{
		DefaultImageAPIProfileID: "cover",
		ImageAPIProfiles: []ImageAPIProfileSettings{{
			ID: "cover", Model: "image-v2", DefaultQuality: "medium",
		}},
	}
	out := Merge(parent, child)
	if out.DefaultImageAPIProfileID != "cover" || len(out.ImageAPIProfiles) != 1 {
		t.Fatalf("profile merge mismatch: %#v", out)
	}
	got := out.ImageAPIProfiles[0]
	if got.BaseURL != "https://parent.test/v1" || got.Model != "image-v2" || got.APIKey != "secret" || got.DefaultQuality != "medium" {
		t.Fatalf("merged profile mismatch: %#v", got)
	}
}

func TestImageProviderChangeDropsCredentialsFromPreviousOrigin(t *testing.T) {
	merged := mergeImageAPIProfile(ImageAPIProfileSettings{
		ID: "image", Provider: ImageProviderOpenAI, APIKey: "secret", BaseURL: DefaultImageAPIBaseURL,
		DefaultSize: "1024x1024", DefaultQuality: "high", DefaultOutputFormat: "png",
	}, ImageAPIProfileSettings{ID: "image", Provider: ImageProviderGoogle})
	if merged.APIKey != "" || merged.BaseURL != "" || merged.Protocol != "" ||
		merged.DefaultSize != "" || merged.DefaultQuality != "" || merged.DefaultOutputFormat != "" {
		t.Fatalf("provider switch retained scoped state: %#v", merged)
	}
}

func TestImageProtocolChangeDropsProtocolSpecificDefaults(t *testing.T) {
	merged := mergeImageAPIProfile(ImageAPIProfileSettings{
		ID: "image", Provider: ImageProviderCustom, Protocol: ImageProtocolOpenAI,
		BaseURL: "https://images.example.test/v1", Model: "old-model",
		DefaultSize: "1024x1024", DefaultQuality: "high", DefaultOutputFormat: "png",
		ComfyUI: &ComfyUIProfileSettings{WorkflowMode: ComfyUIWorkflowAPI, Workflow: `{}`},
	}, ImageAPIProfileSettings{
		ID: "image", Provider: ImageProviderCustom, Protocol: ImageProtocolXAI,
		BaseURL: "https://images.example.test/v1", DefaultResolution: "2k",
	})
	if merged.Model != "" || merged.ComfyUI != nil || merged.DefaultSize != "" || merged.DefaultQuality != "" || merged.DefaultOutputFormat != "" {
		t.Fatalf("protocol switch retained incompatible state: %#v", merged)
	}
	if merged.DefaultResolution != "2k" {
		t.Fatalf("protocol switch lost explicit child defaults: %#v", merged)
	}
}

func TestResolveImageAPIProfileRejectsUnknownProvider(t *testing.T) {
	_, err := ResolveImageAPIProfile(&Config{ImageAPIProfiles: []ImageAPIProfileSettings{{
		ID: "unknown", Provider: "not-a-provider", BaseURL: "https://example.test", Model: "image",
	}}}, "unknown")
	if err == nil {
		t.Fatal("unknown provider should fail")
	}
}

func TestApplyImageAPIEnvironmentOverridesDefaultProfile(t *testing.T) {
	t.Setenv("DENOVA_IMAGE_PROVIDER", ImageProviderXAI)
	t.Setenv("DENOVA_IMAGE_API_KEY", "environment-key")
	t.Setenv("DENOVA_IMAGE_MODEL", "grok-custom")
	cfg := &Config{ImageAPIProfiles: []ImageAPIProfileSettings{{
		ID: DefaultImageAPIProfileID, Provider: ImageProviderOpenAI, APIKey: "stored-key",
	}}}
	ApplyImageAPIEnvironment(cfg)
	resolved, err := ResolveImageAPIProfile(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Provider != ImageProviderXAI || resolved.Protocol != ImageProtocolXAI || resolved.APIKey != "environment-key" || resolved.Model != "grok-custom" {
		t.Fatalf("environment override mismatch: %#v", resolved)
	}
}

func TestSanitizeImageAPIProfiles(t *testing.T) {
	settings := sanitizeEditableSettings(Settings{ImageAPIProfiles: []ImageAPIProfileSettings{
		{Model: " image-v1 ", Provider: " CUSTOM ", Protocol: ImageProtocolOpenAI, DefaultSize: " 1024x1024 ", DefaultQuality: "HIGH", DefaultOutputFormat: "jpg"},
		{ID: "  "},
	}})
	if len(settings.ImageAPIProfiles) != 1 {
		t.Fatalf("sanitized image profiles length = %d", len(settings.ImageAPIProfiles))
	}
	profile := settings.ImageAPIProfiles[0]
	if profile.ID != "image-v1" || profile.Model != "image-v1" || profile.Provider != ImageProviderCustom {
		t.Fatalf("profile identity not normalized: %#v", profile)
	}
	if profile.DefaultSize != "1024x1024" || profile.DefaultQuality != "high" || profile.DefaultOutputFormat != "jpeg" {
		t.Fatalf("profile defaults not normalized: %#v", profile)
	}
}
