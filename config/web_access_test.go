package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWebSearchSettingsUpgradeRoundTripAndRemoval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	// The previous release has no explicit service selection or hosted keys.
	if err := os.WriteFile(path, []byte("[web_access]\nsearxng_base_url = 'https://search.example.org'\nfetch_max_response_kb = 1234\nfetch_max_content_chars = 5678\n"), 0600); err != nil {
		t.Fatal(err)
	}
	old, err := ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	resolved := ResolveWebAccessSettings(Merge(DefaultSettings(), old).WebAccess)
	if resolved.SearchProvider != "" || resolved.SearXNGBaseURL != "https://search.example.org" {
		t.Fatalf("legacy selection changed: %+v", resolved)
	}
	next, err := ApplySettingsMergePatch(old, json.RawMessage(`{"web_access":{"search_provider":"deepseek","search_fallback":"none","deepseek":{"api_key":" ds "},"perplexity":{"api_key":"px","mode":"fast"},"tavily":{}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteSettingsFile(path, next); err != nil {
		t.Fatal(err)
	}
	restored, err := ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	runtime := ResolveWebAccessSettings(Merge(DefaultSettings(), restored).WebAccess)
	if runtime.SearchProvider != "deepseek" || runtime.SearchFallback != "none" || runtime.DeepSeek.APIKey != "ds" || runtime.Perplexity.Mode != "fast" || runtime.Tavily == nil || runtime.FetchMaxResponseKB != 1234 || runtime.FetchMaxContentChars != 5678 {
		t.Fatalf("round trip lost fields: %+v", runtime)
	}
	if !reflect.DeepEqual(ResolveWebAccessConfig(runtime), runtime) {
		t.Fatal("runtime normalization lost service configuration")
	}
	cleared, err := ApplySettingsMergePatch(restored, json.RawMessage(`{"web_access":{"deepseek":{"api_key":""},"perplexity":null}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteSettingsFile(path, cleared); err != nil {
		t.Fatal(err)
	}
	cleared, err = ReadSettingsFile(path)
	if err != nil || cleared.WebAccess.Perplexity != nil || cleared.WebAccess.DeepSeek.APIKey != "" {
		t.Fatal("removed credentials survived persistence")
	}
	for _, patch := range []string{`{"web_access":{"search_provider":"unknown"}}`, `{"web_access":{"search_fallback":"tavily"}}`, `{"web_access":{"tavily":{"mode":"web"}}}`} {
		if _, err := ApplySettingsMergePatch(restored, json.RawMessage(patch)); err == nil {
			t.Fatalf("invalid routing accepted: %s", patch)
		}
	}
	keenable, err := ApplySettingsMergePatch(restored, json.RawMessage(`{"web_access":{"search_provider":"keenable","keenable":{}}}`))
	if err != nil {
		t.Fatalf("keyless Keenable rejected: %v", err)
	}
	if runtime := ResolveWebAccessSettings(Merge(DefaultSettings(), keenable).WebAccess); runtime.SearchProvider != "keenable" {
		t.Fatalf("keenable selection lost: %+v", runtime)
	}
	if _, err := ApplySettingsMergePatch(restored, json.RawMessage(`{"web_access":{"keenable":{"mode":"advanced"}}}`)); err == nil {
		t.Fatal("keenable accepted a search mode")
	}
	if err := ValidateWorkspaceSettingsPatch(json.RawMessage(`{"web_access":{"search_provider":"deepseek"}}`)); err == nil {
		t.Fatal("workspace accepted global search configuration")
	}
}
