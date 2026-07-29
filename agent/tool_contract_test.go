package agent

import "testing"

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

func TestToolDescriptorRequiresExplicitContextRetention(t *testing.T) {
	descriptor := validDescriptorForScope(ToolMutationNone)
	descriptor.ContextRetention = ""
	if err := descriptor.Validate(); err == nil {
		t.Fatal("descriptor accepted an implicit cross-turn retention policy")
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
		ContextRetention: ToolContextReceipt,
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
