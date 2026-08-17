package harnessstate

import (
	"context"
	"encoding/json"
	"testing"

	"denova/config"

	agentscript "github.com/alfredxw/denova/agent/script"
	agentstate "github.com/alfredxw/denova/agent/state"
	publictools "github.com/alfredxw/denova/agent/tools"
)

func scriptToolConfigForTest(engine *agentscript.Engine) publictools.ScriptConfig {
	return publictools.ScriptConfig{Engine: engine, MaxResultBytes: config.DefaultAgentToolResultLimitKB * 1024}
}

func TestManagerMaterializesSavedScriptToolAsOrdinaryDefinition(t *testing.T) {
	manager := openTestManager(t)
	ctx := context.Background()
	current, err := manager.Store().Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Store().Update(ctx, agentstate.ChangeSet{
		BaseRevision: current.Revision,
		Changes: []agentstate.Change{{Path: "tools/echo_input.js", Content: []byte(`---
name: echo_input
description: Return the validated input unchanged.
agents: [general, ide]
input_schema:
  type: object
  properties:
    value:
      type: string
      description: Value to return.
  required: [value]
---
return { value: input.value }
`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := manager.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	metadata := harness.ScriptToolMetadata()
	if len(metadata) != 1 || metadata[0].Name != "echo_input" || metadata[0].Resource != "tools/echo_input.js" {
		t.Fatalf("Script Tool metadata = %#v", metadata)
	}
	var schema map[string]any
	if err := json.Unmarshal(metadata[0].InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("additionalProperties default = %#v", schema["additionalProperties"])
	}
	engine, err := scriptEngine()
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := harness.ScriptToolDefinitions(config.AgentKindGeneral, scriptToolConfigForTest(engine))
	if err != nil || len(definitions) != 1 {
		t.Fatalf("definitions = %#v err=%v", definitions, err)
	}
	toolResult, err := definitions[0].Tool.Run(ctx, `{"value":"hello"}`)
	if err != nil || toolResult.ModelContent != `{"value":"hello"}` {
		t.Fatalf("Script Tool result = %#v err=%v", toolResult, err)
	}
}

func TestManagerReturnsIndependentScriptToolDiagnostics(t *testing.T) {
	manager := openTestManager(t)
	ctx := context.Background()
	current, err := manager.Store().Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Store().Update(ctx, agentstate.ChangeSet{
		BaseRevision: current.Revision,
		Changes: []agentstate.Change{
			{Path: "tools/read.js", Content: []byte(`---
name: read
description: 读取文件。
agents: [unknown]
input_schema:
  type: object
  surprise: true
---
return input
`)},
			{Path: "tools/broken.js", Content: []byte(`---
name: broken
description: Return a value.
agents: [general]
input_schema:
  type: object
---
return {
`)},
		},
	})
	validation, ok := err.(*agentstate.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T %v", err, err)
	}
	codes := make(map[string]bool)
	compileLine := 0
	for _, diagnostic := range validation.Diagnostics {
		codes[diagnostic.Code] = true
		if diagnostic.Code == "script_compile_failed" {
			compileLine = diagnostic.Line
		}
	}
	for _, code := range []string{"script_description_not_english", "script_agent_invalid", "script_schema_unknown_keyword", "script_tool_name_conflict", "script_compile_failed"} {
		if !codes[code] {
			t.Fatalf("missing diagnostic %q in %#v", code, validation.Diagnostics)
		}
	}
	if compileLine != 9 {
		t.Fatalf("compile diagnostic line = %d, want source line 9", compileLine)
	}
}
