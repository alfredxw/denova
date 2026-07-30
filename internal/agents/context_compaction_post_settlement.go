package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"denova/internal/agents/session"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

// PostSettlementContextStructuralProvider exposes a checkpoint prepared during
// model execution only after the parent operation has durably succeeded. The
// caller must submit the returned spec after actor settlement, never from
// Engine.Run, so the same binding is not recursively entered while locked.
type PostSettlementContextStructuralProvider interface {
	PostSettlementContextStructuralSpec(
		context.Context,
		OperationID,
		RunOptions,
	) (*ContextStructuralSpec, error)
}

type preparedSessionContextCompaction struct {
	Result           ContextCompactionResult
	SourceStartIndex int
	SourceEndIndex   int
}

func (c *SessionConversation) stagePreparedSessionCompaction(prepared preparedSessionContextCompaction) {
	if c == nil || !prepared.Result.Triggered {
		return
	}
	c.cycleMu.Lock()
	copy := prepared
	c.pendingCompaction = &copy
	c.cycleMu.Unlock()
}

func (c *SessionConversation) PostSettlementContextStructuralSpec(
	ctx context.Context,
	settledOperationID OperationID,
	options RunOptions,
) (*ContextStructuralSpec, error) {
	if c == nil || c.session == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.cycleMu.Lock()
	prepared := c.pendingCompaction
	c.pendingCompaction = nil
	c.cycleMu.Unlock()
	if prepared == nil || !prepared.Result.Triggered {
		return nil, nil
	}
	cursor := c.session.ContextCursor()
	preparedHash, err := postSettlementContextHash(struct {
		Result           ContextCompactionResult
		SourceStartIndex int
		SourceEndIndex   int
	}{prepared.Result, prepared.SourceStartIndex, prepared.SourceEndIndex})
	if err != nil {
		return nil, err
	}
	commandID := postSettlementContextCommandID(
		"auto-context-compaction", string(settledOperationID), c.session.ID,
		fmt.Sprint(prepared.SourceStartIndex), fmt.Sprint(prepared.SourceEndIndex), preparedHash,
	)
	recordID := postSettlementContextRecordID("cc", commandID)
	record := sessionContextCompactionRecord(recordID, c.agentKind, *prepared)
	options.SessionID = c.session.ID
	options = options.normalized(options.Workspace)
	binding, err := harnessBindingForOptions(options)
	if err != nil {
		return nil, err
	}
	bindingRef, err := runstate.BindingReference(binding)
	if err != nil {
		return nil, err
	}
	ref := runstate.ContextCompactionRef{
		Source: "session.effective_messages", Purpose: "persist an automatic bounded model-history checkpoint after turn settlement",
		Resource: c.session.ID, ExpectedRevision: fmt.Sprintf("session-context:%d", cursor.Revision),
	}
	mutation, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("encode post-settlement Session compaction mutation: %w", err)
	}
	productBinding, err := ParseRuntimeBinding(bindingRef)
	if err != nil {
		return nil, err
	}
	intentHash, err := ContextStructuralIntentHash(ContextStructuralCompact, productBinding, ref.ExpectedRevision, recordID, mutation)
	if err != nil {
		return nil, err
	}
	result := ContextStructuralResult{Compaction: prepared.Result}
	plan := ContextStructuralRestorePlan{
		Version: ContextStructuralRestorePlanVersion, Domain: ContextStructuralDomainSession,
		Action: ContextStructuralCompact, Commit: true, IntentHash: intentHash,
		RecordID: recordID, Result: result, Mutation: mutation,
	}
	operation := &postSettlementSessionCompactionOperation{
		session: c.session, expected: cursor, record: record, result: prepared.Result,
		hash: intentHash,
	}
	return &ContextStructuralSpec{
		CommandID: commandID, Action: ContextStructuralCompact,
		Ref: contextCompactionRefFromRuntime(ref), Options: options, Operation: operation, RestorePlan: &plan,
	}, nil
}

type postSettlementSessionCompactionOperation struct {
	session  *session.Session
	expected session.ContextCursor
	record   session.ContextCompaction
	result   ContextCompactionResult
	hash     string
}

