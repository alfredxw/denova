package config

import "testing"

func TestResolveImageAPIProfileUsesDefaultsAndRequiresKey(t *testing.T) {
	_, err := ResolveImageAPIProfile(&Config{}, "")
	if err == nil {
		t.Fatalf("missing API key should fail")
	}
	if err != ErrImageAPIKeyMissing {
		t.Fatalf("missing key error = %v, want %v", err, ErrImageAPIKeyMissing)
	}

	resolved, err := ResolveImageAPIProfile(&Config{ImageAPIKey: "key"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ProfileID != DefaultImageAPIProfileID {
		t.Fatalf("profile id = %q", resolved.ProfileID)
	}
	if resolved.OpenAIBaseURL != DefaultImageAPIBaseURL || resolved.OpenAIModel != DefaultImageAPIModel {
		t.Fatalf("defaults not applied: %#v", resolved)
	}
	if resolved.Size != "" {
		t.Fatalf("default size should be unset, got %q", resolved.Size)
	}
}

func TestResolveImageAPIProfileSelectsConfiguredProfile(t *testing.T) {
	cfg := &Config{
		ImageAPIKey:              "root-key",
		DefaultImageAPIProfileID: "cover",
		ImageAPIProfiles: []ImageAPIProfileSettings{{
			ID:                  "cover",
			Name:                "Cover",
			OpenAIAPIKey:        "profile-key",
			OpenAIBaseURL:       "https://example.test/v1",
			OpenAIModel:         "gpt-image-1-mini",
			DefaultSize:         "4096x2304",
			DefaultQuality:      "high",
			DefaultOutputFormat: "jpeg",
		}},
	}
	resolved, err := ResolveImageAPIProfile(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ProfileID != "cover" || resolved.OpenAIAPIKey != "profile-key" || resolved.OpenAIModel != "gpt-image-1-mini" {
		t.Fatalf("resolved profile mismatch: %#v", resolved)
	}
	if resolved.Size != "" || resolved.Quality != "high" || resolved.OutputFormat != "jpeg" {
		t.Fatalf("resolved defaults mismatch: %#v", resolved)
	}
}

func TestMergeImageAPIProfilesByID(t *testing.T) {
	parent := Settings{
		ImageAPIProfiles: []ImageAPIProfileSettings{{
			ID:            "cover",
			OpenAIBaseURL: "https://parent.test/v1",
			OpenAIModel:   "gpt-image-1",
		}},
	}
	child := Settings{
		DefaultImageAPIProfileID: "cover",
		ImageAPIProfiles: []ImageAPIProfileSettings{{
			ID:             "cover",
			OpenAIModel:    "gpt-image-1-mini",
			DefaultQuality: "medium",
		}},
	}
	out := Merge(parent, child)
	if out.DefaultImageAPIProfileID != "cover" {
		t.Fatalf("default profile id not merged")
	}
	if len(out.ImageAPIProfiles) != 1 {
		t.Fatalf("profile count = %d", len(out.ImageAPIProfiles))
	}
	got := out.ImageAPIProfiles[0]
	if got.OpenAIBaseURL != "https://parent.test/v1" || got.OpenAIModel != "gpt-image-1-mini" || got.DefaultQuality != "medium" {
		t.Fatalf("merged profile mismatch: %#v", got)
	}
}

func TestSanitizeImageAPIProfiles(t *testing.T) {
	settings := sanitizeEditableSettings(Settings{
		ImageAPIProfiles: []ImageAPIProfileSettings{
			{OpenAIModel: " gpt-image-1 ", DefaultSize: "bad", DefaultQuality: "high", DefaultOutputFormat: "jpeg"},
			{ID: "  "},
		},
	})
	if len(settings.ImageAPIProfiles) != 1 {
		t.Fatalf("sanitized image profiles length = %d", len(settings.ImageAPIProfiles))
	}
	profile := settings.ImageAPIProfiles[0]
	if profile.ID != "gpt-image-1" || profile.OpenAIModel != "gpt-image-1" {
		t.Fatalf("profile ID/model not normalized: %#v", profile)
	}
	if profile.DefaultSize != "" || profile.DefaultQuality != "high" || profile.DefaultOutputFormat != "jpeg" {
		t.Fatalf("profile defaults not normalized: %#v", profile)
	}
}
