package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	agentpermission "github.com/alfredxw/denova/agent/permission"

	"denova/config"
	"denova/internal/webaccess"
)

type stubWebAccessClient struct {
	searchResponse webaccess.SearchResponse
	fetchResponse  webaccess.FetchResponse
}

type managedWebAccessProbe struct {
	mu      sync.Mutex
	created int
	closed  int
}

type managedWebAccessStub struct{ probe *managedWebAccessProbe }

func (*managedWebAccessStub) Search(context.Context, webaccess.SearchRequest) (webaccess.SearchResponse, error) {
	return webaccess.SearchResponse{Schema: webaccess.SearchResponseSchema, Status: webaccess.SearchStatusNoResults}, nil
}

func (*managedWebAccessStub) Fetch(_ context.Context, request webaccess.FetchRequest) (webaccess.FetchResponse, error) {
	return webaccess.FetchResponse{
		Schema: webaccess.FetchResponseSchema, Status: webaccess.FetchStatusSuccess,
		FetchMethod: webaccess.FetchMethodDirectHTTP, URL: request.URL, FinalURL: request.URL,
		Content: "ok",
	}, nil
}

func (client *managedWebAccessStub) Close(context.Context) error {
	client.probe.mu.Lock()
	client.probe.closed++
	client.probe.mu.Unlock()
	return nil
}

type webAccessLifecycleModel struct {
	mu    sync.Mutex
	calls int
}

func (model *webAccessLifecycleModel) Generate(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.Message, error) {
	model.mu.Lock()
	model.calls++
	call := model.calls
	model.mu.Unlock()
	if call%2 == 1 {
		return agent.AssistantMessage("", []agent.ToolCall{{
			ID: fmt.Sprintf("fetch-%d", call), Type: "function",
			Function: agent.FunctionCall{Name: webFetchToolName, Arguments: `{"url":"https://example.com"}`},
		}}), nil
	}
	return agent.AssistantMessage("done", nil), nil
}

