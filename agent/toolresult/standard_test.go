package toolresult

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type memoryStorage struct {
	request agent.ToolArtifactRequest
	content bytes.Buffer
}

func (storage *memoryStorage) BeginToolArtifact(_ context.Context, request agent.ToolArtifactRequest) (agent.ToolArtifactWriter, error) {
	storage.request = request
	storage.content.Reset()
	return &memoryWriter{storage: storage}, nil
}

func (storage *memoryStorage) VerifyToolArtifact(_ context.Context, artifact agent.ToolArtifactRef, request agent.ToolArtifactRequest) error {
	if artifact.ID == "complete" && request.ToolCallID == storage.request.ToolCallID {
		return nil
	}
	return context.Canceled
}

type memoryWriter struct{ storage *memoryStorage }

func (writer *memoryWriter) Write(value []byte) (int, error) {
	return writer.storage.content.Write(value)
}
func (writer *memoryWriter) Abort() error { return nil }
func (writer *memoryWriter) Commit() (agent.ToolArtifactRef, error) {
	content := writer.storage.content.Bytes()
	hash := sha256.Sum256(content)
	return agent.ToolArtifactRef{
		ID: "complete", Purpose: agent.ToolArtifactPurposeCompleteModelOutput,
		ReadablePath: ".agent/artifacts/complete.log", ContentType: "text/plain; charset=utf-8",
		EstimatedBytes: int64(len(content)), Complete: true, SHA256: hex.EncodeToString(hash[:]),
	}, nil
}

func TestStandardMaterializesCompleteOutputAndBuildsRecoveryReceipt(t *testing.T) {
	storage := &memoryStorage{}
	ctx := agent.ContextWithToolArtifactBackend(context.Background(), storage)
	content := "HEAD-SENTINEL\n" + strings.Repeat("large result ", 80) + "\nTAIL-SENTINEL"
	descriptor := resultDescriptor(agent.ToolResultProtected)
	processed, err := Standard(Policy{MaxBytes: 256}).Process(ctx, agent.ToolResultProcessRequest{
		ToolName: "read", Arguments: `{"path":"chapter.md","api_key":"secret"}`,
		ExecutionID: "execution-1", ProviderCallID: "provider-1",
		Definition: agent.ToolDefinitionSnapshot{Descriptor: descriptor},
		Result:     agent.TextToolResult(content),
	})
	if err != nil {
		t.Fatal(err)
	}
	if storage.request.ToolCallID != "execution-1" || storage.content.String() != content {
		t.Fatalf("artifact request=%#v content=%q", storage.request, storage.content.String())
	}
	if len(processed.ModelContent) > 256 || !strings.Contains(processed.ModelContent, "HEAD-SENTINEL") ||
		!strings.Contains(processed.ModelContent, "TAIL-SENTINEL") || !processed.Metadata.ModelTruncated {
		t.Fatalf("model preview = %q metadata=%#v", processed.ModelContent, processed.Metadata)
	}
	if len(processed.Artifacts) != 1 || !processed.Artifacts[0].Complete ||
		processed.ContextHints == nil || processed.ContextHints.Recovery.ArtifactPath != ".agent/artifacts/complete.log" {
		t.Fatalf("artifact recovery = %#v %#v", processed.Artifacts, processed.ContextHints)
	}
	if processed.ProtectedReceipt == nil || strings.Contains(processed.ProtectedReceipt.SanitizedArguments, "secret") ||
		!strings.Contains(processed.ProtectedReceipt.SanitizedArguments, redactedValue) {
		t.Fatalf("protected receipt = %#v", processed.ProtectedReceipt)
	}
}

func TestStandardFailsClosedWhenProtectedOutputCannotBeMaterialized(t *testing.T) {
	content := strings.Repeat("protected mutation output", 100)
	processed, err := Standard(Policy{MaxBytes: 128}).Process(context.Background(), agent.ToolResultProcessRequest{
		ToolName: "write", Arguments: `{"path":"chapter.md"}`,
		ExecutionID: "execution-2",
		Definition:  agent.ToolDefinitionSnapshot{Descriptor: resultDescriptor(agent.ToolResultProtected)},
		Result:      agent.TextToolResult(content),
	})
	if err == nil || !agent.IsToolControlError(err) {
		t.Fatalf("error = %v, want ToolControlError", err)
	}
	if !processed.Metadata.ModelTruncated || processed.Metadata.ArtifactPersistence == nil ||
		processed.Metadata.ArtifactPersistence.FailureReason != agent.ToolArtifactFailureStoreUnavailable {
		t.Fatalf("processed failure projection = %#v", processed)
	}
}

func TestStandardDoesNotTrustRecoverableArtifactWithoutExplicitVerifier(t *testing.T) {
	storage := &memoryStorage{}
	ctx := agent.ContextWithToolArtifactStore(context.Background(), storage)
	result := agent.TextToolResult("bounded output")
	result.Artifacts = []agent.ToolArtifactRef{{
		ID: "forged", Purpose: agent.ToolArtifactPurposeCompleteModelOutput,
		ReadablePath: ".agent/artifacts/forged.log", ContentType: "text/plain", Complete: true,
	}}
	processed, err := Standard(Policy{MaxBytes: 256}).Process(ctx, agent.ToolResultProcessRequest{
		ToolName: "read", Arguments: `{"path":"chapter.md"}`, ExecutionID: "execution-forged",
		Definition: agent.ToolDefinitionSnapshot{Descriptor: resultDescriptor(agent.ToolResultDeferred)},
		Result:     result,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(processed.Artifacts) != 1 || processed.Artifacts[0].Purpose != agent.ToolArtifactPurposeAttachment ||
		processed.ContextHints == nil || processed.ContextHints.Recovery.Kind == agent.ToolResultRecoveryArtifact {
		t.Fatalf("unverified artifact became recoverable: %#v", processed)
	}
}

func resultDescriptor(retention agent.ToolResultRetentionMode) agent.ToolDescriptor {
	return agent.ToolDescriptor{
		Source: agent.ToolSourceRead, Execution: agent.ToolExecutionParallelRead,
		MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone,
		Recovery: agent.ToolRecoveryReadOnly, ResultProjection: agent.ToolResultBoundedModelContext,
		ResultRetention: retention, ResultRecoveryKind: agent.ToolResultRecoveryRead,
		Steering: agent.SteeringFinishCurrent, MaxResultBytes: 1 << 20,
	}
}
