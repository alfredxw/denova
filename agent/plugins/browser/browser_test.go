package browser

import (
	"context"
	"errors"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type testController struct{ artifact agent.ToolArtifactRef }

func (testController) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "browser.test", Version: 1}
}

func (controller testController) Execute(_ context.Context, action Action) (ActionResult, error) {
	if action.Kind == "fail" {
		return ActionResult{}, errors.New("action failed")
	}
	return ActionResult{URL: "https://example.com", Artifact: &controller.artifact}, nil
}

func TestOrderedBatchPreservesArtifactsAndPartialErrors(t *testing.T) {
	artifact := agent.ToolArtifactRef{ID: "screenshot", Purpose: agent.ToolArtifactPurposeAttachment, Complete: true}
	set, err := New(testController{artifact: artifact})
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := set.PrepareTools(context.Background(), agent.ToolRequest{})
	if err != nil || len(definitions) != 1 {
		t.Fatalf("definitions=%d err=%v", len(definitions), err)
	}
	result, err := definitions[0].Tool.Run(context.Background(), `{"actions":[{"kind":"navigate"},{"kind":"fail"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0] != artifact || !strings.Contains(result.ModelContent, `"error":"action failed"`) {
		t.Fatalf("result=%#v", result)
	}
}
