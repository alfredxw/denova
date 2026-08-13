package interactive

import (
	interactivestate "denova/internal/interactive/state"
	"fmt"
	"strings"
)

const (
	storyContextCurrentLocationField = "当前详细地点"
	storyContextCurrentEventField    = "当前事件"
)

// storyContextSubmissionDiagnostic keeps the built-in story_context useful
// after the model-facing contract was reduced to state_changes and choices.
func storyContextSubmissionDiagnostic(system StoryDirectorActorStateSystem, currentState map[string]any, updates []interactivestate.Update) *TurnSubmissionDiagnostic {
	template := actorStateTemplateByID(system, ActorStateStoryContextTemplateID)
	if template.ID == "" || !hasStoryContextActor(system, currentState) || len(template.Fields) == 0 {
		return nil
	}

	if _, exists := actorStateFieldByID(template, storyContextCurrentEventField); exists {
		value, found := submittedStoryContextValue(updates, storyContextCurrentEventField)
		if !found || !meaningfulStoryContextValue(value) {
			return newStoryContextRequiredDiagnostic(storyContextCurrentEventField, "This turn's state_changes is missing a non-empty story/当前事件 value.")
		}
	}

	if _, exists := actorStateFieldByID(template, storyContextCurrentLocationField); !exists {
		return nil
	}
	currentLocation, _ := actorStateFieldValue(currentState, DefaultStoryContextActorID, storyContextCurrentLocationField).(string)
	if strings.TrimSpace(currentLocation) != "" {
		return nil
	}
	value, found := submittedStoryContextValue(updates, storyContextCurrentLocationField)
	if !found || !meaningfulStoryContextValue(value) {
		return newStoryContextRequiredDiagnostic(storyContextCurrentLocationField, "Story state has not been initialized, so state_changes must provide a non-empty story/当前详细地点 value.")
	}
	return nil
}

func submittedStoryContextValue(updates []interactivestate.Update, fieldID string) (any, bool) {
	for _, update := range updates {
		if update.Op != interactivestate.Replace {
			continue
		}
		segments, err := interactivestate.ParsePath(update.Path)
		if err != nil || len(segments) != 2 {
			continue
		}
		if segments[0] == DefaultStoryContextActorID && actorStateFieldNameKey(segments[1]) == actorStateFieldNameKey(fieldID) {
			return update.Value, true
		}
	}
	return nil, false
}

func hasStoryContextActor(system StoryDirectorActorStateSystem, currentState map[string]any) bool {
	if templateID, found := actorTemplateIDFromStateOrSystem(currentState, system, DefaultStoryContextActorID); found && templateID == ActorStateStoryContextTemplateID {
		return true
	}
	return false
}

func meaningfulStoryContextValue(value any) bool {
	if value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	return true
}

func newStoryContextRequiredDiagnostic(field, reason string) *TurnSubmissionDiagnostic {
	path := interactivestate.FormatPath([]string{DefaultStoryContextActorID, field})
	return newTurnSubmissionDiagnostic(
		TurnSubmissionModuleStateChanges,
		nil,
		TurnSubmissionDiagnosticStoryContextRequired,
		path,
		fmt.Sprintf(`{"op":"replace","actor_id":%q,"field_id":%q,"value":"..."}`, DefaultStoryContextActorID, field),
		"missing",
		reason+" Every turn must replace story/当前事件, and initialization must also replace story/当前详细地点. Do not clear unchanged fields.",
	)
}
