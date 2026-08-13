package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToolPresentationIsDisplayOnlyAndRoundTripsThroughLifecycleMetadata(t *testing.T) {
	descriptor := validDescriptorForScope(ToolMutationNone)
	descriptor.Presentation = ToolPresentation{Call: ToolPresentationSearch}

	encodedDescriptor, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedDescriptor), "presentation") {
		t.Fatalf("display presentation entered descriptor identity JSON: %s", encodedDescriptor)
	}

	metadata, err := toolExecutionMetadata(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	projected := decodeToolDescriptorMetadata(metadata)
	if projected == nil || projected.Presentation.Call != ToolPresentationSearch || projected.Presentation.Result != ToolPresentationSearch {
		t.Fatalf("lifecycle presentation = %#v, want uniform search", projected)
	}
}

func TestToolPresentationRejectsUnknownKind(t *testing.T) {
	descriptor := validDescriptorForScope(ToolMutationNone)
	descriptor.Presentation = UniformToolPresentation(ToolPresentationKind("future"))
	if err := descriptor.Validate(); err == nil {
		t.Fatal("descriptor accepted an unknown presentation kind")
	}
}

func TestToolDescriptorExecutionMutationMatrix(t *testing.T) {
	executions := []ToolExecutionClass{
		ToolExecutionParallelRead,
		ToolExecutionWorkspaceExclusive,
		ToolExecutionSessionExclusive,
		ToolExecutionConfigExclusive,
		ToolExecutionInteractiveWait,
		ToolExecutionChild,
	}
	scopes := []ToolMutationScope{
		ToolMutationNone,
		ToolMutationWorkspace,
		ToolMutationSession,
		ToolMutationConfig,
		ToolMutationExternal,
	}
	allowed := map[ToolExecutionClass]map[ToolMutationScope]bool{
		ToolExecutionParallelRead:       {ToolMutationNone: true},
		ToolExecutionWorkspaceExclusive: {ToolMutationWorkspace: true, ToolMutationExternal: true},
		ToolExecutionSessionExclusive:   {ToolMutationSession: true, ToolMutationExternal: true},
		ToolExecutionConfigExclusive:    {ToolMutationConfig: true},
		ToolExecutionInteractiveWait:    {ToolMutationNone: true, ToolMutationSession: true},
		ToolExecutionChild:              {ToolMutationNone: true},
	}
	for _, execution := range executions {
		for _, scope := range scopes {
			descriptor := validDescriptorForScope(scope)
			descriptor.Execution = execution
			err := descriptor.Validate()
			if got, want := err == nil, allowed[execution][scope]; got != want {
				t.Errorf("execution=%q scope=%q valid=%v, want %v (err=%v)", execution, scope, got, want, err)
			}
		}
	}
}

func TestToolDescriptorPostCheckMatrix(t *testing.T) {
	scopes := []ToolMutationScope{
		ToolMutationNone,
		ToolMutationWorkspace,
		ToolMutationSession,
		ToolMutationConfig,
		ToolMutationExternal,
	}
	policies := []ToolPostCheckPolicy{
		ToolPostCheckNone,
		ToolPostCheckWorkspaceChange,
		ToolPostCheckSessionState,
		ToolPostCheckConfigRevision,
		ToolPostCheckExternalReceipt,
	}
	matching := map[ToolMutationScope]ToolPostCheckPolicy{
		ToolMutationNone:      ToolPostCheckNone,
		ToolMutationWorkspace: ToolPostCheckWorkspaceChange,
		ToolMutationSession:   ToolPostCheckSessionState,
		ToolMutationConfig:    ToolPostCheckConfigRevision,
		ToolMutationExternal:  ToolPostCheckExternalReceipt,
	}
	for _, scope := range scopes {
		for _, policy := range policies {
			descriptor := validDescriptorForScope(scope)
			descriptor.PostCheck = policy
			err := descriptor.Validate()
			want := policy == ToolPostCheckNone || policy == matching[scope]
			if got := err == nil; got != want {
				t.Errorf("scope=%q post_check=%q valid=%v, want %v (err=%v)", scope, policy, got, want, err)
			}
		}
	}
}

func TestToolDescriptorInterruptibleWaitIsReadOnly(t *testing.T) {
	descriptor := validDescriptorForScope(ToolMutationNone)
	descriptor.Execution = ToolExecutionInteractiveWait
	descriptor.Steering = SteeringInterruptibleWait
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("read-only interactive wait: %v", err)
	}

	descriptor = validDescriptorForScope(ToolMutationSession)
	descriptor.Execution = ToolExecutionInteractiveWait
	descriptor.Steering = SteeringInterruptibleWait
	if err := descriptor.Validate(); err == nil {
		t.Fatal("interruptible wait accepted a session mutation")
	}
}

func TestToolDescriptorRequiresExplicitResultRetention(t *testing.T) {
	descriptor := validDescriptorForScope(ToolMutationNone)
	descriptor.ResultRetention = ""
	if err := descriptor.Validate(); err == nil {
		t.Fatal("descriptor accepted an implicit cross-turn retention policy")
	}
}

func TestToolDescriptorAcceptsApplicationOwnedToolSource(t *testing.T) {
	descriptor := validDescriptorForScope(ToolMutationNone)
	descriptor.Source = ToolSource("denova.lore")
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("custom tool source: %v", err)
	}

	for _, source := range []ToolSource{"", "Lore", " denova.lore", ".lore", "denova/lore"} {
		descriptor.Source = source
		if err := descriptor.Validate(); err == nil {
			t.Fatalf("invalid custom tool source %q was accepted", source)
		}
	}
}

func TestToolDescriptorAcceptsStableResultRetentionModes(t *testing.T) {
	for _, mode := range []ToolResultRetentionMode{
		ToolResultDeferred,
		ToolResultEagerCandidate,
		ToolResultProtected,
	} {
		descriptor := validDescriptorForScope(ToolMutationNone)
		descriptor.ResultRetention = mode
		if mode == ToolResultEagerCandidate {
			descriptor.ResultRecoveryKind = ToolResultRecoveryRerun
		}
		if err := descriptor.Validate(); err != nil {
			t.Errorf("result retention %q: %v", mode, err)
		}
	}

	descriptor := validDescriptorForScope(ToolMutationNone)
	descriptor.ResultRetention = ToolResultRetentionMode("future")
	if err := descriptor.Validate(); err == nil {
		t.Fatal("descriptor accepted an unknown result retention mode")
	}
}

func validDescriptorForScope(scope ToolMutationScope) ToolDescriptor {
	descriptor := ToolDescriptor{
		Source:           ToolSourceOther,
		Execution:        ToolExecutionParallelRead,
		MutationScope:    scope,
		PostCheck:        ToolPostCheckNone,
		Recovery:         ToolRecoveryReadOnly,
		ResultProjection: ToolResultBoundedModelContext,
		ResultRetention:  ToolResultDeferred,
		Steering:         SteeringFinishCurrent,
		MaxResultBytes:   1024,
	}
	switch scope {
	case ToolMutationWorkspace, ToolMutationExternal:
		descriptor.Execution = ToolExecutionWorkspaceExclusive
	case ToolMutationSession:
		descriptor.Execution = ToolExecutionSessionExclusive
	case ToolMutationConfig:
		descriptor.Execution = ToolExecutionConfigExclusive
	}
	return descriptor
}
