package interactive

import (
	"fmt"
	"sort"
	"strings"
)

const (
	TurnSubmissionDiagnosticTurnResultInvalid      = "turn_result_invalid"
	TurnSubmissionDiagnosticUnknownActorStateField = "unknown_actor_state_field"
	TurnSubmissionDiagnosticActorStateInvalid      = "actor_state_invalid"

	turnSubmissionSeverityWarning = "warning"
	turnSubmissionSeverityError   = "error"

	maxTurnSubmissionDiagnostics       = 8
	maxTurnSubmissionAllowedFields     = 16
	maxTurnSubmissionDiagnosticMessage = 1024
	maxTurnSubmissionActorReference    = 128
	maxTurnSubmissionFieldReference    = 256
)

// TurnSubmissionDiagnostic is bounded, model-actionable feedback produced
// while a hidden interactive turn result is normalized and validated.
type TurnSubmissionDiagnostic struct {
	Code          string   `json:"code"`
	Severity      string   `json:"severity"`
	ActorID       string   `json:"actor_id,omitempty"`
	TemplateID    string   `json:"template_id,omitempty"`
	Field         string   `json:"field,omitempty"`
	AllowedFields []string `json:"allowed_fields,omitempty"`
	Message       string   `json:"message"`
}

// TurnSubmissionReceipt tells the Game Agent whether it should continue to
// narrative generation or correct and resubmit the hidden turn result.
type TurnSubmissionReceipt struct {
	Accepted             bool                       `json:"accepted"`
	Retryable            bool                       `json:"retryable"`
	Diagnostics          []TurnSubmissionDiagnostic `json:"diagnostics,omitempty"`
	DiagnosticsTruncated bool                       `json:"diagnostics_truncated,omitempty"`
}

// PreparedTurnSubmission is the only TurnResult shape accepted for staging.
// The persistence layer still performs strict validation at atomic commit.
type PreparedTurnSubmission struct {
	result TurnResult
}

func (s *PreparedTurnSubmission) TurnResult() TurnResult {
	if s == nil {
		return TurnResult{}
	}
	return s.result
}

// PrepareTurnSubmission normalizes a Game Agent submission, drops only Actor
// State fields that are unknown to an otherwise unambiguous frozen template,
// and strictly validates every retained mutation.
func PrepareTurnSubmission(system StoryDirectorActorStateSystem, currentState map[string]any, raw TurnResult) (*PreparedTurnSubmission, TurnSubmissionReceipt) {
	result := NormalizeTurnResult(raw)
	if err := ValidateTurnResult(result); err != nil {
		return nil, rejectedTurnSubmission(TurnSubmissionDiagnosticTurnResultInvalid, err)
	}

	system = normalizeActorStateSystem(system)
	patches, diagnostics, diagnosticsTruncated := sanitizeTurnSubmissionActorStatePatches(system, currentState, result.ActorStatePatches)
	result.ActorStatePatches = patches
	if len(patches) > 0 {
		if _, err := ValidateActorStatePatchesAgainstState(system, currentState, patches, ""); err != nil {
			receipt := rejectedTurnSubmission(TurnSubmissionDiagnosticActorStateInvalid, err)
			remaining := maxTurnSubmissionDiagnostics - len(receipt.Diagnostics)
			if remaining > len(diagnostics) {
				remaining = len(diagnostics)
			}
			receipt.Diagnostics = append(receipt.Diagnostics, diagnostics[:remaining]...)
			receipt.DiagnosticsTruncated = diagnosticsTruncated || remaining < len(diagnostics)
			return nil, receipt
		}
	}

	return &PreparedTurnSubmission{result: result}, TurnSubmissionReceipt{
		Accepted:             true,
		Retryable:            false,
		Diagnostics:          diagnostics,
		DiagnosticsTruncated: diagnosticsTruncated,
	}
}

type turnSubmissionActorContract struct {
	exists     bool
	templateID string
}

