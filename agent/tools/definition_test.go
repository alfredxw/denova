package tools

import (
	"context"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestRegistryUsesToolInfoAsSingleNameSource(t *testing.T) {
	ctx := context.Background()
	tool, err := agent.InferTool("read_file", "read", func(context.Context, struct{}) (string, error) { return "ok", nil })
	if err != nil {
		t.Fatal(err)
	}
	definition := Definition{Tool: tool, Descriptor: Descriptor{
		Source: SourceRead, Execution: ExecutionParallelRead, Recovery: RecoveryReadOnly,
		ResultProjection: ResultBoundedModelContext, MaxResultBytes: 1024,
	}}
	registry, err := Build(ctx, definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Lookup("read_file"); !ok {
		t.Fatal("registry did not index Tool.Info name")
	}
	tools := registry.Tools()
	if len(tools) != 1 {
		t.Fatalf("tools = %d, want 1", len(tools))
	}
	info, err := tools[0].Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, ok := DescriptorFromInfo(info)
	if !ok || descriptor.Recovery != RecoveryReadOnly {
		t.Fatalf("descriptor = %#v, %t", descriptor, ok)
	}
	invokable, ok := tools[0].(agent.InvokableTool)
	if !ok {
		t.Fatal("definition wrapper lost invokable capability")
	}
	if result, err := invokable.InvokableRun(ctx, `{}`); err != nil || result != "ok" {
		t.Fatalf("run = %q, %v", result, err)
	}
}

func TestRegistryRejectsMissingContractAndDuplicates(t *testing.T) {
	ctx := context.Background()
	tool, err := agent.InferTool("same", "same", func(context.Context, struct{}) (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(ctx, Definition{Tool: tool}); err == nil {
		t.Fatal("expected missing contract error")
	}
	descriptor := Descriptor{
		Execution: ExecutionParallelRead, Recovery: RecoveryReadOnly,
		ResultProjection: ResultBoundedModelContext, MaxResultBytes: 1,
	}
	if err := Validate(ctx, Definition{Tool: tool, Descriptor: descriptor}, Definition{Tool: tool, Descriptor: descriptor}); err == nil {
		t.Fatal("expected duplicate error")
	}
}
