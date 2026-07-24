package tools

import (
	"context"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

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
		if _, ok := tool.(agent.InvokableTool); !ok {
			t.Fatalf("tool %d should be invokable: %T", index, tool)
		}
		info, err := tool.Info(context.Background())
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
