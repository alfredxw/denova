package webaccess

import (
	"context"
	"os"
	"testing"
	"time"
)

// This opt-in integration test checks the real free-provider and public-page
// paths without making the normal test suite depend on internet availability.
func TestLiveWebAccessIntegration(t *testing.T) {
	if os.Getenv("DENOVA_LIVE_WEB_ACCESS_TEST") != "1" {
		t.Skip("set DENOVA_LIVE_WEB_ACCESS_TEST=1 to run live web access integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := New(testWebAccessConfig())
	if err != nil {
		t.Fatal(err)
	}
	search, err := client.Search(ctx, SearchRequest{Query: "golang Go programming language official website", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(search.Results) == 0 {
		t.Fatal("live free-provider search returned no results")
	}
	t.Logf("search provider=%s results=%d first=%s", search.Provider, len(search.Results), search.Results[0].URL)

	fetched, err := client.Fetch(ctx, FetchRequest{URL: "https://go.dev/", MaxChars: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if fetched.Content == "" || fetched.FinalURL == "" {
		t.Fatalf("live fetch returned incomplete response: %+v", fetched)
	}
	t.Logf("fetch final_url=%s chars=%d returned=%d", fetched.FinalURL, fetched.TotalChars, len([]rune(fetched.Content)))
}
