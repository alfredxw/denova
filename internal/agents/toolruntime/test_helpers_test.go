package toolruntime

import (
	"context"
	"errors"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agenttool "denova/internal/agents/tool"
	"denova/internal/agents/toolresult"
	producttools "denova/internal/agents/tools"
)

type testTextToolEndpoint func(context.Context, string, ...agent.ToolOption) (string, error)

func wrapTextToolCallForTest(middleware agent.Middleware, endpoint testTextToolEndpoint, toolCtx *agent.ToolContext) (testTextToolEndpoint, error) {
	wrapped, err := middleware.WrapToolCall(
		context.Background(),
		func(ctx context.Context, arguments string, options ...agent.ToolOption) (agent.ToolResult, error) {
			content, runErr := endpoint(ctx, arguments, options...)
			return agent.TextToolResult(content), runErr
		},
		toolCtx,
	)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, arguments string, options ...agent.ToolOption) (string, error) {
		result, runErr := wrapped(ctx, arguments, options...)
		return result.ModelContent, runErr
	}, nil
}

func testToolContext(name, callID string) *agent.ToolContext {
	var descriptor agent.ToolDescriptor
	switch name {
	case "read", "grep", "search_story_history":
		descriptor = producttools.BoundedReadDescriptor(agent.ToolSourceRead, config.AgentToolWorkspaceRead)
		if name == "search_story_history" {
			descriptor = producttools.BoundedReadDescriptor(agenttool.ToolSourceHistory, "")
		}
	case "write", "edit":
		descriptor = producttools.WorkspaceWriteDescriptor(agent.ToolSourceWrite, config.AgentToolWorkspaceWrite, agent.ToolRecoveryReconcilable)
	case "bash", "pwsh":
		descriptor = agent.ToolDescriptor{
			Source: agent.ToolSourceShell, Capability: config.AgentToolShell,
			Execution: agent.ToolExecutionWorkspaceExclusive, MutationScope: agent.ToolMutationExternal,
			PostCheck: agent.ToolPostCheckExternalReceipt, Recovery: agent.ToolRecoveryNonIdempotent,
			ResultProjection: agent.ToolResultBoundedModelContext, ContextRetention: agent.ToolContextReceipt,
			Steering: agent.SteeringFinishCurrent, MaxResultBytes: toolresult.DefaultMaxBytes,
		}
	default:
		return &agent.ToolContext{Name: name, ProviderCallID: callID}
	}
	return &agent.ToolContext{
		Name: name, ProviderCallID: callID,
		Definition: agent.ToolDefinitionSnapshot{Info: &agent.ToolInfo{Name: name}, Descriptor: descriptor},
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