func (model *webAccessLifecycleModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := model.Generate(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
}

func (stub stubWebAccessClient) Search(context.Context, webaccess.SearchRequest) (webaccess.SearchResponse, error) {
	return stub.searchResponse, nil
}

func (stub stubWebAccessClient) Fetch(context.Context, webaccess.FetchRequest) (webaccess.FetchResponse, error) {
	return stub.fetchResponse, nil
}

func TestWebAccessToolsAreConstructedWithIndependentCapabilities(t *testing.T) {
	client := stubWebAccessClient{}
	search, err := newWebSearchTool(client, "web_search_capability")
	if err != nil {
		t.Fatal(err)
	}
	fetch, err := newWebFetchTool(client, "web_fetch_capability")
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{config.AgentToolWebSearch, webFetchToolName}
	wantCapabilities := []string{"web_search_capability", "web_fetch_capability"}
	for index, tool := range []agent.ToolDefinition{search, fetch} {
		info, err := tool.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name != wantNames[index] {
			t.Fatalf("tool %d name = %q, want %q", index, info.Name, wantNames[index])
		}
		if tool.Descriptor.Capability != wantCapabilities[index] {
			t.Fatalf("tool %s capability = %q, want %q", info.Name, tool.Descriptor.Capability, wantCapabilities[index])
		}
	}
	if search.Descriptor.ResultRetention != agent.ToolResultDeferred {
		t.Fatalf("web_search retention = %q, want deferred", search.Descriptor.ResultRetention)
	}
	if fetch.Descriptor.ResultRetention != agent.ToolResultEagerCandidate {
		t.Fatalf("web_fetch retention = %q, want eager candidate", fetch.Descriptor.ResultRetention)
	}
	if search.Descriptor.ResultRecoveryKind != agent.ToolResultRecoveryRerun || fetch.Descriptor.ResultRecoveryKind != agent.ToolResultRecoveryRefetch {
		t.Fatalf("web recovery = search:%q fetch:%q", search.Descriptor.ResultRecoveryKind, fetch.Descriptor.ResultRecoveryKind)
	}
}

func TestNewWebAccessClientFillsOmittedRuntimeLimits(t *testing.T) {
	client, err := newWebAccessClient(&config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("web access client is nil")
	}
}

func TestInvocationWebAccessClientClosesOneRuntimePerAgentRun(t *testing.T) {
	probe := &managedWebAccessProbe{}
	client, err := newInvocationWebAccessClient(func() (managedWebAccessClient, error) {
		probe.mu.Lock()
		probe.created++
		probe.mu.Unlock()
		return &managedWebAccessStub{probe: probe}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fetch, err := newWebFetchTool(client, config.AgentToolWebFetch)
	if err != nil {
		t.Fatal(err)
	}
	toolset, err := agent.StaticTools(fetch)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := agent.New(context.Background(), agent.Definition{
		Name: "web-access-lifecycle", Model: &webAccessLifecycleModel{},
		Tools: toolset, Permission: agentpermission.FullAccess(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	for index := 0; index < 2; index++ {
		run, err := owner.Run(context.Background(), agent.Input{
			Text: "fetch", IdempotencyKey: fmt.Sprintf("web-access-lifecycle-%d", index),
		})
		if err != nil {
			t.Fatal(err)
		}
		for range run.Events() {
		}
		if result, err := run.Wait(context.Background()); err != nil || result.Status != agent.ResultCompleted {
			t.Fatalf("web access lifecycle run %d result=%#v error=%v", index, result, err)
		}
	}
	probe.mu.Lock()
	created, closed := probe.created, probe.closed
	probe.mu.Unlock()
	if created != 2 || closed != 2 {
		t.Fatalf("managed web clients created=%d closed=%d, want 2/2", created, closed)
	}
}

func TestWebAccessToolsExposeEvidenceCitationContract(t *testing.T) {
	client := stubWebAccessClient{}
	search, err := newWebSearchTool(client, "web_search")
	if err != nil {
		t.Fatal(err)
	}
	fetch, err := newWebFetchTool(client, "web_fetch")
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []agent.ToolDefinition{search, fetch} {
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
			"output protocol permits Markdown",
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
	fetchTool, err := newWebFetchTool(stubWebAccessClient{fetchResponse: want}, "web_fetch")
	if err != nil {
		t.Fatal(err)
	}
	info, err := fetchTool.Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, contract := range []string{"status", "attempts", "retry_strategy", "suggested_action", "do not immediately retry"} {
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
	if got.Schema != webaccess.FetchResponseSchema || got.Status != want.Status || got.RetryStrategy != want.RetryStrategy || got.SuggestedAction != want.SuggestedAction || len(got.Attempts) != 1 {
		t.Fatalf("web_fetch recovery output = %+v, want %+v", got, want)
	}
	for _, successOnlyField := range []string{`"fetch_method"`, `"content_type"`, `"content"`, `"start_index"`, `"warning"`} {
		if strings.Contains(output, successOnlyField) {
			t.Fatalf("blocked web_fetch output should omit success-only field %s: %s", successOnlyField, output)
		}
	}
}

func TestWebSearchToolReturnsVersionedResult(t *testing.T) {
	searchTool, err := newWebSearchTool(stubWebAccessClient{searchResponse: webaccess.SearchResponse{
		Status: webaccess.SearchStatusNoResults,
		Query:  "denova",
	}}, "web_search")
	if err != nil {
		t.Fatal(err)
	}
	output, err := runToolForTest(context.Background(), searchTool, `{"query":"denova"}`)
	if err != nil {
		t.Fatal(err)
	}
	var got webaccess.SearchResponse
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatal(err)
	}
	if got.Schema != webaccess.SearchResponseSchema || got.Status != webaccess.SearchStatusNoResults {
		t.Fatalf("web_search result = %+v", got)
	}
}
