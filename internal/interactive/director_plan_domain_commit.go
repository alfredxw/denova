package interactive

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// ErrDirectorPlanDomainCommitConflict rejects one durable Agent command being
// reused for different Director output. An exact retry returns the receipt
// persisted with the canonical plan metadata.
var ErrDirectorPlanDomainCommitConflict = errors.New("director plan domain commit identity conflict")

// DirectorPlanDomainCommitIdentity mirrors the durable Agent cycle identity
// without coupling the interactive store to the runtime package.
type DirectorPlanDomainCommitIdentity struct {
	CommandID   string `json:"command_id"`
	OperationID string `json:"operation_id"`
	Cycle       int    `json:"cycle"`
}

// DirectorPlanDomainCommitIntent is an immutable, hash-checked Director plan
// publication staged before the durable actor authorizes its output commit.
type DirectorPlanDomainCommitIntent struct {
	Identity        DirectorPlanDomainCommitIdentity
	Hash            string
	AgentOutputHash string
	Token           DirectorPlanRunToken
	SourceTurnID    string
	Summary         string
	Docs            DirectorPlanDocs
}

// DirectorPlanDomainCommitReceipt is stored in Director metadata in the same
// atomic write that makes a plan run terminal.
type DirectorPlanDomainCommitReceipt struct {
	Identity        DirectorPlanDomainCommitIdentity `json:"identity"`
	Hash            string                           `json:"hash"`
	AgentOutputHash string                           `json:"agent_output_hash"`
	Revision        string                           `json:"revision"`
	Plan            DirectorPlan                     `json:"-"`
}

func NewDirectorPlanDomainCommitIntent(identity DirectorPlanDomainCommitIdentity, agentOutputHash string, token DirectorPlanRunToken, sourceTurnID, summary string, docs DirectorPlanDocs) (DirectorPlanDomainCommitIntent, error) {
	identity.CommandID = strings.TrimSpace(identity.CommandID)
	identity.OperationID = strings.TrimSpace(identity.OperationID)
	agentOutputHash = strings.TrimSpace(agentOutputHash)
	token.StoryID = strings.TrimSpace(token.StoryID)
	token.BranchID = strings.TrimSpace(token.BranchID)
	sourceTurnID = strings.TrimSpace(sourceTurnID)
	if identity.CommandID == "" || identity.OperationID == "" || identity.Cycle <= 0 || agentOutputHash == "" {
		return DirectorPlanDomainCommitIntent{}, fmt.Errorf("%w: command_id, operation_id, positive cycle, and Agent output hash are required", ErrDirectorPlanDomainCommitConflict)
	}
	if token.StoryID == "" || token.BranchID == "" || sourceTurnID == "" {
		return DirectorPlanDomainCommitIntent{}, fmt.Errorf("%w: story_id, branch_id, and source_turn_id are required", ErrDirectorPlanDomainCommitConflict)
	}
	payload := struct {
		AgentOutputHash string
		Token           DirectorPlanRunToken
		SourceTurnID    string
		Summary         string
		Docs            DirectorPlanDocs
	}{AgentOutputHash: agentOutputHash, Token: token, SourceTurnID: sourceTurnID, Summary: summary, Docs: docs}
	data, err := json.Marshal(payload)
	if err != nil {
		return DirectorPlanDomainCommitIntent{}, fmt.Errorf("hash Director plan domain commit: %w", err)
	}
	sum := sha256.Sum256(data)
	return DirectorPlanDomainCommitIntent{
		Identity: identity, Hash: fmt.Sprintf("sha256:%x", sum[:]), AgentOutputHash: agentOutputHash, Token: token,
		SourceTurnID: sourceTurnID, Summary: summary, Docs: docs,
	}, nil
}

// CommitDirectorPlanRun publishes or reconciles one authorized Director plan
// output. Exact retries, including after process restart, return the original
// canonical receipt instead of rewriting plan files or terminal metadata.
func (s *Store) CommitDirectorPlanRun(intent DirectorPlanDomainCommitIntent) (DirectorPlanDomainCommitReceipt, error) {
	canonical, err := NewDirectorPlanDomainCommitIntent(intent.Identity, intent.AgentOutputHash, intent.Token, intent.SourceTurnID, intent.Summary, intent.Docs)
	if err != nil {
		return DirectorPlanDomainCommitReceipt{}, err
	}
	if canonical.Hash != strings.TrimSpace(intent.Hash) {
		return DirectorPlanDomainCommitReceipt{}, fmt.Errorf("%w: staged intent hash changed", ErrDirectorPlanDomainCommitConflict)
	}
	pending := DirectorPlanDomainCommitReceipt{Identity: canonical.Identity, Hash: canonical.Hash, AgentOutputHash: canonical.AgentOutputHash}
	plan, err := s.completeDirectorPlanRun(
		canonical.Token.StoryID,
		canonical.Token.BranchID,
		canonical.Token,
		canonical.SourceTurnID,
		canonical.Summary,
		&canonical.Docs,
		&pending,
	)
	if err != nil {
		return DirectorPlanDomainCommitReceipt{}, err
	}
	if plan.Metadata.LastRun == nil || plan.Metadata.LastRun.DomainCommit == nil {
		return DirectorPlanDomainCommitReceipt{}, fmt.Errorf("Director plan commit completed without a canonical receipt")
	}
	receipt := *plan.Metadata.LastRun.DomainCommit
	receipt.Plan = plan
	if receipt.Identity != canonical.Identity || receipt.Hash != canonical.Hash ||
		receipt.AgentOutputHash != canonical.AgentOutputHash || strings.TrimSpace(receipt.Revision) == "" {
		return DirectorPlanDomainCommitReceipt{}, fmt.Errorf("%w: canonical receipt does not match staged intent", ErrDirectorPlanDomainCommitConflict)
	}
	return receipt, nil
}

