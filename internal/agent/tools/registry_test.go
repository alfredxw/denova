package tools

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
)

func TestCapabilitiesExposeStableManifest(t *testing.T) {
	capabilities := Capabilities()
	if len(capabilities) != 11 {
		t.Fatalf("capability count = %d, want 11", len(capabilities))
	}
	if capabilities[0].Source != AgentToolFileRead || capabilities[len(capabilities)-1].Source != AgentToolAgentConfigWrite {
		t.Fatalf("unexpected capability order: %#v", capabilities)
	}
	capabilities[0].Source = "mutated"
	if Capabilities()[0].Source != AgentToolFileRead {
		t.Fatalf("Capabilities should return a defensive copy")
	}
}

func TestAllowedUsesResolvedSettingsProjection(t *testing.T) {
	settings := Settings{FileRead: true, WebSearch: true}
	if !Allowed(settings, AgentToolFileRead) || !Allowed(settings, AgentToolWebSearch) {
		t.Fatalf("enabled capabilities should be allowed")
	}
	if Allowed(settings, AgentToolFileWrite) || Allowed(settings, "unknown") {
		t.Fatalf("disabled or unknown capabilities should be denied")
	}
}

func TestBuildAssemblesEnabledAdaptersInOrder(t *testing.T) {
	calls := []string{}
	mw := &adk.BaseChatModelAgentMiddleware{}
	result, err := Build(context.Background(), BuildRequest{
		Settings: Settings{FileRead: true, Skills: true, WebSearch: true},
		Middlewares: []MiddlewareRegistration{
			{
				Name:    "filesystem",
				Enabled: FilesystemAllowed,
				Build: func(context.Context, Settings) (adk.ChatModelAgentMiddleware, error) {
					calls = append(calls, "filesystem")
					return mw, nil
				},
			},
			{
				Name:    "skills",
				Enabled: CapabilityAllowed(AgentToolSkills),
				Build: func(context.Context, Settings) (adk.ChatModelAgentMiddleware, error) {
					calls = append(calls, "skills")
					return mw, nil
				},
			},
			{
				Name: "compaction",
				Build: func(context.Context, Settings) (adk.ChatModelAgentMiddleware, error) {
					calls = append(calls, "compaction")
					return mw, nil
				},
			},
			{
				Name: "orchestrator",
				Build: func(context.Context, Settings) (adk.ChatModelAgentMiddleware, error) {
					calls = append(calls, "orchestrator")
					return mw, nil
				},
			},
			{
				Name: "model_log",
				Build: func(context.Context, Settings) (adk.ChatModelAgentMiddleware, error) {
					calls = append(calls, "model_log")
					return mw, nil
				},
			},
		},
		Tools: []ToolRegistration{
			StaticTools("static", nil),
			{
				Name: "extra",
				Build: func(Settings) ([]tool.BaseTool, error) {
					calls = append(calls, "extra")
					return []tool.BaseTool{nil}, nil
				},
			},
			{
				Name:    "web_search",
				Enabled: CapabilityAllowed(AgentToolWebSearch),
				Build: func(Settings) ([]tool.BaseTool, error) {
					calls = append(calls, "web_search")
					return []tool.BaseTool{nil}, nil
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := []string{"filesystem", "skills", "compaction", "orchestrator", "model_log", "extra", "web_search"}
	if !sameStrings(calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
	if len(result.Tools) != 3 {
		t.Fatalf("tools = %d, want 3", len(result.Tools))
	}
	if len(result.Handlers) != 5 {
		t.Fatalf("handlers = %d, want 5", len(result.Handlers))
	}
}

func TestBuildSkipsWebSearchWhenCapabilityDisabled(t *testing.T) {
	called := false
	_, err := Build(context.Background(), BuildRequest{
		Settings: Settings{WebSearch: false},
		Tools: []ToolRegistration{{
			Name:    "web_search",
			Enabled: CapabilityAllowed(AgentToolWebSearch),
			Build: func(Settings) ([]tool.BaseTool, error) {
				called = true
				return nil, nil
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("web search factory should not run when capability is disabled")
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
