package skills

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestRestrictedSkillInstallClientRejectsNonPublicDestinations(t *testing.T) {
	client := newSkillInstallHTTPClient()
	for _, rawURL := range []string{
		"https://127.0.0.1/archive.zip",
		"https://[::1]/archive.zip",
		"https://169.254.169.254/latest/meta-data/",
		"https://10.0.0.1/archive.zip",
	} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Do(req); err == nil || !strings.Contains(err.Error(), "non-public") {
			t.Fatalf("client.Do(%q) error = %v, want non-public destination rejection", rawURL, err)
		}
	}
}

func TestSkillInstallRedirectPolicyRevalidatesEveryHop(t *testing.T) {
	for _, rawURL := range []string{
		"http://example.com/archive.zip",
		"file:///tmp/archive.zip",
	} {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		if err := skillInstallRedirectPolicy(&http.Request{URL: parsed}, nil); err == nil {
			t.Fatalf("redirect to %q should be rejected", rawURL)
		}
	}
	parsed, _ := url.Parse("https://example.com/archive.zip")
	via := make([]*http.Request, 10)
	if err := skillInstallRedirectPolicy(&http.Request{URL: parsed}, via); err == nil {
		t.Fatal("redirect chain longer than the client limit should be rejected")
	}
}
