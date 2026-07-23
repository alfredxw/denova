package automation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	TriggerEvaluationStatusClaimed   = "claimed"
	TriggerEvaluationStatusDecided   = "decided"
	TriggerEvaluationStatusCompleted = "completed"

	// These limits bind the durable semantic intent independently from display
	// history. Instruction is deliberately above 128 KiB because it contains
	// the bounded evidence payload injected into the evaluator.
	MaxSemanticTriggerWorkspaceChars   = 16 * 1024
	MaxSemanticTriggerTaskIDChars      = 512
	MaxSemanticTriggerIDChars          = 512
	MaxSemanticTriggerConditionChars   = 16 * 1024
	MaxSemanticTriggerObservationChars = 512
	MaxSemanticTriggerInstructionChars = 384 * 1024
)

// ErrTriggerEvaluationConflict rejects reuse of a deterministic evaluation
// identity with different bounded input or a different canonical decision.
var ErrTriggerEvaluationConflict = errors.New("automation trigger evaluation conflict")

// ErrTriggerActionConflict rejects reuse of a deterministic side-effect
// identity for a different inbox or run intent.
var ErrTriggerActionConflict = errors.New("automation trigger action conflict")

type TriggerEvaluationClaimDisposition string

const (
	// TriggerEvaluationClaimed means this call durably wrote the observation.
	TriggerEvaluationClaimed TriggerEvaluationClaimDisposition = "claimed"
	// TriggerEvaluationResumed means an incomplete, semantically identical
	// record survived an earlier caller or process.
	TriggerEvaluationResumed TriggerEvaluationClaimDisposition = "resumed"
	// TriggerEvaluationReplayed means the same semantic observation already
	// reached its durable completion barrier.
	TriggerEvaluationReplayed TriggerEvaluationClaimDisposition = "replayed"
)

// SemanticTriggerIntent is the bounded input whose meaning must remain stable
// across evaluator retries. Workspace is part of the identity even for a
// user-scoped task because content triggers are evaluated per workspace.
type SemanticTriggerIntent struct {
	Scope                  string
	Workspace              string
	TaskID                 string
	TriggerID              string
	Condition              string
	ObservationFingerprint string
	Instruction            string
	Context                TriggerContext
	ActionPolicy           string
	NotifyPolicy           string
}

// MatchedTriggerIntent is the complete, bounded observation produced by a
// deterministic trigger evaluator such as schedule or chapter-batch. Match is
// persisted while the record is claimed, so a crash between evaluation and
// action planning never requires re-reading mutable workspace state.
type MatchedTriggerIntent struct {
	Scope        string
	Workspace    string
	TaskID       string
	TriggerID    string
	TriggerType  string
	Match        TriggerMatch
	ActionPolicy string
	NotifyPolicy string
}

// TriggerEvaluationRecord is the persisted semantic evaluation state machine.
// Claimed records may call the model; decided records may reconcile only the
// preplanned action; completed records are exact-replay receipts.
type TriggerEvaluationRecord struct {
	ID                     string              `json:"id"`
	IntentHash             string              `json:"intent_hash"`
	Status                 string              `json:"status"`
	Scope                  string              `json:"scope"`
	Workspace              string              `json:"workspace,omitempty"`
	TaskID                 string              `json:"task_id"`
	TriggerID              string              `json:"trigger_id"`
	TriggerType            string              `json:"trigger_type"`
	Condition              string              `json:"condition"`
	ObservationFingerprint string              `json:"observation_fingerprint"`
	Instruction            string              `json:"instruction"`
	Context                TriggerContext      `json:"context"`
	ActionPolicy           string              `json:"action_policy"`
	NotifyPolicy           string              `json:"notify_policy"`
	Decision               *SemanticEvaluation `json:"decision,omitempty"`
	DecisionHash           string              `json:"decision_hash,omitempty"`
	CandidateMatch         *TriggerMatch       `json:"candidate_match,omitempty"`
	Match                  *TriggerMatch       `json:"match,omitempty"`
	Action                 *TriggerActionPlan  `json:"action,omitempty"`
	ClaimedAt              time.Time           `json:"claimed_at"`
	DecidedAt              time.Time           `json:"decided_at,omitempty"`
	CompletedAt            time.Time           `json:"completed_at,omitempty"`
	UpdatedAt              time.Time           `json:"updated_at"`
}

