package interactive

import (
	"fmt"
	"strings"
)

const (
	storyContextCurrentLocationField = "当前详细地点"
	storyContextCurrentEventField    = "当前事件"
)

// storyContextSubmissionDiagnostic enforces the built-in story_context
// contract at the model submission boundary. Other custom non-character state
// objects remain optional and continue to use the generic Actor State path.
func storyContextSubmissionDiagnostic(system StoryDirectorActorStateSystem, currentState map[string]any, result TurnResult) *TurnSubmissionDiagnostic {
	template := actorStateTemplateByID(system, ActorStateStoryContextTemplateID)
	if template.ID == "" || !hasStoryContextActor(system, currentState) || len(template.Fields) == 0 {
		return nil
	}

	patch, ok := actorStatePatchByID(result.ActorStatePatches, DefaultStoryContextActorID)
	if !ok || len(patch.State) == 0 {
		return newStoryContextRequiredDiagnostic(template, storyContextCurrentEventField, "本回合未提交 story 状态补丁")
	}

	if _, exists := actorStateFieldByID(template, storyContextCurrentEventField); exists {
		if !meaningfulStoryContextValue(patch.State[storyContextCurrentEventField]) {
			return newStoryContextRequiredDiagnostic(template, storyContextCurrentEventField, "story 状态补丁缺少非空的当前事件")
		}
	}

	if _, exists := actorStateFieldByID(template, storyContextCurrentLocationField); !exists {
		return nil
	}
	currentLocation, _ := actorStateFieldValue(currentState, DefaultStoryContextActorID, storyContextCurrentLocationField).(string)
	currentLocation = strings.TrimSpace(currentLocation)
	submittedLocation, _ := patch.State[storyContextCurrentLocationField].(string)
	submittedLocation = strings.TrimSpace(submittedLocation)
	expectedLocation := turnResultLocation(result.SceneResult)
	if currentLocation == "" && submittedLocation == "" {
		return newStoryContextRequiredDiagnostic(template, storyContextCurrentLocationField, "story 状态尚未初始化，补丁必须填写非空的当前详细地点")
	}
	if expectedLocation == "" || currentLocation == expectedLocation {
		return nil
	}
	if submittedLocation == expectedLocation {
		return nil
	}
	return newStoryContextRequiredDiagnostic(template, storyContextCurrentLocationField, fmt.Sprintf("story 状态补丁必须把当前详细地点同步为 %q", expectedLocation))
}

func hasStoryContextActor(system StoryDirectorActorStateSystem, currentState map[string]any) bool {
	if actor, ok := turnSubmissionActorContracts(currentState)[DefaultStoryContextActorID]; ok && actor.templateID == ActorStateStoryContextTemplateID {
		return true
	}
	for _, actor := range system.InitialActors {
		if actor.ID == DefaultStoryContextActorID && actor.TemplateID == ActorStateStoryContextTemplateID {
			return true
		}
	}
	return false
}

func actorStatePatchByID(patches []ActorStatePatch, actorID string) (ActorStatePatch, bool) {
	actorID = normalizeActorStateID(actorID)
	for _, patch := range patches {
		if normalizeActorStateID(patch.ActorID) == actorID {
			return patch, true
		}
	}
	return ActorStatePatch{}, false
}

func turnResultLocation(result TurnSceneResult) string {
	if result.Status == "transitioned" && strings.TrimSpace(result.NextSceneID) != "" {
		return strings.TrimSpace(result.NextSceneID)
	}
	return strings.TrimSpace(result.SceneID)
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

func newStoryContextRequiredDiagnostic(template ActorStateTemplate, field, reason string) *TurnSubmissionDiagnostic {
	return &TurnSubmissionDiagnostic{
		Code:          TurnSubmissionDiagnosticStoryContextRequired,
		Severity:      turnSubmissionSeverityError,
		ActorID:       DefaultStoryContextActorID,
		TemplateID:    ActorStateStoryContextTemplateID,
		Field:         field,
		AllowedFields: turnSubmissionAllowedFields(template),
		Message: trimBytes(
			reason+`；每回合都必须在 actor_state_patches 中提交 actor_id="story" 的 story_context 更新。至少填写“当前事件”，并在当前地点尚未初始化或 scene_result 表示场景切换时填写“当前详细地点”；未变化的其他字段不要置空。 / Every turn must submit the story actor's story_context patch. Include a non-empty current event and synchronize the current location when it is missing or changes.`,
			maxTurnSubmissionDiagnosticMessage,
		),
	}
}
