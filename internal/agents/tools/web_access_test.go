package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/webaccess"
)

type stubWebAccessClient struct {
	fetchResponse webaccess.FetchResponse
}

func (stub stubWebAccessClient) Search(context.Context, webaccess.SearchRequest) (webaccess.SearchResponse, error) {
	return webaccess.SearchResponse{}, nil
}

func (stub stubWebAccessClient) Fetch(context.Context, webaccess.FetchRequest) (webaccess.FetchResponse, error) {
	return stub.fetchResponse, nil
}

func TestNewWebAccessToolsRegistersSearchAndFetch(t *testing.T) {
	registered, err := newWebAccessTools(&config.Config{WebAccess: config.DefaultWebAccessConfig()})
	if err != nil {
		t.Fatal(err)
	}
	if len(registered) != 2 {
		t.Fatalf("expected web_search and web_fetch, got %d tools", len(registered))
	}
	wantNames := []string{config.AgentToolWebSearch, webFetchToolName}
	for index, tool := range registered {
		info, err := tool.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name != wantNames[index] {
			t.Fatalf("tool %d name = %q, want %q", index, info.Name, wantNames[index])
		}
	}
}

func TestNewWebAccessToolsFillsOmittedRuntimeLimits(t *testing.T) {
	registered, err := newWebAccessTools(&config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(registered) != 2 {
		t.Fatalf("expected two web tools, got %d", len(registered))
	}
}

func TestWebAccessToolsExposeEvidenceCitationContract(t *testing.T) {
	registered, err := buildWebAccessTools(stubWebAccessClient{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range registered {
		info, err := tool.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, contract := range []string{
			"discovery only",
			"successful web_fetch",
			"same paragraph or list item",
			"[source title](final_url)",
			"Never invent",
			"输出协议允许 Markdown",
		} {
			if !strings.Contains(info.Desc, contract) {
				t.Fatalf("%s description is missing citation contract %q: %s", info.Name, contract, info.Desc)
			}
		}
	}
}

func TestWebFetchToolReturnsStructuredRecoveryResult(t *testing.T) {
	want := webaccess.FetchResponse{
		Status:          webaccess.FetchStatusBlocked,
		Attempts:        []webaccess.FetchAttempt{{Method: webaccess.FetchMethodDirectHTTP, Outcome: webaccess.FetchAttemptAccessDenied, HTTPStatus: 403}},
		RetryStrategy:   webaccess.FetchRetryUseAlternateSource,
		SuggestedAction: "Use another public source. 改用其他公开来源。",
		URL:             "https://example.com/article",
		FinalURL:        "https://example.com/article",
	}
	registered, err := buildWebAccessTools(stubWebAccessClient{fetchResponse: want})
	if err != nil {
		t.Fatal(err)
	}
	fetchTool := registered[1]
	info, err := fetchTool.Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, contract := range []string{"status", "attempts", "retry_strategy", "suggested_action", "不要"} {
		if !strings.Contains(info.Desc, contract) {
			t.Fatalf("web_fetch description does not explain %q recovery field: %s", contract, info.Desc)
		}
	}
	output, err := runToolForTest(context.Background(), fetchTool, `{"url":"https://example.com/article"}`)
	if err != nil {
		t.Fatal(err)
	}
	var got webaccess.FetchResponse
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("web_fetch output is not JSON: %v\n%s", err, output)
	}
	if got.Status != want.Status || got.RetryStrategy != want.RetryStrategy || got.SuggestedAction != want.SuggestedAction || len(got.Attempts) != 1 {
		t.Fatalf("web_fetch recovery output = %+v, want %+v", got, want)
	}
	for _, successOnlyField := range []string{`"fetch_method"`, `"content_type"`, `"content"`, `"start_index"`, `"warning"`} {
		if strings.Contains(output, successOnlyField) {
			t.Fatalf("blocked web_fetch output should omit success-only field %s: %s", successOnlyField, output)
		}
	}
}