func sanitizeTurnSubmissionActorStatePatches(system StoryDirectorActorStateSystem, currentState map[string]any, patches []ActorStatePatch) ([]ActorStatePatch, []TurnSubmissionDiagnostic, bool) {
	if len(patches) == 0 {
		return nil, nil, false
	}
	templates := make(map[string]ActorStateTemplate, len(system.Templates))
	for _, template := range system.Templates {
		templates[template.ID] = template
	}
	contracts := turnSubmissionActorContracts(currentState)
	out := make([]ActorStatePatch, 0, len(patches))
	diagnostics := make([]TurnSubmissionDiagnostic, 0)
	diagnosticsTruncated := false

	for _, source := range patches {
		patch := source
		patch.State = cloneTurnSubmissionState(source.State)
		patch.TraitChanges = append([]ActorTraitChange(nil), source.TraitChanges...)
		actorID := normalizeActorStateID(patch.ActorID)
		patch.ActorID = actorID
		patch.TemplateID = normalizeActorStateID(patch.TemplateID)
		contract := contracts[actorID]
		created := !contract.exists
		bindLegacyTemplate := contract.exists && contract.templateID == ""

		effectiveTemplateID := patch.TemplateID
		contractIsUnambiguous := true
		if contract.exists && contract.templateID != "" {
			effectiveTemplateID = contract.templateID
			if patch.TemplateID != "" && patch.TemplateID != contract.templateID {
				contractIsUnambiguous = false
			}
		}
		template, templateFound := templates[effectiveTemplateID]
		if contractIsUnambiguous && templateFound && len(patch.State) > 0 {
			fieldByReference := actorStateFieldsByReference(template)
			allowedFields := turnSubmissionAllowedFields(template)
			allowedFieldsReported := false
			normalizedState := make(map[string]any, len(patch.State))
			keys := make([]string, 0, len(patch.State))
			for key := range patch.State {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				field, ok := fieldByReference[actorStateFieldNameKey(key)]
				if ok {
					normalizedState[actorStateFieldID(field)] = patch.State[key]
					continue
				}
				diagnostic := TurnSubmissionDiagnostic{
					Code:       TurnSubmissionDiagnosticUnknownActorStateField,
					Severity:   turnSubmissionSeverityWarning,
					ActorID:    trimBytes(actorID, maxTurnSubmissionActorReference),
					TemplateID: trimBytes(effectiveTemplateID, maxTurnSubmissionActorReference),
					Field:      trimBytes(normalizeActorStateFieldName(key), maxTurnSubmissionFieldReference),
					Message: trimBytes(fmt.Sprintf(
						"已忽略模板中不存在的 Actor 状态字段 %q；提交已继续处理，无需为该 warning 重试。 / Unknown Actor State field was ignored; do not retry this accepted submission.",
						key,
					), maxTurnSubmissionDiagnosticMessage),
				}
				if len(diagnostics) < maxTurnSubmissionDiagnostics {
					if !allowedFieldsReported {
						diagnostic.AllowedFields = allowedFields
						allowedFieldsReported = true
					}
					diagnostics = append(diagnostics, diagnostic)
				} else {
					diagnosticsTruncated = true
				}
			}
			patch.State = normalizedState
		}

		if created && patch.TemplateID != "" {
			contracts[actorID] = turnSubmissionActorContract{exists: true, templateID: patch.TemplateID}
		} else if bindLegacyTemplate && patch.TemplateID != "" {
			contracts[actorID] = turnSubmissionActorContract{exists: true, templateID: patch.TemplateID}
		}
		if shouldDropUnknownOnlyTurnSubmissionPatch(patch, created, bindLegacyTemplate, len(source.State) > 0) {
			continue
		}
		out = append(out, patch)
	}
	if len(out) == 0 {
		out = nil
	}
	return out, diagnostics, diagnosticsTruncated
}

func turnSubmissionActorContracts(currentState map[string]any) map[string]turnSubmissionActorContract {
	contracts := map[string]turnSubmissionActorContract{}
	actors, _ := currentState[actorStateRoot].(map[string]any)
	for actorKey, raw := range actors {
		actor, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		actorID := normalizeActorStateID(actorKey)
		if actorID == "" {
			continue
		}
		templateID, _ := actor["template_id"].(string)
		contracts[actorID] = turnSubmissionActorContract{exists: true, templateID: normalizeActorStateID(templateID)}
	}
	return contracts
}

func turnSubmissionAllowedFields(template ActorStateTemplate) []string {
	limit := len(template.Fields)
	if limit > maxTurnSubmissionAllowedFields {
		limit = maxTurnSubmissionAllowedFields
	}
	allowed := make([]string, 0, limit)
	for _, field := range template.Fields[:limit] {
		if fieldID := actorStateFieldID(field); fieldID != "" {
			allowed = append(allowed, trimBytes(fieldID, maxTurnSubmissionFieldReference))
		}
	}
	return allowed
}

func cloneTurnSubmissionState(state map[string]any) map[string]any {
	if state == nil {
		return nil
	}
	cloned := make(map[string]any, len(state))
	for key, value := range state {
		cloned[key] = value
	}
	return cloned
}

func shouldDropUnknownOnlyTurnSubmissionPatch(patch ActorStatePatch, created, bindLegacyTemplate, originallyHadState bool) bool {
	if !originallyHadState || len(patch.State) > 0 || created || bindLegacyTemplate || len(patch.TraitChanges) > 0 {
		return false
	}
	return strings.TrimSpace(patch.ActorName) == "" &&
		strings.TrimSpace(patch.Role) == "" &&
		strings.TrimSpace(patch.Description) == ""
}

func rejectedTurnSubmission(code string, err error) TurnSubmissionReceipt {
	message := "TurnResult 校验失败 / TurnResult validation failed"
	if err != nil {
		message = trimBytes(err.Error(), maxTurnSubmissionDiagnosticMessage)
	}
	return TurnSubmissionReceipt{
		Accepted:  false,
		Retryable: true,
		Diagnostics: []TurnSubmissionDiagnostic{{
			Code:     code,
			Severity: turnSubmissionSeverityError,
			Message:  message,
		}},
	}
}
