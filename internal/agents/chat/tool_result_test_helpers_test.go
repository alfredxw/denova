package chat

import (
	"context"
	agenttool "denova/internal/agents/tool"
	"errors"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/toolresult"
)

func processorTestDecision(retention agent.ToolResultRetentionMode) agenttool.Decision {
	return agenttool.Decision{
		ToolName: "read", ProviderCallID: "call-42",
		Descriptor: agent.ToolDescriptor{
			Source: agent.ToolSourceRead, Execution: agent.ToolExecutionParallelRead,
			MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone,
			Recovery: agent.ToolRecoveryReadOnly, ResultRecoveryKind: agent.ToolResultRecoveryRead,
			ResultProjection: agent.ToolResultBoundedModelContext,
			ResultRetention:  retention, Steering: agent.SteeringFinishCurrent, MaxResultBytes: 1024,
		},
	}
}

func processorShellTestDecision() agenttool.Decision {
	return agenttool.Decision{
		ToolName: "bash", ProviderCallID: "call-shell", ExecutionID: "exec-shell",
		Descriptor: agent.ToolDescriptor{
			Source: agent.ToolSourceShell, Execution: agent.ToolExecutionWorkspaceExclusive,
			MutationScope: agent.ToolMutationExternal, PostCheck: agent.ToolPostCheckExternalReceipt,
			Recovery: agent.ToolRecoveryNonIdempotent, ResultProjection: agent.ToolResultBoundedModelContext,
			ResultRetention: agent.ToolResultProtected, Steering: agent.SteeringFinishCurrent, MaxResultBytes: 1024,
		},
	}
}

func processorTestProcessingPolicy(maxBytes int) toolresult.ProcessingPolicy {
	return toolresult.ProcessingPolicy{MaxBytes: maxBytes, EagerMinTokens: 32_000, ContextWindowTokens: 160_000}
}

type processorArtifactStore struct {
	request         agent.ToolArtifactRequest
	content         strings.Builder
	beginErr        error
	beginCalls      int
	returnedPurpose agent.ToolArtifactPurpose
	verified        bool
}

func (store *processorArtifactStore) BeginToolArtifact(_ context.Context, request agent.ToolArtifactRequest) (agent.ToolArtifactWriter, error) {
	store.beginCalls++
	store.request = request
	if store.beginErr != nil {
		return nil, store.beginErr
	}
	store.content.Reset()
	return &processorArtifactWriter{store: store}, nil
}

func (store *processorArtifactStore) VerifyToolArtifact(_ context.Context, _ agent.ToolArtifactRef, _ agent.ToolArtifactRequest) error {
	if !store.verified {
		return errors.New("artifact was not issued by this store")
	}
	return nil
}

type processorArtifactWriter struct {
	store    *processorArtifactStore
	terminal bool
}

func (writer *processorArtifactWriter) Write(data []byte) (int, error) {
	if writer.terminal {
		return 0, errors.New("writer is closed")
	}
	return writer.store.content.Write(data)
}

func (writer *processorArtifactWriter) Commit() (agent.ToolArtifactRef, error) {
	writer.terminal = true
	purpose := writer.store.returnedPurpose
	if purpose == "" {
		purpose = writer.store.request.Purpose
	}
	return agent.ToolArtifactRef{
		ID: "artifact-call-42", Purpose: purpose,
		ReadablePath: ".denova/artifacts/session/call-42.log",
		ContentType:  "text/plain; charset=utf-8", EstimatedBytes: int64(writer.store.content.Len()),
		EstimatedTokens: (writer.store.content.Len() + 3) / 4, Complete: true,
	}, nil
}

func (writer *processorArtifactWriter) Abort() error {
	writer.terminal = true
	return nil
}
