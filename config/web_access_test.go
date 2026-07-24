package config

import "testing"

func TestDefaultWebAccessSettingsUseHighBoundedContentLimit(t *testing.T) {
	resolved := ResolveWebAccessSettings(DefaultWebAccessSettings())
	if resolved.SearchMaxResults != DefaultWebSearchMaxResults {
		t.Fatalf("search max results = %d", resolved.SearchMaxResults)
	}
	if resolved.SearchProviderTimeoutSeconds != DefaultWebSearchProviderTimeoutSeconds {
		t.Fatalf("search provider timeout = %d seconds", resolved.SearchProviderTimeoutSeconds)
	}
	if resolved.FetchMaxResponseKB != DefaultWebFetchMaxResponseKB {
		t.Fatalf("fetch response limit = %d KB", resolved.FetchMaxResponseKB)
	}
	if resolved.FetchMaxContentChars != DefaultWebFetchMaxContentChars || resolved.FetchMaxContentChars <= 128*1024 {
		t.Fatalf("fetch content limit = %d, want an explicit limit above 128K", resolved.FetchMaxContentChars)
	}
}

func TestMergeAndResolveWebAccessSettings(t *testing.T) {
	parent := WebAccessSettings{
		SearXNGBaseURL:               "https://search.example.com/",
		SearchMaxResults:             intPtr(8),
		SearchProviderTimeoutSeconds: intPtr(30),
		FetchMaxResponseKB:           intPtr(4096),
		FetchMaxContentChars:         intPtr(200000),
	}
	child := WebAccessSettings{SearchMaxResults: intPtr(15), SearchProviderTimeoutSeconds: intPtr(0)}
	resolved := ResolveWebAccessSettings(MergeWebAccessSettings(parent, child))
	if resolved.SearXNGBaseURL != "https://search.example.com" || resolved.SearchMaxResults != 15 || resolved.SearchProviderTimeoutSeconds != 0 || resolved.FetchMaxResponseKB != 4096 || resolved.FetchMaxContentChars != 200000 {
		t.Fatalf("unexpected merged web access settings: %+v", resolved)
	}
}

func TestResolveWebAccessSettingsBoundsUnsafeLimits(t *testing.T) {
	settings := WebAccessSettings{
		SearchMaxResults:     intPtr(999),
		FetchMaxResponseKB:   intPtr(999999),
		FetchMaxContentChars: intPtr(999999),
	}
	resolved := ResolveWebAccessSettings(settings)
	if resolved.SearchMaxResults != MaxWebSearchMaxResults || resolved.FetchMaxResponseKB != MaxWebFetchMaxResponseKB || resolved.FetchMaxContentChars != MaxWebFetchMaxContentChars {
		t.Fatalf("web access limits were not bounded: %+v", resolved)
	}
}

func TestWorkspaceSettingsCannotOverrideWebAccess(t *testing.T) {
	workspace := workspaceAgentSettings(Settings{WebAccess: WebAccessSettings{SearXNGBaseURL: "http://workspace.invalid"}})
	if workspace.WebAccess.SearXNGBaseURL != "" {
		t.Fatalf("workspace web access setting escaped user-only scope: %+v", workspace.WebAccess)
	}
}
