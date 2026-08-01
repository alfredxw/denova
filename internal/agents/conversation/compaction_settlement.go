package conversation

import (
	"context"
	"crypto/sha256"
	agentstructural "denova/internal/agents/context/structural"
	agentrun "denova/internal/agents/run"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	agentcompaction "denova/internal/agents/context/compaction"
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
		agentrun.OperationID,
		agentrun.Options,
	) (*agentstructural.Spec, error)
}

type preparedSessionContextCompaction struct {
	Result           agentcompaction.Result
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
	settledOperationID agentrun.OperationID,
	options agentrun.Options,
) (*agentstructural.Spec, error) {
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
		Result           agentcompaction.Result
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
	options = options.Normalize(options.Workspace)
	binding, err := agentrun.BindingForOptions(options)
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
	productBinding, err := agentrun.ParseRuntimeBinding(bindingRef)
	if err != nil {
		return nil, err
	}
	intentHash, err := agentstructural.IntentHash(agentstructural.Compact, productBinding, ref.ExpectedRevision, recordID, mutation)
	if err != nil {
		return nil, err
	}
	result := agentstructural.Result{Compaction: prepared.Result}
	plan := agentstructural.RestorePlan{
		Version: agentstructural.RestorePlanVersion, Domain: agentstructural.DomainSession,
		Action: agentstructural.Compact, Commit: true, IntentHash: intentHash,
		RecordID: recordID, Result: result, Mutation: mutation,
	}
	operation := &postSettlementSessionCompactionOperation{
		session: c.session, expected: cursor, record: record, result: prepared.Result,
		hash: intentHash,
	}
	return &agentstructural.Spec{
		CommandID: commandID, Action: agentstructural.Compact,
		Ref: agentrun.ContextCompactionRefFromRuntime(ref), Options: options, Operation: operation, RestorePlan: &plan,
	}, nil
}

type postSettlementSessionCompactionOperation struct {
	session  *session.Session
	expected session.ContextCursor
	record   session.ContextCompaction
	result   agentcompaction.Result
	hash     string
}

func (o *postSettlementSessionCompactionOperation) Prepare(context.Context, agentstructural.Identity, func(agentrun.Event)) (agentstructural.Intent, error) {
	if o == nil || o.session == nil {
		return agentstructural.Intent{}, fmt.Errorf("post-settlement session compaction is unavailable")
	}
	return agentstructural.Intent{
		Hash: o.hash, Commit: true,
		Result: agentstructural.Result{Compaction: o.result},
	}, nil
}

func (o *postSettlementSessionCompactionOperation) Commit(ctx context.Context, _ agentstructural.Identity, intent agentstructural.Intent) (agentstructural.Receipt, error) {
	if intent.Hash != o.hash {
		return agentstructural.Receipt{}, fmt.Errorf("post-settlement session compaction intent changed")
	}
	record, err := o.session.AppendContextCompactionAtContext(ctx, o.expected, o.record)
	if err != nil {
		return agentstructural.Receipt{}, err
	}
	return agentstructural.Receipt{Revision: fmt.Sprintf("session-context:%d", record.ContextRevision)}, nil
}

func (o *postSettlementSessionCompactionOperation) Reconcile(context.Context) (agentstructural.Result, agentstructural.Receipt, bool, error) {
	if o == nil || o.session == nil {
		return agentstructural.Result{}, agentstructural.Receipt{}, false, nil
	}
	record, found := o.session.ContextCompactionByID(o.record.ID)
	if !found {
		return agentstructural.Result{}, agentstructural.Receipt{}, false, nil
	}
	return agentstructural.Result{Compaction: contextCompactionResultFromSessionRecord(record)},
		agentstructural.Receipt{Revision: fmt.Sprintf("session-context:%d", record.ContextRevision)}, true, nil
}

func sessionContextCompactionRecord(id, agentKind string, prepared preparedSessionContextCompaction) session.ContextCompaction {
	result := prepared.Result
	result.TriggerReason = agentcompaction.NormalizeTriggerReason(result.TriggerReason, result.Phase)
	return session.ContextCompaction{
		ID: id, CompactionCheckpoint: agentcompaction.NewCheckpoint(agentKind, result),
		SourceStartIndex: prepared.SourceStartIndex, SourceEndIndex: prepared.SourceEndIndex,
		SourceMessageCount: result.SourceMessageCount,
	}
}

func contextCompactionResultFromSessionRecord(record session.ContextCompaction) agentcompaction.Result {
	result := agentcompaction.ResultFromCheckpoint(record.CompactionCheckpoint)
	result.SourceMessageCount = record.SourceMessageCount
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
