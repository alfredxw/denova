package interactive

import (
	"fmt"
	"strings"
)

const (
	storyContextCurrentLocationField = "当前详细地点"
	storyContextCurrentEventField    = "当前事件"
)

// storyContextSubmissionDiagnostic keeps the built-in story_context useful
// after the model-facing contract was reduced to actor_state_patches and choices.
func storyContextSubmissionDiagnostic(system StoryDirectorActorStateSystem, currentState map[string]any, updates []StateUpdate) *TurnSubmissionDiagnostic {
	template := actorStateTemplateByID(system, ActorStateStoryContextTemplateID)
	if template.ID == "" || !hasStoryContextActor(system, currentState) || len(template.Fields) == 0 {
		return nil
	}

	if _, exists := actorStateFieldByID(template, storyContextCurrentEventField); exists {
		value, found := submittedStoryContextValue(updates, storyContextCurrentEventField)
		if !found || !meaningfulStoryContextValue(value) {
			return newStoryContextRequiredDiagnostic(storyContextCurrentEventField, "本回合 patches 缺少非空的 story/当前事件")
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
		return newStoryContextRequiredDiagnostic(storyContextCurrentLocationField, "story 状态尚未初始化，patches 必须填写非空的 story/当前详细地点")
	}
	return nil
}

func submittedStoryContextValue(updates []StateUpdate, fieldID string) (any, bool) {
	for _, update := range updates {
		if update.Op != TurnStateUpdateReplace {
			continue
		}
		segments, err := parseStateUpdatePath(update.Path)
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
	path := formatStateUpdatePath([]string{DefaultStoryContextActorID, field})
	return newTurnSubmissionDiagnostic(
		TurnSubmissionModuleActorStatePatches,
		nil,
		TurnSubmissionDiagnosticStoryContextRequired,
		path,
		fmt.Sprintf(`{"op":"replace","path":%q,"value":"..."}`, path),
		"missing",
		reason+"；每回合至少维护“当前事件”，首次初始化时还要维护“当前详细地点”，其他未变化字段不要提交空值。",
		"Every turn must replace story/Current Event, and initialization must also replace story/Current Detailed Location. Do not clear unchanged fields.",
	)
}
