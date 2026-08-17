package tools

import (
	"context"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestAskRejectsChildInvocationBeforeInteractionLookup(t *testing.T) {
	toolset := Ask()
	definitions, err := toolset.PrepareTools(context.Background(), agent.ToolRequest{})
	if err != nil || len(definitions) != 1 {
		t.Fatalf("Ask definitions=%d error=%v", len(definitions), err)
	}
	child, finish, err := agent.BeginChildInvocation(context.Background(), "researcher")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := finish(); err != nil {
			t.Error(err)
		}
	}()
	_, err = definitions[0].Tool.Run(child, `{"questions":[{"id":"q","prompt":"问题","allow_free_text":true}]}`)
	if err == nil || !strings.Contains(err.Error(), "root Agent invocation") {
		t.Fatalf("child Ask error=%v", err)
	}
}
