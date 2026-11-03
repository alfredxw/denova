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

// storyContextSubmissionDiagnostic checks continuity for both existing and
// newly created story Actors after the state changes pass compilation.
func storyContextSubmissionDiagnostic(system StoryDirectorActorStateSystem, currentState map[string]any, updates []interactivestate.Update) *TurnSubmissionDiagnostic {
	template := actorStateTemplateByID(system, ActorStateStoryContextTemplateID)
	if len(template.Fields) == 0 {
		return nil
	}
	values, created := submittedStoryContextState(updates)
	if !created && !hasStoryContextActor(system, currentState) {
		return nil
	}

	if _, exists := actorStateFieldByID(template, storyContextCurrentEventField); exists {
		if !hasConcreteActorStateValue(values[actorStateFieldNameKey(storyContextCurrentEventField)]) {
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
	if !hasConcreteActorStateValue(values[actorStateFieldNameKey(storyContextCurrentLocationField)]) {
		return newStoryContextRequiredDiagnostic(storyContextCurrentLocationField, "Story state has not been initialized, so state_changes must provide a non-empty story/当前详细地点 value.")
	}
	return nil
}

// submittedStoryContextState reads submissions rather than reducer operations.
// A repeated current event satisfies the protocol even when it changes no state.
func submittedStoryContextState(updates []interactivestate.Update) (values map[string]any, created bool) {
	for _, update := range updates {
		segments, err := interactivestate.ParsePath(update.Path)
		if err != nil || len(segments) == 0 || segments[0] != DefaultStoryContextActorID {
			continue
		}
		if update.Op == interactivestate.Create && len(segments) == 1 {
			patch, err := actorPatchFromCreateUpdate(DefaultStoryContextActorID, update.Value)
			if err == nil && normalizeActorStateID(patch.TemplateID) == ActorStateStoryContextTemplateID {
				values = make(map[string]any, len(patch.State))
				for field, value := range patch.State {
					values[actorStateFieldNameKey(field)] = value
				}
				return values, true
			}
		}
		if update.Op == interactivestate.Replace && len(segments) == 2 {
			if values == nil {
				values = map[string]any{}
			}
			values[actorStateFieldNameKey(segments[1])] = update.Value
		}
	}
	return values, false
}

func hasStoryContextActor(system StoryDirectorActorStateSystem, currentState map[string]any) bool {
	if templateID, found := actorTemplateIDFromStateOrSystem(currentState, system, DefaultStoryContextActorID); found && templateID == ActorStateStoryContextTemplateID {
		return true
	}
	return false
}

func newStoryContextRequiredDiagnostic(field, reason string) *TurnSubmissionDiagnostic {
	path := interactivestate.FormatPath([]string{DefaultStoryContextActorID, field})
	return newTurnSubmissionDiagnostic(
		TurnSubmissionModuleStateChanges,
		nil,
		TurnSubmissionDiagnosticStoryContextRequired,
		path,
		fmt.Sprintf("a non-empty %s value on the story Actor", field),
		"missing",
		reason+" Use create.initial_state when creating story, or replace the field on an existing story Actor. Do not clear unchanged fields.",
	)
}
