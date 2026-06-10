package agent

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"nova/config"
)

func TestNewWebSearchToolsRegistersWebSearch(t *testing.T) {
	tools, err := newWebSearchTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected one web search tool, got %d", len(tools))
	}
	if _, ok := tools[0].(tool.InvokableTool); !ok {
		t.Fatalf("web search tool should be invokable: %T", tools[0])
	}
	info, err := tools[0].Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != config.AgentToolWebSearch {
		t.Fatalf("expected tool name %q, got %q", config.AgentToolWebSearch, info.Name)
	}
}
