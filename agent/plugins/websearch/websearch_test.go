package websearch

import (
	"context"
	"errors"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type testProvider struct{}

func (testProvider) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "websearch.test", Version: 1}
}

func (testProvider) Search(_ context.Context, query Query) ([]Result, error) {
	if query.Text == "fail" {
		return nil, errors.New("provider failed")
	}
	return []Result{{Title: query.Text, URL: "https://example.com"}}, nil
}

func TestBatchReturnsPartialOutcomes(t *testing.T) {
	set, err := New(testProvider{})
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := set.PrepareTools(context.Background(), agent.ToolRequest{})
	if err != nil || len(definitions) != 1 {
		t.Fatalf("definitions=%d err=%v", len(definitions), err)
	}
	result, err := definitions[0].Tool.Run(context.Background(), `{"queries":[{"text":"ok"},{"text":"fail"},{"text":""}]}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"title":"ok"`, `"error":"provider failed"`, `"error":"invalid web search query"`} {
		if !strings.Contains(result.ModelContent, want) {
			t.Fatalf("result %q does not contain %q", result.ModelContent, want)
		}
	}
}
