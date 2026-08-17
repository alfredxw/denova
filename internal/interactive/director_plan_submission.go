package interactive

import (
	"denova/internal/book/lore"
	"denova/internal/interactive/director"
	"fmt"
	"strings"
)

// DirectorPlanUpdateSubmission incrementally stages independently retryable
// Markdown document patches. The first accepted submission fixes the decision
// mode for the run; later retries keep that mode and only resend rejected
// documents. No workspace file changes before Finalize succeeds.
type DirectorPlanUpdateSubmission struct {
	Decision           director.Decision            `json:"decision"`
	Updates            []DirectorPlanDocumentUpdate `json:"updates,omitempty"`
	Finalize           bool                         `json:"finalize"`
	ReviewedLoreIDs    []string                     `json:"-"`
	SourceLoreRevision string                       `json:"-"`
}

type DirectorPlanDocumentAcceptance struct {
	Document        string `json:"document"`
	Hash            string `json:"hash"`
	AlreadyAccepted bool   `json:"already_accepted,omitempty"`
}

type DirectorPlanDocumentIssue struct {
	Document  string `json:"document,omitempty"`
	Code      string `json:"code"`
	Path      string `json:"path,omitempty"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// DirectorPlanUpdateReceipt exposes per-document progress. Finalized means the
// accumulated draft is complete and can be atomically published by the app.
type DirectorPlanUpdateReceipt struct {
	Accepted          []DirectorPlanDocumentAcceptance `json:"accepted"`
	Rejected          []DirectorPlanDocumentIssue      `json:"rejected"`
	RetryDocuments    []string                         `json:"retry_documents,omitempty"`
	AcceptedDocuments []string                         `json:"accepted_documents,omitempty"`
	ChangedDocuments  []string                         `json:"changed_documents,omitempty"`
	Finalized         bool                             `json:"finalized"`
	Decision          director.Decision                `json:"decision"`
}

// StageDirectorPlanRunUpdate validates each document independently and keeps
// successful patches in the run-local draft. Domain validation failures are
// returned as per-document issues so the model can retry only rejected files.
func (s *Store) StageDirectorPlanRunUpdate(storyID, branchID string, token DirectorPlanRunToken, sourceTurnID string, draft *DirectorPlanUpdateDraft, submission DirectorPlanUpdateSubmission) (DirectorPlanUpdateReceipt, error) {
	receipt := DirectorPlanUpdateReceipt{
		Accepted: []DirectorPlanDocumentAcceptance{},
		Rejected: []DirectorPlanDocumentIssue{},
	}
	if s == nil {
		return receipt, fmt.Errorf("interactive story store is unavailable")
	}
	if draft == nil {
		return receipt, fmt.Errorf("Director plan patch draft is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	metadata, err := s.readDirectorPlanMetadataLocked(storyID, branchID)
	if err != nil {
		return receipt, err
	}
	if token.Revision != "" && token.Revision != metadata.Revision {
		return receipt, fmt.Errorf("Director plan was updated by another operation; reload before submitting")
	}
	if metadata.LastRun == nil || metadata.LastRun.Status != director.PlanStatusRunning || strings.TrimSpace(metadata.LastRun.SourceTurnID) != strings.TrimSpace(sourceTurnID) {
		return receipt, fmt.Errorf("the current Director plan run is stale and cannot accept results")
	}
	if draft.finalized {
		receipt.Rejected = append(receipt.Rejected, directorPlanDocumentIssue("", "draft_finalized", "", "The Director plan patch draft is already finalized.", false))
		receipt.Finalized = true
		receipt.Decision, _ = draft.Decision()
		receipt.AcceptedDocuments = draft.acceptedDocuments()
		receipt.ChangedDocuments = append([]string(nil), receipt.AcceptedDocuments...)
		return receipt, nil
	}

	if err := validateDirectorPlanSubmissionLoreRevision(s.root, submission.SourceLoreRevision); err != nil {
		return receipt, err
	}
	if err := draft.acceptDecision(submission.Decision); err != nil {
		return receipt, err
	}
	receipt.Decision, _ = draft.Decision()
	if len(submission.Updates) > maxDirectorDocumentUpdatesPerCall {
		receipt.Rejected = append(receipt.Rejected, directorPlanDocumentIssue("", "too_many_documents", "updates", fmt.Sprintf("too many updates in one call: %d > %d", len(submission.Updates), maxDirectorDocumentUpdatesPerCall), true))
		receipt.RetryDocuments = directorSubmissionDocuments(submission.Updates)
		return receipt, nil
	}

	documentCounts := map[string]int{}
	for _, update := range submission.Updates {
		documentCounts[normalizeDirectorDocument(update.Document)]++
	}
	for index, update := range submission.Updates {
		document := normalizeDirectorDocument(update.Document)
		path := fmt.Sprintf("updates[%d]", index)
		if document == "" {
			receipt.Rejected = append(receipt.Rejected, directorPlanDocumentIssue(strings.TrimSpace(update.Document), "invalid_document", path+".document", "document must be director.md, agent-brief.md, or lore-context.md.", true))
			continue
		}
		if documentCounts[document] > 1 {
			receipt.Rejected = append(receipt.Rejected, directorPlanDocumentIssue(document, "duplicate_document", path+".document", "The same document cannot be updated more than once in one submission.", true))
			continue
		}
		fingerprint := directorDocumentUpdateFingerprint(update)
		if accepted, ok := draft.accepted[document]; ok {
			if accepted.fingerprint == fingerprint {
				receipt.Accepted = append(receipt.Accepted, DirectorPlanDocumentAcceptance{Document: document, Hash: accepted.hash, AlreadyAccepted: true})
				continue
			}
			receipt.Rejected = append(receipt.Rejected, directorPlanDocumentIssue(document, "document_already_accepted", path, "This document is already accepted; do not resend accepted files on retry.", false))
			continue
		}
		if strings.TrimSpace(update.BaseHash) == "" || strings.TrimSpace(update.BaseHash) != draft.baseHash[document] {
			receipt.Rejected = append(receipt.Rejected, directorPlanDocumentIssue(document, "base_hash_mismatch", path+".base_hash", "base_hash does not match the complete document snapshot injected for this run.", true))
			continue
		}
		base := directorDocumentContent(draft.baseline, document)
		content, applyErr := applyDirectorDocumentUpdate(base, update)
		if applyErr != nil {
			receipt.Rejected = append(receipt.Rejected, directorPlanDocumentIssue(document, "patch_failed", path+".edits", applyErr.Error(), true))
			continue
		}
		if strings.TrimSpace(content) == strings.TrimSpace(base) {
			receipt.Rejected = append(receipt.Rejected, directorPlanDocumentIssue(document, "no_change", path+".edits", "The patch does not change the document; omit this file.", true))
			continue
		}
		if validationErr := s.validateDirectorPatchedDocument(document, draft.baseline, content, submission.ReviewedLoreIDs); validationErr != nil {
			receipt.Rejected = append(receipt.Rejected, directorPlanDocumentIssue(document, "validation_failed", path+".edits", validationErr.Error(), true))
			continue
		}
		hash := textHash(content)
		draft.accepted[document] = directorPlanAcceptedDocument{content: content, hash: hash, fingerprint: fingerprint}
		receipt.Accepted = append(receipt.Accepted, DirectorPlanDocumentAcceptance{Document: document, Hash: hash})
	}

	receipt.AcceptedDocuments = draft.acceptedDocuments()
	for _, issue := range receipt.Rejected {
		if issue.Retryable && issue.Document != "" && !containsString(receipt.RetryDocuments, issue.Document) {
			receipt.RetryDocuments = append(receipt.RetryDocuments, issue.Document)
		}
	}
	if !submission.Finalize || len(receipt.Rejected) > 0 {
		return receipt, nil
	}
	if issue := draft.finalizeIssue(); issue != nil {
		receipt.Rejected = append(receipt.Rejected, *issue)
		if issue.Document != "" {
			receipt.RetryDocuments = append(receipt.RetryDocuments, issue.Document)
		}
		return receipt, nil
	}
	if err := validateDirectorPlanDocs(draft.currentDocs()); err != nil {
		receipt.Rejected = append(receipt.Rejected, directorPlanDocumentIssue("", "finalize_validation_failed", "updates", err.Error(), true))
		return receipt, nil
	}
	draft.finalized = true
	receipt.Finalized = true
	receipt.ChangedDocuments = draft.acceptedDocuments()
	return receipt, nil
}

func (d *DirectorPlanUpdateDraft) acceptDecision(value director.Decision) error {
	rawMode := strings.TrimSpace(value.Mode)
	switch rawMode {
	case director.DecisionKeep, director.DecisionPatch, director.DecisionReplan:
	default:
		return fmt.Errorf("invalid Director plan mode: %s", rawMode)
	}
	decision := director.NormalizeDecision(value)
	if d.decision == nil {
		d.decision = &decision
		return nil
	}
	if d.decision.Mode != decision.Mode {
		return fmt.Errorf("one Director run cannot change mode from %s to %s; keep the first decision.mode", d.decision.Mode, decision.Mode)
	}
	if strings.TrimSpace(decision.Reason) != "" {
		d.decision.Reason = decision.Reason
	}
	if len(decision.Triggers) > 0 {
		d.decision.Triggers = decision.Triggers
	}
	if decision.SceneTransition.Kind != "" && decision.SceneTransition.Kind != "none" {
		d.decision.SceneTransition = decision.SceneTransition
	}
	if decision.Deviation.Level != "" && decision.Deviation.Level != "none" {
		d.decision.Deviation = decision.Deviation
	}
	if decision.EventDecision != nil {
		d.decision.EventDecision = decision.EventDecision
	}
	return nil
}

func (d *DirectorPlanUpdateDraft) finalizeIssue() *DirectorPlanDocumentIssue {
	decision, ok := d.Decision()
	if !ok {
		issue := directorPlanDocumentIssue("", "decision_missing", "decision", "A decision must be submitted before finalization.", true)
		return &issue
	}
	changed := d.acceptedDocuments()
	switch decision.Mode {
	case director.DecisionKeep:
		if len(changed) > 0 {
			issue := directorPlanDocumentIssue("", "keep_with_updates", "updates", "A keep decision must not modify documents.", false)
			return &issue
		}
	case director.DecisionPatch:
		if len(changed) == 0 {
			issue := directorPlanDocumentIssue(DirectorDocumentAgentBrief, "patch_without_updates", "updates", "A patch decision requires at least one document patch; routine updates should prefer changing only agent-brief.md.", true)
			return &issue
		}
	case director.DecisionReplan:
		if _, ok := d.accepted[DirectorDocumentPlan]; !ok {
			issue := directorPlanDocumentIssue(DirectorDocumentPlan, "replan_requires_plan", "updates", "A replan must update director.md.", true)
			return &issue
		}
		if _, ok := d.accepted[DirectorDocumentAgentBrief]; !ok {
			issue := directorPlanDocumentIssue(DirectorDocumentAgentBrief, "replan_requires_brief", "updates", "A replan must also update agent-brief.md; lore-context.md remains optional.", true)
			return &issue
		}
	}
	return nil
}

func (s *Store) validateDirectorPatchedDocument(document string, baseline DirectorPlanDocs, content string, reviewedLoreIDs []string) error {
	kind := directorDocumentKind(document)
	if kind == "" {
		return fmt.Errorf("unknown Director document: %s", document)
	}
	if kind != DirectorPlanDocLoreContext {
		return validateDirectorPlanDoc(kind, content)
	}
	if err := validateDirectorLoreContextDoc(content); err != nil {
		return err
	}
	if err := s.validateDirectorLoreContext(content); err != nil {
		return err
	}
	return s.validateDirectorLoreGrounding(baseline.LoreContext, content, reviewedLoreIDs)
}

func validateDirectorPlanSubmissionLoreRevision(workspace, sourceRevision string) error {
	sourceRevision = strings.TrimSpace(sourceRevision)
	if sourceRevision == "" {
		return fmt.Errorf("Director plan submission is missing the source lore revision")
	}
	currentRevision, err := lore.NewStore(workspace).Revision()
	if err != nil {
		return fmt.Errorf("read lore revision: %w", err)
	}
	if sourceRevision != currentRevision {
		return fmt.Errorf("lore changed during Director review; replan from the latest name roster")
	}
	return nil
}

func directorPlanDocumentIssue(document, code, path, message string, retryable bool) DirectorPlanDocumentIssue {
	return DirectorPlanDocumentIssue{Document: document, Code: code, Path: path, Message: message, Retryable: retryable}
}

func directorSubmissionDocuments(updates []DirectorPlanDocumentUpdate) []string {
	result := []string{}
	for _, update := range updates {
		document := normalizeDirectorDocument(update.Document)
		if document != "" && !containsString(result, document) {
			result = append(result, document)
		}
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