// TriggerActionPlan names every external effect before any of them happens.
// Empty InboxID means the notification policy is silent; empty RunID means the
// action does not auto-run.
type TriggerActionPlan struct {
	ID           string `json:"id"`
	ActionPolicy string `json:"action_policy"`
	NotifyPolicy string `json:"notify_policy"`
	InboxID      string `json:"inbox_id,omitempty"`
	RunID        string `json:"run_id,omitempty"`
}

// NewSemanticTriggerEvaluation normalizes and hashes one bounded observation.
// The evaluation ID describes what was observed; IntentHash additionally binds
// the exact prompt/context and action policy, enabling semantic conflict checks.
func NewSemanticTriggerEvaluation(intent SemanticTriggerIntent, now time.Time) (TriggerEvaluationRecord, error) {
	intent.Scope = strings.TrimSpace(intent.Scope)
	if intent.Scope != ScopeUser && intent.Scope != ScopeWorkspace {
		return TriggerEvaluationRecord{}, fmt.Errorf("invalid semantic trigger scope %q", intent.Scope)
	}
	intent.Workspace = strings.TrimSpace(intent.Workspace)
	intent.TaskID = strings.TrimSpace(intent.TaskID)
	intent.TriggerID = strings.TrimSpace(intent.TriggerID)
	intent.Condition = strings.TrimSpace(intent.Condition)
	intent.ObservationFingerprint = strings.TrimSpace(intent.ObservationFingerprint)
	intent.Instruction = strings.TrimSpace(intent.Instruction)
	if intent.TaskID == "" || intent.TriggerID == "" || intent.Condition == "" || intent.ObservationFingerprint == "" || intent.Instruction == "" {
		return TriggerEvaluationRecord{}, fmt.Errorf("semantic trigger identity, condition, observation, and instruction are required")
	}
	for _, bounded := range []struct {
		name  string
		value string
		limit int
	}{
		{"workspace", intent.Workspace, MaxSemanticTriggerWorkspaceChars},
		{"task id", intent.TaskID, MaxSemanticTriggerTaskIDChars},
		{"trigger id", intent.TriggerID, MaxSemanticTriggerIDChars},
		{"condition", intent.Condition, MaxSemanticTriggerConditionChars},
		{"observation fingerprint", intent.ObservationFingerprint, MaxSemanticTriggerObservationChars},
		{"instruction", intent.Instruction, MaxSemanticTriggerInstructionChars},
	} {
		if len(bounded.value) > bounded.limit*utf8.UTFMax || utf8.RuneCountInString(bounded.value) > bounded.limit {
			return TriggerEvaluationRecord{}, fmt.Errorf("semantic trigger %s exceeds %d characters", bounded.name, bounded.limit)
		}
	}
	intent.Context = BoundedTriggerContext(intent.Context)
	intent.ActionPolicy = normalizeActionPolicy(intent.ActionPolicy, ActionPolicyConfirm)
	intent.NotifyPolicy = normalizeNotifyPolicy(intent.NotifyPolicy, NotifyPolicyInbox)
	if intent.ActionPolicy == ActionPolicyConfirm {
		intent.NotifyPolicy = NotifyPolicyInbox
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	canonicalWorkspace := canonicalStoreRoot(intent.Workspace)
	identity := struct {
		Scope       string `json:"scope"`
		Workspace   string `json:"workspace,omitempty"`
		TaskID      string `json:"task_id"`
		TriggerID   string `json:"trigger_id"`
		Condition   string `json:"condition"`
		Observation string `json:"observation"`
	}{intent.Scope, canonicalWorkspace, intent.TaskID, intent.TriggerID, intent.Condition, intent.ObservationFingerprint}
	semantic := struct {
		Identity     any            `json:"identity"`
		Instruction  string         `json:"instruction"`
		Context      TriggerContext `json:"context"`
		ActionPolicy string         `json:"action_policy"`
		NotifyPolicy string         `json:"notify_policy"`
	}{identity, intent.Instruction, intent.Context, intent.ActionPolicy, intent.NotifyPolicy}
	return TriggerEvaluationRecord{
		ID:                     deterministicTriggerID("trigger-eval", identity),
		IntentHash:             deterministicTriggerHash(semantic),
		Status:                 TriggerEvaluationStatusClaimed,
		Scope:                  intent.Scope,
		Workspace:              intent.Workspace,
		TaskID:                 intent.TaskID,
		TriggerID:              intent.TriggerID,
		TriggerType:            TriggerTypeSemantic,
		Condition:              intent.Condition,
		ObservationFingerprint: intent.ObservationFingerprint,
		Instruction:            intent.Instruction,
		Context:                intent.Context,
		ActionPolicy:           intent.ActionPolicy,
		NotifyPolicy:           intent.NotifyPolicy,
		ClaimedAt:              now,
		UpdatedAt:              now,
	}, nil
}

// NewMatchedTriggerEvaluation durably names one deterministic schedule or
// chapter observation. The candidate match is part of the intent hash and is
// stored before action identities are decided.
func NewMatchedTriggerEvaluation(intent MatchedTriggerIntent, now time.Time) (TriggerEvaluationRecord, error) {
	intent.Scope = strings.TrimSpace(intent.Scope)
	intent.Workspace = strings.TrimSpace(intent.Workspace)
	intent.TaskID = strings.TrimSpace(intent.TaskID)
	intent.TriggerID = strings.TrimSpace(intent.TriggerID)
	intent.TriggerType = strings.TrimSpace(intent.TriggerType)
	intent.Match.TaskID = strings.TrimSpace(intent.Match.TaskID)
	intent.Match.TriggerID = strings.TrimSpace(intent.Match.TriggerID)
	intent.Match.Title = strings.TrimSpace(intent.Match.Title)
	intent.Match.Summary = strings.TrimSpace(intent.Match.Summary)
	intent.Match.Fingerprint = strings.TrimSpace(intent.Match.Fingerprint)
	if intent.Scope != ScopeUser && intent.Scope != ScopeWorkspace {
		return TriggerEvaluationRecord{}, fmt.Errorf("invalid trigger scope %q", intent.Scope)
	}
	if intent.TriggerType != TriggerTypeSchedule && intent.TriggerType != TriggerTypeChapterBatch {
		return TriggerEvaluationRecord{}, fmt.Errorf("unsupported deterministic trigger type %q", intent.TriggerType)
	}
	if intent.TaskID == "" || intent.TriggerID == "" || intent.Match.Fingerprint == "" {
		return TriggerEvaluationRecord{}, fmt.Errorf("trigger identity and match fingerprint are required")
	}
	if intent.Match.TaskID != intent.TaskID || intent.Match.TriggerID != intent.TriggerID {
		return TriggerEvaluationRecord{}, fmt.Errorf("trigger match belongs to a different task or trigger")
	}
	for _, bounded := range []struct {
		name  string
		value string
		limit int
	}{
		{"workspace", intent.Workspace, MaxSemanticTriggerWorkspaceChars},
		{"task id", intent.TaskID, MaxSemanticTriggerTaskIDChars},
		{"trigger id", intent.TriggerID, MaxSemanticTriggerIDChars},
		{"match title", intent.Match.Title, MaxSemanticEvaluationTitleChars},
		{"match summary", intent.Match.Summary, MaxSemanticEvaluationReasonChars},
		{"match fingerprint", intent.Match.Fingerprint, MaxSemanticTriggerObservationChars},
	} {
		if len(bounded.value) > bounded.limit*utf8.UTFMax || utf8.RuneCountInString(bounded.value) > bounded.limit {
			return TriggerEvaluationRecord{}, fmt.Errorf("trigger %s exceeds %d characters", bounded.name, bounded.limit)
		}
	}
	context := BoundedTriggerContext(TriggerContext{
		Source: intent.TriggerType, Summary: intent.Match.Summary, Evidence: intent.Match.Evidence,
	})
	intent.Match.Evidence = context.Evidence
	intent.ActionPolicy = normalizeActionPolicy(intent.ActionPolicy, ActionPolicyConfirm)
	intent.NotifyPolicy = normalizeNotifyPolicy(intent.NotifyPolicy, NotifyPolicyInbox)
	if intent.ActionPolicy == ActionPolicyConfirm {
		intent.NotifyPolicy = NotifyPolicyInbox
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	identity := struct {
		Scope       string `json:"scope"`
		Workspace   string `json:"workspace,omitempty"`
		TaskID      string `json:"task_id"`
		TriggerID   string `json:"trigger_id"`
		TriggerType string `json:"trigger_type"`
		Observation string `json:"observation"`
	}{intent.Scope, canonicalStoreRoot(intent.Workspace), intent.TaskID, intent.TriggerID, intent.TriggerType, intent.Match.Fingerprint}
	semantic := struct {
		Identity     any          `json:"identity"`
		Match        TriggerMatch `json:"match"`
		ActionPolicy string       `json:"action_policy"`
		NotifyPolicy string       `json:"notify_policy"`
	}{identity, intent.Match, intent.ActionPolicy, intent.NotifyPolicy}
	candidate := intent.Match
	return TriggerEvaluationRecord{
		ID:                     deterministicTriggerID("trigger-eval", identity),
		IntentHash:             deterministicTriggerHash(semantic),
		Status:                 TriggerEvaluationStatusClaimed,
		Scope:                  intent.Scope,
		Workspace:              intent.Workspace,
		TaskID:                 intent.TaskID,
		TriggerID:              intent.TriggerID,
		TriggerType:            intent.TriggerType,
		ObservationFingerprint: intent.Match.Fingerprint,
		Context:                context,
		ActionPolicy:           intent.ActionPolicy,
		NotifyPolicy:           intent.NotifyPolicy,
		CandidateMatch:         &candidate,
		ClaimedAt:              now,
		UpdatedAt:              now,
	}, nil
}

// DeterministicTriggerDecision converts a persisted non-model candidate into
// the same canonical decision shape used by the shared action planner.
func DeterministicTriggerDecision(record TriggerEvaluationRecord) (SemanticEvaluation, TriggerMatch, error) {
	if record.TriggerType != TriggerTypeSchedule && record.TriggerType != TriggerTypeChapterBatch {
		return SemanticEvaluation{}, TriggerMatch{}, fmt.Errorf("evaluation %s is not a deterministic trigger", record.ID)
	}
	if record.CandidateMatch == nil {
		return SemanticEvaluation{}, TriggerMatch{}, fmt.Errorf("evaluation %s has no persisted candidate match", record.ID)
	}
	match := *record.CandidateMatch
	refs := make([]string, 0, len(match.Evidence))
	for _, evidence := range match.Evidence {
		if ref := strings.TrimSpace(evidence.Ref); ref != "" {
			refs = append(refs, ref)
		} else if title := strings.TrimSpace(evidence.Title); title != "" {
			refs = append(refs, title)
		}
	}
	decision, err := ValidateSemanticEvaluationEvidence(SemanticEvaluation{
		Matched: true, Confidence: 1, Reason: match.Summary, Title: match.Title, EvidenceRefs: refs,
	}, record.Context)
	return decision, match, err
}

func NewTriggerActionPlan(record TriggerEvaluationRecord, match TriggerMatch) (TriggerActionPlan, error) {
	if strings.TrimSpace(record.ID) == "" || strings.TrimSpace(record.IntentHash) == "" || strings.TrimSpace(match.Fingerprint) == "" {
		return TriggerActionPlan{}, fmt.Errorf("evaluation identity and match fingerprint are required")
	}
	plan := TriggerActionPlan{
		ID:           deterministicTriggerID("trigger-action", record.ID, record.IntentHash, match.Fingerprint),
		ActionPolicy: normalizeActionPolicy(record.ActionPolicy, ActionPolicyConfirm),
		NotifyPolicy: normalizeNotifyPolicy(record.NotifyPolicy, NotifyPolicyInbox),
	}
	if plan.ActionPolicy == ActionPolicyConfirm {
		plan.NotifyPolicy = NotifyPolicyInbox
	}
	if plan.NotifyPolicy == NotifyPolicyInbox || plan.ActionPolicy == ActionPolicyConfirm {
		plan.InboxID = deterministicTriggerID("inbox", plan.ID)
	}
	if plan.ActionPolicy == ActionPolicyAutoRun {
		plan.RunID = deterministicTriggerID("run", plan.ID)
	}
	return plan, nil
}

func triggerEvaluationDecisionHash(decision SemanticEvaluation, match *TriggerMatch, action *TriggerActionPlan) string {
	return deterministicTriggerHash(struct {
		Decision SemanticEvaluation `json:"decision"`
		Match    *TriggerMatch      `json:"match,omitempty"`
		Action   *TriggerActionPlan `json:"action,omitempty"`
	}{decision, match, action})
}

func deterministicTriggerID(prefix string, values ...any) string {
	return strings.TrimSpace(prefix) + "-" + deterministicTriggerHash(values...)
}

func deterministicTriggerHash(values ...any) string {
	h := sha256.New()
	for _, value := range values {
		encoded, _ := json.Marshal(value)
		_, _ = h.Write(encoded)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}
