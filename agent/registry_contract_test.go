package agent

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/invopop/jsonschema"
)

func TestRegistryCapturesImmutableDefinitionSnapshots(t *testing.T) {
	tool, err := InferTool("lookup", "original", func(context.Context, struct{}) (string, error) { return "ok", nil })
	if err != nil {
		t.Fatal(err)
	}
	definition := testToolDefinition(tool)
	registry, err := NewRegistry(context.Background(), definition)
	if err != nil {
		t.Fatal(err)
	}
	first, ok := registry.Snapshot("lookup")
	if !ok || first.Info == nil {
		t.Fatalf("snapshot = %#v", first)
	}
	first.Info.Name = "mutated"
	second, _ := registry.Snapshot("lookup")
	if second.Info.Name != "lookup" || second.Descriptor.Execution != ToolExecutionParallelRead {
		t.Fatalf("registry snapshot was mutable: %#v", second)
	}
}

type changingRegistryInfoTool struct{ calls atomic.Int32 }

func (tool *changingRegistryInfoTool) Info(context.Context) (*ToolInfo, error) {
	call := tool.calls.Add(1)
	return &ToolInfo{Name: "snapshot-" + string(rune('0'+call))}, nil
}

func (*changingRegistryInfoTool) Run(context.Context, string, ...ToolOption) (ToolResult, error) {
	return TextToolResult("ok"), nil
}

func TestRegistryReadsDefinitionInfoExactlyOnce(t *testing.T) {
	tool := &changingRegistryInfoTool{}
	registry, err := NewRegistry(context.Background(), testToolDefinition(tool))
	if err != nil {
		t.Fatal(err)
	}
	if tool.calls.Load() != 1 {
		t.Fatalf("Info calls = %d, want 1", tool.calls.Load())
	}
	if snapshot, ok := registry.Snapshot("snapshot-1"); !ok || snapshot.Info.Name != "snapshot-1" {
		t.Fatalf("snapshot = %#v, ok=%t", snapshot, ok)
	}
}

type invalidSchemaTool struct{ schema *jsonschema.Schema }

func (tool invalidSchemaTool) Info(context.Context) (*ToolInfo, error) {
	return &ToolInfo{Name: "invalid_schema", ParamsOneOf: NewParamsOneOfByJSONSchema(tool.schema)}, nil
}

func (invalidSchemaTool) Run(context.Context, string, ...ToolOption) (ToolResult, error) {
	return TextToolResult("unexpected"), nil
}

func TestRegistryRejectsMalformedSchemas(t *testing.T) {
	tests := []struct {
		name   string
		schema *jsonschema.Schema
	}{
		{name: "unknown type", schema: &jsonschema.Schema{Type: "future"}},
		{name: "invalid pattern", schema: &jsonschema.Schema{Type: "string", Pattern: "["}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewRegistry(context.Background(), testToolDefinition(invalidSchemaTool{schema: test.schema})); err == nil || !strings.Contains(err.Error(), "schema") {
				t.Fatalf("registry error = %v", err)
			}
		})
	}
}

func TestRegistryRejectsDuplicateAndInvalidDescriptors(t *testing.T) {
	tool, err := InferTool("lookup", "", func(context.Context, struct{}) (string, error) { return "ok", nil })
	if err != nil {
		t.Fatal(err)
	}
	definition := testToolDefinition(tool)
	if _, err := NewRegistry(context.Background(), definition, definition); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate error = %v", err)
	}
	invalid := definition
	invalid.Descriptor.MutatesWorkspace = true
	if _, err := NewRegistry(context.Background(), invalid); err == nil || !strings.Contains(err.Error(), "parallel read") {
		t.Fatalf("invalid descriptor error = %v", err)
	}
}
