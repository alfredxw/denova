package webfetch

import (
	"context"
	"errors"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type testProvider struct{ artifact agent.ToolArtifactRef }

func (testProvider) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "webfetch.test", Version: 1}
}

func (provider testProvider) Fetch(_ context.Context, request Request) (Response, error) {
	if strings.Contains(request.URL, "fail") {
		return Response{}, errors.New("fetch failed")
	}
	return Response{URL: request.URL, Status: 200, Content: "ok", Artifact: &provider.artifact}, nil
}

func TestBatchPreservesArtifactsAndPartialErrors(t *testing.T) {
	artifact := agent.ToolArtifactRef{ID: "fetch-artifact", Purpose: agent.ToolArtifactPurposeAttachment, Complete: true}
	set, err := New(testProvider{artifact: artifact})
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := set.PrepareTools(context.Background(), agent.ToolRequest{})
	if err != nil || len(definitions) != 1 {
		t.Fatalf("definitions=%d err=%v", len(definitions), err)
	}
	result, err := definitions[0].Tool.Run(context.Background(), `{"requests":[{"url":"https://example.com"},{"url":"https://fail.example"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0] != artifact || !strings.Contains(result.ModelContent, `"error":"fetch failed"`) {
		t.Fatalf("result=%#v", result)
	}
}