func (o *postSettlementSessionCompactionOperation) Prepare(context.Context, ContextStructuralIdentity, func(Event)) (ContextStructuralIntent, error) {
	if o == nil || o.session == nil {
		return ContextStructuralIntent{}, fmt.Errorf("post-settlement session compaction is unavailable")
	}
	return ContextStructuralIntent{
		Hash: o.hash, Commit: true,
		Result: ContextStructuralResult{Compaction: o.result},
	}, nil
}

func (o *postSettlementSessionCompactionOperation) Commit(ctx context.Context, _ ContextStructuralIdentity, intent ContextStructuralIntent) (ContextStructuralReceipt, error) {
	if intent.Hash != o.hash {
		return ContextStructuralReceipt{}, fmt.Errorf("post-settlement session compaction intent changed")
	}
	record, err := o.session.AppendContextCompactionAtContext(ctx, o.expected, o.record)
	if err != nil {
		return ContextStructuralReceipt{}, err
	}
	return ContextStructuralReceipt{Revision: fmt.Sprintf("session-context:%d", record.ContextRevision)}, nil
}

func (o *postSettlementSessionCompactionOperation) Reconcile(context.Context) (ContextStructuralResult, ContextStructuralReceipt, bool, error) {
	if o == nil || o.session == nil {
		return ContextStructuralResult{}, ContextStructuralReceipt{}, false, nil
	}
	record, found := o.session.ContextCompactionByID(o.record.ID)
	if !found {
		return ContextStructuralResult{}, ContextStructuralReceipt{}, false, nil
	}
	return ContextStructuralResult{Compaction: contextCompactionResultFromSessionRecord(record)},
		ContextStructuralReceipt{Revision: fmt.Sprintf("session-context:%d", record.ContextRevision)}, true, nil
}

func sessionContextCompactionRecord(id, agentKind string, prepared preparedSessionContextCompaction) session.ContextCompaction {
	result := prepared.Result
	return session.ContextCompaction{
		ID: id, AgentKind: agentKind, Epoch: result.Epoch, Summary: result.Summary,
		SourceStartIndex: prepared.SourceStartIndex, SourceEndIndex: prepared.SourceEndIndex,
		SourceMessageCount: result.SourceMessageCount, RetainedTurns: result.RetainedTurns,
		EstimatedTokensBefore:  result.EstimatedTokensBefore,
		ObservedPromptTokens:   result.ObservedPromptTokens,
		ObservedEstimateTokens: result.ObservedEstimateTokens,
		TokensBefore:           result.TokensBefore, TokensAfter: result.TokensAfter, TargetRatio: result.TargetRatio,
		ContextWindowTokens: result.ContextWindowTokens, Strategy: result.Strategy, Threshold: result.Threshold,
		Reason: contextCompactionTriggerReason(result.TriggerReason, result.Phase), Phase: result.Phase,
		CandidateFingerprint: result.CandidateFingerprint, CandidateGeneration: result.CandidateGeneration,
	}
}

func contextCompactionResultFromSessionRecord(record session.ContextCompaction) ContextCompactionResult {
	result := ContextCompactionResult{
		Triggered: true, Phase: record.Phase, TriggerReason: record.Reason,
		EstimatedTokensBefore:  record.EstimatedTokensBefore,
		ObservedPromptTokens:   record.ObservedPromptTokens,
		ObservedEstimateTokens: record.ObservedEstimateTokens,
		TokensBefore:           record.TokensBefore, TokensAfter: record.TokensAfter,
		ContextWindowTokens: record.ContextWindowTokens, Strategy: record.Strategy, Threshold: record.Threshold,
		Epoch: record.Epoch, Summary: record.Summary, TargetRatio: record.TargetRatio,
		SourceMessageCount: record.SourceMessageCount, RetainedTurns: record.RetainedTurns,
		CandidateFingerprint: record.CandidateFingerprint, CandidateGeneration: record.CandidateGeneration,
	}
	applyContextCompactionRecovery(&result)
	return result
}

func postSettlementContextCommandID(prefix string, parts ...string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(strings.TrimSpace(prefix)))
	for _, part := range parts {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(strings.TrimSpace(part)))
	}
	return strings.TrimSpace(prefix) + "-" + hex.EncodeToString(hash.Sum(nil))
}

func postSettlementContextRecordID(prefix, commandID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(commandID)))
	return strings.TrimSpace(prefix) + "-" + hex.EncodeToString(sum[:16])
}

func postSettlementContextHash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode post-settlement context identity: %w", err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
