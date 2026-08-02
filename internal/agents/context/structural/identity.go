package structural

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	agentrun "denova/internal/agents/run"
)

// CommandID derives a deterministic durable command identity from a semantic
// prefix and its ordered identity parts.
func CommandID(prefix string, parts ...string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(strings.TrimSpace(prefix)))
	for _, part := range parts {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(strings.TrimSpace(part)))
	}
	return strings.TrimSpace(prefix) + "-" + hex.EncodeToString(hash.Sum(nil))
}

// RecordID derives the bounded canonical-store identity owned by a durable
// structural command.
func RecordID(prefix, commandID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(commandID)))
	return strings.TrimSpace(prefix) + "-" + hex.EncodeToString(sum[:16])
}

// ValueHash returns the canonical JSON identity used to fence a prepared
// structural mutation from a different payload with the same command.
func ValueHash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode structural context identity: %w", err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// StoryRevision encodes the canonical branch head used by a structural CAS.
func StoryRevision(head string) string {
	head = strings.TrimSpace(head)
	if head == "" {
		head = "root"
	}
	return "story-head:" + head
}

// NewRestorePlan freezes the exact mutation authorized for durable recovery.
func NewRestorePlan(
	domain Domain,
	action Action,
	binding agentrun.RuntimeBinding,
	ref agentrun.ContextCompactionRef,
	recordID string,
	result Result,
	mutation any,
) (RestorePlan, error) {
	encoded, err := json.Marshal(mutation)
	if err != nil {
		return RestorePlan{}, fmt.Errorf("encode structural context mutation: %w", err)
	}
	hash, err := IntentHash(action, binding, ref.ExpectedRevision, recordID, encoded)
	if err != nil {
		return RestorePlan{}, err
	}
	return RestorePlan{
		Version: RestorePlanVersion, Domain: domain, Action: action,
		Commit: true, IntentHash: hash, RecordID: recordID, Result: result, Mutation: encoded,
	}, nil
}

// FixedOperation rebuilds an already prepared durable operation without
// invoking the model or re-reading mutable caller input.
func FixedOperation(
	plan RestorePlan,
	commit func(context.Context) (Receipt, error),
	reconcile func(context.Context) (Receipt, bool, error),
) Operation {
	return fixedOperation{plan: plan, commit: commit, reconcile: reconcile}
}

type fixedOperation struct {
	plan      RestorePlan
	commit    func(context.Context) (Receipt, error)
	reconcile func(context.Context) (Receipt, bool, error)
}

func (o fixedOperation) Prepare(ctx context.Context, _ Identity, _ func(agentrun.Event)) (Intent, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return Intent{Result: o.plan.Result}, err
		}
	}
	return Intent{Hash: o.plan.IntentHash, Commit: o.plan.Commit, Result: o.plan.Result}, nil
}

func (o fixedOperation) Commit(ctx context.Context, _ Identity, intent Intent) (Receipt, error) {
	if intent.Hash != o.plan.IntentHash || intent.Commit != o.plan.Commit || !reflect.DeepEqual(intent.Result, o.plan.Result) {
		return Receipt{}, fmt.Errorf("structural context intent changed before frozen commit")
	}
	if !o.plan.Commit {
		return Receipt{}, fmt.Errorf("non-committing structural plan reached canonical commit")
	}
	return o.commit(ctx)
}

func (o fixedOperation) Reconcile(ctx context.Context) (Result, Receipt, bool, error) {
	if !o.plan.Commit {
		return o.plan.Result, Receipt{}, false, nil
	}
	receipt, found, err := o.reconcile(ctx)
	return o.plan.Result, receipt, found, err
}
