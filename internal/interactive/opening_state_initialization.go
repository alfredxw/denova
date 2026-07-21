package interactive

import (
	"fmt"
	"strings"
)

// OpeningStateInitializationField identifies one writable field that still
// needs a concrete value after schema defaults and initial Actor values apply.
type OpeningStateInitializationField struct {
	ActorID    string `json:"actor_id"`
	TemplateID string `json:"template_id"`
	FieldID    string `json:"field_id"`
	Type       string `json:"type"`
}

// OpeningStateInitializationGuide bridges the structure-only schema phase and
// the first submit_interactive_turn call without exposing or persisting a draft.
type OpeningStateInitializationGuide struct {
	TotalWritableFields   int                               `json:"total_writable_fields"`
	AutoInitializedFields int                               `json:"auto_initialized_fields"`
	RequiredStateChanges  []OpeningStateInitializationField `json:"required_state_changes"`
}

func buildOpeningStateInitializationGuide(system StoryDirectorActorStateSystem) (OpeningStateInitializationGuide, error) {
	system = normalizeActorStateSystem(system)
	state := initialStoryState()
	rawActors := state[actorStateRoot].(map[string]any)
	for _, actor := range system.InitialActors {
		template := actorStateTemplateByID(system, actor.TemplateID)
		if template.ID == "" {
			continue
		}
		_, normalizedState, err := buildActorStateValueOps(template, actor.ID, actor.State, "", "")
		if err != nil {
			return OpeningStateInitializationGuide{}, err
		}
		rawActors[actor.ID] = map[string]any{
			"id": actor.ID, "template_id": template.ID, "state": normalizedState,
		}
	}
	total, missing := openingInitialStateCoverage(system, state)
	return OpeningStateInitializationGuide{
		TotalWritableFields: total, AutoInitializedFields: total - len(missing), RequiredStateChanges: missing,
	}, nil
}

func openingInitialStateCoverage(system StoryDirectorActorStateSystem, state map[string]any) (int, []OpeningStateInitializationField) {
	system = normalizeActorStateSystem(system)
	total := 0
	missing := make([]OpeningStateInitializationField, 0)
	for _, actor := range system.InitialActors {
		template := actorStateTemplateByID(system, actor.TemplateID)
		if template.ID == "" {
			continue
		}
		for _, field := range template.Fields {
			total++
			fieldID := actorStateFieldID(field)
			if meaningfulOpeningInitialStateValue(actorStateFieldValue(state, actor.ID, fieldID)) {
				continue
			}
			missing = append(missing, OpeningStateInitializationField{
				ActorID: actor.ID, TemplateID: template.ID, FieldID: fieldID, Type: field.Type,
			})
		}
	}
	return total, missing
}

func meaningfulOpeningInitialStateValue(value any) bool {
	if value == nil {
		return false
	}
	text, ok := value.(string)
	return !ok || strings.TrimSpace(text) != ""
}

func openingInitialStateSubmissionDiagnostic(system StoryDirectorActorStateSystem, currentState map[string]any, compiled CompiledTurnStateUpdates) *TurnSubmissionDiagnostic {
	state := cloneActorStateRoot(currentState)
	for _, op := range compiled.Ops {
		applyStateOp(state, op)
	}
	for _, op := range compiled.ActorOps {
		applyActorStateOp(state, op)
	}
	_, missing := openingInitialStateCoverage(system, state)
	if len(missing) == 0 {
		return nil
	}
	const displayedMissingFields = 8
	labels := make([]string, 0, min(len(missing), displayedMissingFields))
	for index, field := range missing {
		if index >= displayedMissingFields {
			break
		}
		labels = append(labels, field.ActorID+"/"+field.FieldID+" ("+field.Type+")")
	}
	remaining := ""
	if len(missing) > len(labels) {
		remaining = fmt.Sprintf("，另有 %d 项", len(missing)-len(labels))
	}
	actual := strings.Join(labels, ", ") + remaining
	return newTurnSubmissionDiagnostic(
		TurnSubmissionModuleStateChanges,
		nil,
		TurnSubmissionDiagnosticInitialStateIncomplete,
		"/state_changes",
		"every writable field of each initial Actor has a concrete opening value",
		actual,
		"开局状态尚未完整初始化，缺少 "+actual+"；请按 initialize_story_state_schema 回执的 initialization_guide.required_state_changes 一次补齐，不能使用空字符串、未设置、未知或待定占位。",
		"The opening state is incomplete. Initialize every field listed by initialization_guide.required_state_changes with a concrete value in one state_changes submission.",
	)
}
