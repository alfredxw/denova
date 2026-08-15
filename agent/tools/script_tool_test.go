package tools

import (
	"context"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"
	agentscript "github.com/alfredxw/denova/agent/script"
	"github.com/invopop/jsonschema"
)

func TestSavedScriptToolRejectsIndirectCallCycle(t *testing.T) {
	engine, err := agentscript.NewEngine(agentscript.Config{
		MaxSourceBytes: 1024, MaxOutputBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	compile := func(name, source string) agentscript.Program {
		t.Helper()
		program, diagnostics := engine.Compile(context.Background(), agentscript.Source{Name: name, Code: source})
		if len(diagnostics) != 0 {
			t.Fatalf("compile %s: %#v", name, diagnostics)
		}
		return program
	}
	config := ScriptConfig{Engine: engine, MaxResultBytes: 4096}
	schema := &jsonschema.Schema{Type: "object"}
	a, err := SavedScriptTool(config, SavedScriptToolSpec{
		Name: "a", Description: "Call script B.", InputSchema: schema,
		Program: compile("tools/a.js", `return ctx.tools.call("b", {})`),
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := SavedScriptTool(config, SavedScriptToolSpec{
		Name: "b", Description: "Call script A.", InputSchema: schema,
		Program: compile("tools/b.js", `return ctx.tools.call("a", {})`),
	})
	if err != nil {
		t.Fatal(err)
	}
	toolset, err := agent.StaticToolsIdentified(
		agent.CapabilityIdentity{Kind: "test.script-cycle", Version: 1}, a, b,
	)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := agent.New(context.Background(), agent.Definition{
		Name: "script-cycle", Model: &taskModel{responses: []*agent.Message{
			agent.AssistantMessage("", []agent.ToolCall{{
				ID: "outer-a", Type: "function", Function: agent.FunctionCall{Name: "a", Arguments: `{}`},
			}}),
			agent.AssistantMessage("done", nil),
		}},
		ModelIdentity: agent.CapabilityIdentity{Kind: "test.script-cycle-model", Version: 1},
		Tools:         toolset,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	run, err := parent.Run(ctx, agent.Text("run"))
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(ctx); waitErr != nil || result.Status != agent.ResultCompleted {
		t.Fatalf("run result=%#v err=%v", result, waitErr)
	}
	nestedCalls := map[string]bool{}
	cycleFound := false
	for event := range run.Events() {
		switch payload := event.Payload.(type) {
		case agent.ToolInputStarted:
			if payload.ParentCallID != "" {
				nestedCalls[payload.CallID] = true
			}
		case agent.ToolFinished:
			if nestedCalls[payload.CallID] && payload.Projection != nil &&
				payload.Projection.SyntheticReason == agent.ToolSyntheticPolicyBlocked {
				cycleFound = true
			}
		}
	}
	if !cycleFound {
		t.Fatal("nested Script Tool cycle was not rejected visibly")
	}
}

func TestSavedScriptToolCallsAnotherSavedToolThroughJSONBoundary(t *testing.T) {
	engine, err := agentscript.NewEngine(agentscript.Config{MaxSourceBytes: 1024, MaxOutputBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	compile := func(name, source string) agentscript.Program {
		t.Helper()
		program, diagnostics := engine.Compile(context.Background(), agentscript.Source{Name: name, Code: source})
		if len(diagnostics) != 0 {
			t.Fatalf("compile %s: %#v", name, diagnostics)
		}
		return program
	}
	config := ScriptConfig{Engine: engine, MaxResultBytes: 4096}
	schema := &jsonschema.Schema{Type: "object"}
	outer, err := SavedScriptTool(config, SavedScriptToolSpec{
		Name: "outer", Description: "Call the reusable inner tool.", InputSchema: schema,
		Program: compile("tools/outer.js", `const result = ctx.tools.call("inner", input); return result.output`),
	})
	if err != nil {
		t.Fatal(err)
	}
	inner, err := SavedScriptTool(config, SavedScriptToolSpec{
		Name: "inner", Description: "Increment one input value.", InputSchema: schema,
		Program: compile("tools/inner.js", `return {value: input.value + 1}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	toolset, err := agent.StaticToolsIdentified(agent.CapabilityIdentity{Kind: "test.script-chain", Version: 1}, outer, inner)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := agent.New(context.Background(), agent.Definition{
		Name: "script-chain", Model: &taskModel{responses: []*agent.Message{
			agent.AssistantMessage("", []agent.ToolCall{{
				ID: "outer-call", Type: "function", Function: agent.FunctionCall{Name: "outer", Arguments: `{"value":1}`},
			}}),
			agent.AssistantMessage("done", nil),
		}},
		ModelIdentity: agent.CapabilityIdentity{Kind: "test.script-chain-model", Version: 1}, Tools: toolset,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close(context.Background())
	run, err := parent.Run(context.Background(), agent.Text("run"))
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != agent.ResultCompleted {
		t.Fatalf("run result=%#v err=%v", result, waitErr)
	}
	var outerResult string
	childVisible := false
	for event := range run.Events() {
		if finished, ok := event.Payload.(agent.ToolFinished); ok && finished.Projection != nil {
			if finished.Name == "outer" {
				outerResult = finished.Projection.ModelContent
			}
			if finished.Name == "inner" {
				childVisible = true
			}
		}
	}
	if outerResult != `{"value":2}` || !childVisible {
		t.Fatalf("outer result=%q child_visible=%t", outerResult, childVisible)
	}
}
