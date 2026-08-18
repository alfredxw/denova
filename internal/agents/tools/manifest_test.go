package tools

import (
	"context"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

func TestValidateAgainstManifestAcceptsGeneratedContractAndRejectsDrift(t *testing.T) {
	tool, err := agent.InferTool("read", "Read a file.", func(context.Context, struct{}) (string, error) { return "ok", nil })
	if err != nil {
		t.Fatal(err)
	}
	descriptor := boundedReadDescriptor(agent.ToolSourceRead, config.AgentToolFilesystemRead, agent.ToolResultRecoveryRead)
	definition := agent.ToolDefinition{Tool: tool, Descriptor: descriptor}
	manifest := config.ResolveAgentToolManifestForGOOS(config.ResolvedAgentToolSettings{config.AgentToolFilesystemRead: true}, config.AgentKindIDE, "linux")
	if err := ValidateAgainstManifest(context.Background(), []agent.ToolDefinition{definition}, manifest); err != nil {
		t.Fatalf("generated manifest rejected matching descriptor: %v", err)
	}

	drifts := map[string]func(*agent.ToolDescriptor){
		"source": func(descriptor *agent.ToolDescriptor) {
			descriptor.Source = agent.ToolSourceOther
		},
		"result recovery": func(descriptor *agent.ToolDescriptor) {
			descriptor.ResultRecoveryKind = agent.ToolResultRecoveryRerun
		},
		"result limit": func(descriptor *agent.ToolDescriptor) {
			descriptor.MaxResultBytes++
		},
		"presentation": func(descriptor *agent.ToolDescriptor) {
			descriptor.Presentation = agent.UniformToolPresentation(agent.ToolPresentationTerminal)
		},
	}
	for name, drift := range drifts {
		t.Run(name, func(t *testing.T) {
			changed := definition
			drift(&changed.Descriptor)
			err = ValidateAgainstManifest(context.Background(), []agent.ToolDefinition{changed}, manifest)
			if err == nil || !strings.Contains(err.Error(), "descriptor drift") {
				t.Fatalf("drift error = %v", err)
			}
		})
	}
}

func TestValidateAgainstManifestUsesPerToolRecoveryAndRuntimeLimit(t *testing.T) {
	tool, err := agent.InferTool("glob", "Find files.", func(context.Context, struct{}) (string, error) { return "ok", nil })
	if err != nil {
		t.Fatal(err)
	}
	const runtimeLimit = 64 << 10
	descriptor := boundedReadDescriptor(agent.ToolSourceRead, config.AgentToolFilesystemRead, agent.ToolResultRecoveryRerun)
	descriptor.MaxResultBytes = runtimeLimit
	definition := agent.ToolDefinition{Tool: tool, Descriptor: descriptor}
	manifest := config.ResolveAgentToolManifestForGOOS(
		config.ResolvedAgentToolSettings{config.AgentToolFilesystemRead: true},
		config.AgentKindIDE, "linux", runtimeLimit,
	)
	if err := ValidateAgainstManifest(context.Background(), []agent.ToolDefinition{definition}, manifest); err != nil {
		t.Fatalf("generated per-tool contract rejected: %v", err)
	}
	definition.Descriptor.ResultRecoveryKind = agent.ToolResultRecoveryRead
	if err := ValidateAgainstManifest(context.Background(), []agent.ToolDefinition{definition}, manifest); err == nil || !strings.Contains(err.Error(), "descriptor drift") {
		t.Fatalf("per-tool recovery drift error = %v", err)
	}
}

func TestManifestValidatorIdentityExcludesPresentation(t *testing.T) {
	manifest := config.ResolveAgentToolManifestForGOOS(config.ResolvedAgentToolSettings{config.AgentToolFilesystemRead: true}, config.AgentKindIDE, "linux")
	identity, _, err := ManifestValidator(manifest)
	if err != nil {
		t.Fatal(err)
	}
	changed := append([]config.ResolvedAgentToolCapability(nil), manifest...)
	if len(changed) == 0 {
		t.Fatal("expected non-empty manifest")
	}
	changed[0].Descriptor.CallPresentation = agent.ToolPresentationTerminal
	changed[0].Descriptor.ResultPresentation = agent.ToolPresentationTerminal
	changed[0].ToolDescriptors = cloneDescriptorSummaries(changed[0].ToolDescriptors)
	for name, descriptor := range changed[0].ToolDescriptors {
		descriptor.CallPresentation = agent.ToolPresentationTerminal
		descriptor.ResultPresentation = agent.ToolPresentationTerminal
		changed[0].ToolDescriptors[name] = descriptor
	}
	changedIdentity, _, err := ManifestValidator(changed)
	if err != nil {
		t.Fatal(err)
	}
	if changedIdentity != identity {
		t.Fatalf("presentation-only manifest change rotated validator identity: before=%+v after=%+v", identity, changedIdentity)
	}
}

func TestManifestValidatorOwnsNestedManifestState(t *testing.T) {
	manifest := config.ResolveAgentToolManifestForGOOS(config.ResolvedAgentToolSettings{config.AgentToolFilesystemRead: true}, config.AgentKindIDE, "linux")
	_, validate, err := ManifestValidator(manifest)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := agent.InferTool("read", "Read a file.", func(context.Context, struct{}) (string, error) { return "ok", nil })
	if err != nil {
		t.Fatal(err)
	}
	definition := agent.ToolDefinition{Tool: tool, Descriptor: boundedReadDescriptor(agent.ToolSourceRead, config.AgentToolFilesystemRead, agent.ToolResultRecoveryRead)}

	manifest[0].ToolNames[0] = "changed"
	delete(manifest[0].ToolDescriptors, "read")
	if err := validate(context.Background(), []agent.ToolDefinition{definition}); err != nil {
		t.Fatalf("caller mutation changed validator policy: %v", err)
	}
}