// FindDirectorPlanDomainCommit reads the exact Agent output receipt stored
// atomically with terminal Director metadata. It never infers success from run
// status alone or from the process-local Patch draft.
func (s *Store) FindDirectorPlanDomainCommit(
	storyID string,
	branchID string,
	identity DirectorPlanDomainCommitIdentity,
	agentOutputHash string,
) (DirectorPlanDomainCommitReceipt, bool, error) {
	if s == nil {
		return DirectorPlanDomainCommitReceipt{}, false, fmt.Errorf("interactive store is nil")
	}
	storyID = strings.TrimSpace(storyID)
	branchID = strings.TrimSpace(branchID)
	identity.CommandID = strings.TrimSpace(identity.CommandID)
	identity.OperationID = strings.TrimSpace(identity.OperationID)
	agentOutputHash = strings.TrimSpace(agentOutputHash)
	if storyID == "" || branchID == "" || identity.CommandID == "" || identity.OperationID == "" || identity.Cycle <= 0 || agentOutputHash == "" {
		return DirectorPlanDomainCommitReceipt{}, false, fmt.Errorf("%w: story, branch, identity, and hash are required", ErrDirectorPlanDomainCommitConflict)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	metadata, err := s.readDirectorPlanMetadataLocked(storyID, branchID)
	if os.IsNotExist(err) {
		return DirectorPlanDomainCommitReceipt{}, false, nil
	}
	if err != nil {
		return DirectorPlanDomainCommitReceipt{}, false, err
	}
	if metadata.LastRun == nil || metadata.LastRun.DomainCommit == nil {
		return DirectorPlanDomainCommitReceipt{}, false, nil
	}
	receipt := *metadata.LastRun.DomainCommit
	if strings.TrimSpace(receipt.Identity.CommandID) != identity.CommandID {
		return DirectorPlanDomainCommitReceipt{}, false, nil
	}
	if receipt.Identity != identity || strings.TrimSpace(receipt.AgentOutputHash) != agentOutputHash ||
		strings.TrimSpace(receipt.Hash) == "" || strings.TrimSpace(receipt.Revision) == "" {
		return DirectorPlanDomainCommitReceipt{}, false, fmt.Errorf(
			"%w: command_id=%q canonical receipt does not match requested operation, cycle, and hash",
			ErrDirectorPlanDomainCommitConflict,
			identity.CommandID,
		)
	}
	return receipt, true, nil
}

func matchDirectorPlanDomainCommit(existing *DirectorPlanDomainCommitReceipt, pending *DirectorPlanDomainCommitReceipt) (bool, error) {
	if existing == nil || pending == nil || strings.TrimSpace(existing.Identity.CommandID) != strings.TrimSpace(pending.Identity.CommandID) {
		return false, nil
	}
	if existing.Identity != pending.Identity || strings.TrimSpace(existing.Hash) != strings.TrimSpace(pending.Hash) ||
		strings.TrimSpace(existing.AgentOutputHash) != strings.TrimSpace(pending.AgentOutputHash) || strings.TrimSpace(existing.Revision) == "" {
		return false, fmt.Errorf("%w: command_id=%q was already committed with different Director output", ErrDirectorPlanDomainCommitConflict, pending.Identity.CommandID)
	}
	return true, nil
}

func attachDirectorPlanDomainCommit(run *DirectorPlanRunStatus, pending *DirectorPlanDomainCommitReceipt, revision string) {
	if run == nil || pending == nil {
		return
	}
	receipt := *pending
	receipt.Revision = strings.TrimSpace(revision)
	run.DomainCommit = &receipt
}

// MarkDirectorTurnDerived acknowledges the rebuildable Director projection
// for one canonical turn without changing the user-facing run status. The
// story turn is the durable outbox item; this metadata field is its completion
// receipt.
func (s *Store) MarkDirectorTurnDerived(storyID, branchID, sourceTurnID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	metadata, err := s.readDirectorPlanMetadataLocked(storyID, branchID)
	if err != nil {
		return err
	}
	sourceTurnID = strings.TrimSpace(sourceTurnID)
	if sourceTurnID == "" {
		return fmt.Errorf("director derived receipt requires source turn id")
	}
	if strings.TrimSpace(metadata.DerivedThroughTurnID) == sourceTurnID {
		return nil
	}
	metadata.DerivedThroughTurnID = sourceTurnID
	metadata.DerivedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return s.writeDirectorPlanMetadataLocked(storyID, branchID, metadata)
}
