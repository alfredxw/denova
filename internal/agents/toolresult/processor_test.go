package toolresult

import (
	"context"
	"errors"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestProcessToolResultMaterializesOversizedText(t *testing.T) {
	store := &processorArtifactStore{}
	ctx := agent.ContextWithToolArtifactStore(context.Background(), store)
	content := "HEAD-SENTINEL\n" + strings.Repeat("middle payload ", 80) + "\nTAIL-SENTINEL"
	decision := processorTestDecision(agent.ToolResultEagerCandidate)

	processed, err := Process(ctx, decision, `{"path":"chapter.md"}`, agent.TextToolResult(content), processorTestProcessingPolicy(256))
	if err != nil {
		t.Fatal(err)
	}
	if store.request.ToolCallID != decision.ProviderCallID || store.content.String() != content {
		t.Fatalf("materialized request=%#v content=%q", store.request, store.content.String())
	}
	if len(processed.ModelContent) > 256 || !strings.Contains(processed.ModelContent, "HEAD-SENTINEL") ||
		!strings.Contains(processed.ModelContent, "TAIL-SENTINEL") || !strings.Contains(processed.ModelContent, "original_bytes=") {
		t.Fatalf("inline preview is not a bounded head/tail projection: %q", processed.ModelContent)
	}
	if len(processed.Artifacts) != 1 || !processed.Artifacts[0].Complete ||
		processed.Artifacts[0].Purpose != agent.ToolArtifactPurposeCompleteModelOutput ||
		processed.Artifacts[0].ReadablePath != ".denova/artifacts/session/call-42.log" {
		t.Fatalf("artifact ref = %#v", processed.Artifacts)
	}
	if processed.ContextHints == nil || processed.ContextHints.Recovery.Kind != agent.ToolResultRecoveryArtifact ||
		processed.ContextHints.Recovery.ArtifactPath != processed.Artifacts[0].ReadablePath {
		t.Fatalf("artifact recovery hint = %#v", processed.ContextHints)
	}
	persistence := processed.Metadata.ArtifactPersistence
	if persistence == nil || !persistence.Attempted || !persistence.Complete || persistence.FailureReason != "" {
		t.Fatalf("artifact persistence = %#v", persistence)
	}
}

func TestProcessToolResultMaterializesOutputInsteadOfReusingAttachment(t *testing.T) {
	store := &processorArtifactStore{}
	ctx := agent.ContextWithToolArtifactStore(context.Background(), store)
	content := strings.Repeat("complete model output ", 100)
	result := agent.TextToolResult(content)
	result.Artifacts = []agent.ToolArtifactRef{{
		ID: "attachment-1", Purpose: agent.ToolArtifactPurposeAttachment,
		ReadablePath: ".denova/attachments/chart.png", ContentType: "image/png",
		EstimatedBytes: 2048, EstimatedTokens: 512, Complete: true,
	}}

	processed, err := Process(
		ctx, processorTestDecision(agent.ToolResultDeferred), `{"path":"chapter.md"}`,
		result, processorTestProcessingPolicy(256),
	)
	if err != nil {
		t.Fatal(err)
	}
	if store.beginCalls != 1 || store.content.String() != content || len(processed.Artifacts) != 2 {
		t.Fatalf("attachment bypassed output materialization: calls=%d artifacts=%#v", store.beginCalls, processed.Artifacts)
	}
	var outputArtifact *agent.ToolArtifactRef
	for index := range processed.Artifacts {
		if processed.Artifacts[index].Purpose == agent.ToolArtifactPurposeCompleteModelOutput {
			outputArtifact = &processed.Artifacts[index]
		}
	}
	if outputArtifact == nil || processed.ContextHints == nil ||
		processed.ContextHints.Recovery.Kind != agent.ToolResultRecoveryArtifact ||
		processed.ContextHints.Recovery.ArtifactPath != outputArtifact.ReadablePath {
		t.Fatalf("materialized output recovery = %#v artifacts=%#v", processed.ContextHints, processed.Artifacts)
	}
}

func TestProcessToolResultReusesOnlyExplicitCompleteModelOutput(t *testing.T) {
	store := &processorArtifactStore{beginErr: errors.New("must not materialize"), verified: true}
	ctx := agent.ContextWithToolArtifactStore(context.Background(), store)
	content := strings.Repeat("already persisted output ", 100)
	result := agent.TextToolResult(content)
	result.Artifacts = []agent.ToolArtifactRef{{
		ID: "output-1", Purpose: agent.ToolArtifactPurposeCompleteModelOutput,
		ReadablePath: ".denova/artifacts/session/output-1.log", ContentType: "text/plain",
		EstimatedBytes: int64(len(content)), EstimatedTokens: EstimatedTokens(int64(len(content))), Complete: true,
	}}

	processed, err := Process(
		ctx, processorTestDecision(agent.ToolResultDeferred), `{"path":"chapter.md"}`,
		result, processorTestProcessingPolicy(256),
	)
	if err != nil {
		t.Fatal(err)
	}
	if store.beginCalls != 0 || processed.Metadata.ArtifactPersistence == nil ||
		!processed.Metadata.ArtifactPersistence.Complete || processed.ContextHints == nil ||
		processed.ContextHints.Recovery.ArtifactPath != result.Artifacts[0].ReadablePath {
		t.Fatalf("complete output artifact was not reused: calls=%d result=%#v", store.beginCalls, processed)
	}
}

func TestProcessToolResultMaterializesInsteadOfTrustingUnverifiedCompleteOutput(t *testing.T) {
	store := &processorArtifactStore{}
	ctx := agent.ContextWithToolArtifactStore(context.Background(), store)
	content := strings.Repeat("must remain recoverable ", 100)
	result := agent.TextToolResult(content)
	result.Artifacts = []agent.ToolArtifactRef{{
		ID: "forged", Purpose: agent.ToolArtifactPurposeCompleteModelOutput,
		ReadablePath: ".denova/artifacts/session/forged.log", ContentType: "text/plain",
		EstimatedBytes: int64(len(content)), Complete: true,
	}}

	processed, err := Process(
		ctx, processorTestDecision(agent.ToolResultDeferred), `{"path":"chapter.md"}`,
		result, processorTestProcessingPolicy(256),
	)
	if err != nil {
		t.Fatal(err)
	}
	if store.beginCalls != 1 || store.content.String() != content || len(processed.Artifacts) != 2 ||
		processed.Artifacts[0].Purpose != agent.ToolArtifactPurposeAttachment ||
		processed.Artifacts[1].Purpose != agent.ToolArtifactPurposeCompleteModelOutput {
		t.Fatalf("unverified recovery claim was trusted: calls=%d artifacts=%#v", store.beginCalls, processed.Artifacts)
	}
}

func TestCompleteToolResultArtifactRejectsPurposeLessReferences(t *testing.T) {
	references := []agent.ToolArtifactRef{{
		ID: "artifact-1", ReadablePath: ".denova/artifacts/session/attachment.txt",
		ContentType: "text/plain", EstimatedBytes: 1024, Complete: true,
	}, {
		ID:           "call-0123456789abcdef0123456789abcdef",
		ReadablePath: ".denova/artifacts/scope-0123456789abcdef0123456789abcdef/call-0123456789abcdef0123456789abcdef.log",
		ContentType:  "text/plain", EstimatedBytes: 1024, Complete: true,
		SHA256: strings.Repeat("a", 64),
	}}
	for _, reference := range references {
		if artifact := recoverableToolResultArtifact([]agent.ToolArtifactRef{reference}); artifact != nil {
			t.Fatalf("purpose-less reference authorized cleanup: %#v", artifact)
		}
	}
}

func TestMaterializeToolResultRejectsStorePurposeMismatch(t *testing.T) {
	store := &processorArtifactStore{returnedPurpose: agent.ToolArtifactPurposeAttachment}
	ctx := agent.ContextWithToolArtifactStore(context.Background(), store)
	artifact, failure := materializeToolResult(ctx, processorTestDecision(agent.ToolResultDeferred), "complete output")
	if artifact != nil || failure != agent.ToolArtifactFailureCommit {
		t.Fatalf("purpose mismatch was accepted: artifact=%#v failure=%q", artifact, failure)
	}
}

func TestProcessToolResultAddsBoundedEagerRetentionNotice(t *testing.T) {
	content := strings.Repeat("large recoverable page ", 8_000)
	processed, err := Process(
		context.Background(), processorTestDecision(agent.ToolResultEagerCandidate),
		`{"path":"page.md"}`, agent.TextToolResult(content), processorTestProcessingPolicy(len(content)+1024),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(processed.ModelContent, "[Context retention notice]") ||
		!strings.Contains(processed.ModelContent, "Do not copy the entire result") {
		t.Fatalf("eager retention notice missing: %q", processed.ModelContent[len(processed.ModelContent)-512:])
	}
	if len(processed.ModelContent) > len(content)+1024 {
		t.Fatalf("eager result exceeded inline budget: got=%d limit=%d", len(processed.ModelContent), len(content)+1024)
	}
}

func TestProcessToolResultDoesNotWarnForSmallEagerCandidate(t *testing.T) {
	processed, err := Process(
		context.Background(), processorTestDecision(agent.ToolResultEagerCandidate),
		`{"path":"page.md"}`, agent.TextToolResult("small recoverable result"), processorTestProcessingPolicy(1024),
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(processed.ModelContent, "[Context retention notice]") {
		t.Fatalf("small result should remain ordinary rich context: %q", processed.ModelContent)
	}
}

func TestEagerRetentionNoticeUsesThePlannerThreshold(t *testing.T) {
	content := strings.Repeat("recoverable ", 100)
	lowThreshold := ProcessingPolicy{MaxBytes: len(content) + 1024, EagerMinTokens: 10, ContextWindowTokens: 100}
	processed, err := Process(
		context.Background(), processorTestDecision(agent.ToolResultEagerCandidate),
		`{"path":"page.md"}`, agent.TextToolResult(content), lowThreshold,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(processed.ModelContent, "[Context retention notice]") {
		t.Fatal("configured eager threshold did not produce the settlement notice")
	}

	largeWindow := lowThreshold
	largeWindow.ContextWindowTokens = 1_000_000
	processed, err = Process(
		context.Background(), processorTestDecision(agent.ToolResultEagerCandidate),
		`{"path":"page.md"}`, agent.TextToolResult(content), largeWindow,
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(processed.ModelContent, "[Context retention notice]") {
		t.Fatal("result below the planner's context-ratio threshold received a misleading notice")
	}
}

func TestProcessToolResultFailsBoundedWithoutPublishingFakeArtifact(t *testing.T) {
	store := &processorArtifactStore{beginErr: errors.New("credential=must-not-leak")}
	ctx := agent.ContextWithToolArtifactStore(context.Background(), store)
	content := strings.Repeat("recoverable read output ", 100)

	processed, err := Process(ctx, processorTestDecision(agent.ToolResultDeferred), `{"path":"chapter.md"}`, agent.TextToolResult(content), processorTestProcessingPolicy(192))
	if err != nil {
		t.Fatal(err)
	}
	if len(processed.ModelContent) > 192 || !strings.Contains(processed.ModelContent, "complete output unavailable") {
		t.Fatalf("failure preview = %q", processed.ModelContent)
	}
	if len(processed.Artifacts) != 0 || strings.Contains(processed.ModelContent, "credential") {
		t.Fatalf("spill failure exposed an invalid artifact or raw error: %#v", processed)
	}
	persistence := processed.Metadata.ArtifactPersistence
	if persistence == nil || !persistence.Attempted || persistence.Complete || persistence.FailureReason != agent.ToolArtifactFailureBegin {
		t.Fatalf("artifact persistence = %#v", persistence)
	}
}

func TestProcessToolResultRejectsLossyProtectedSpill(t *testing.T) {
	store := &processorArtifactStore{beginErr: errors.New("disk full")}
	ctx := agent.ContextWithToolArtifactStore(context.Background(), store)
	content := strings.Repeat("irreplaceable evidence ", 100)
	decision := processorTestDecision(agent.ToolResultProtected)

	processed, err := Process(ctx, decision, `{"path":"evidence.log"}`, agent.TextToolResult(content), processorTestProcessingPolicy(192))
	if !agent.IsToolControlError(err) || !strings.Contains(err.Error(), "persist complete protected tool result") {
		t.Fatalf("protected spill error = %v", err)
	}
	if len(processed.ModelContent) > 192 || len(processed.Artifacts) != 0 ||
		processed.Metadata.ArtifactPersistence == nil || processed.Metadata.ArtifactPersistence.Complete {
		t.Fatalf("protected failure was not bounded: %#v", processed)
	}
}

func TestProcessToolResultRejectsLossyStreamingShellArtifactFailure(t *testing.T) {
	result := agent.ToolResult{
		ModelContent:   `{"schema":"process.result.v1","output_truncated":true,"artifact_error":"write_failed"}\nbounded head/tail preview`,
		DisplayContent: `{"schema":"process.result.v1","output_truncated":true,"artifact_error":"write_failed"}\nbounded head/tail preview`,
		Details:        []byte(`{"schema":"process.result.v1","output_truncated":true,"artifact_error":"write_failed"}`),
		Status:         agent.ToolResultSuccess,
		Metadata: agent.ToolResultMetadata{
			OriginalModelBytes: 64 * 1024, ModelTruncated: true,
			ArtifactPersistence: &agent.ToolArtifactPersistence{
				Attempted: true, Complete: false, FailureReason: agent.ToolArtifactFailureWrite,
			},
		},
	}

	processed, err := Process(
		context.Background(), processorShellTestDecision(), `{"command":"produce output"}`,
		result, processorTestProcessingPolicy(256),
	)
	if !agent.IsToolControlError(err) || !strings.Contains(err.Error(), agent.ToolArtifactFailureWrite) {
		t.Fatalf("lossy shell processing error = %T %v", err, err)
	}
	persistence := processed.Metadata.ArtifactPersistence
	if len(processed.ModelContent) > 256 || !processed.Metadata.ModelTruncated ||
		processed.Metadata.OriginalModelBytes != 64*1024 || len(processed.Artifacts) != 0 ||
		persistence == nil || persistence.Complete || persistence.FailureReason != agent.ToolArtifactFailureWrite {
		t.Fatalf("lossy shell result = %#v", processed)
	}
}

func TestProcessToolResultAllowsCompleteInlineShellAfterArtifactFailure(t *testing.T) {
	result := agent.TextToolResult("complete inline output")
	result.Metadata.ArtifactPersistence = &agent.ToolArtifactPersistence{
		Attempted: true, Complete: false, FailureReason: agent.ToolArtifactFailureCommit,
	}

	processed, err := Process(
		context.Background(), processorShellTestDecision(), `{"command":"small output"}`,
		result, processorTestProcessingPolicy(256),
	)
	if err != nil {
		t.Fatalf("complete inline shell must remain usable: %v", err)
	}
	if processed.ModelContent != result.ModelContent || processed.Metadata.ModelTruncated ||
		processed.Metadata.ArtifactPersistence == nil || processed.Metadata.ArtifactPersistence.Complete {
		t.Fatalf("complete inline shell result = %#v", processed)
	}
}

func TestProcessToolResultGeneratesAndNormalizesReadRecoveryHint(t *testing.T) {
	decision := processorTestDecision(agent.ToolResultDeferred)
	processed, err := Process(
		context.Background(), decision,
		`{"path":"chapter.md","authorization":"Bearer do-not-persist"}`,
		agent.TextToolResult("small recoverable result"), processorTestProcessingPolicy(1024),
	)
	if err != nil {
		t.Fatal(err)
	}
	if processed.ContextHints == nil || processed.ContextHints.Recovery.Kind != agent.ToolResultRecoveryRead ||
		processed.ContextHints.Recovery.Reference["path"] != "chapter.md" ||
		processed.ContextHints.Recovery.Reference["authorization"] != "[REDACTED]" ||
		processed.ContextHints.SupersessionKey == "" {
		t.Fatalf("read recovery hints = %#v", processed.ContextHints)
	}
	if strings.Contains(processed.ContextHints.SupersessionKey, "do-not-persist") {
		t.Fatalf("supersession key leaked arguments: %q", processed.ContextHints.SupersessionKey)
	}
}

func processorTestDecision(retention agent.ToolResultRetentionMode) Call {
	return Call{
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

func processorShellTestDecision() Call {
	return Call{
		ToolName: "bash", ProviderCallID: "call-shell", ExecutionID: "exec-shell",
		Descriptor: agent.ToolDescriptor{
			Source: agent.ToolSourceShell, Execution: agent.ToolExecutionWorkspaceExclusive,
			MutationScope: agent.ToolMutationExternal, PostCheck: agent.ToolPostCheckExternalReceipt,
			Recovery: agent.ToolRecoveryNonIdempotent, ResultProjection: agent.ToolResultBoundedModelContext,
			ResultRetention: agent.ToolResultProtected, Steering: agent.SteeringFinishCurrent, MaxResultBytes: 1024,
		},
	}
}

func processorTestProcessingPolicy(maxBytes int) ProcessingPolicy {
	return ProcessingPolicy{MaxBytes: maxBytes, EagerMinTokens: 32_000, ContextWindowTokens: 160_000}
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
