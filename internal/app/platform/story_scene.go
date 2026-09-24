package platform

import (
	"context"
	"errors"

	"denova/internal/interactive"
	"denova/internal/platform"
)

func (h Stories) Scene(ctx context.Context, scope platform.Scope, turnID, sourceRevision string) (platform.StoryScene, error) {
	store, err := h.host.OpenStoryStore(ctx, scope.ProjectID)
	if err != nil {
		return platform.StoryScene{}, err
	}
	defer store.Close()
	scene, err := store.ReadSceneAtTurn(ctx, scope.StoryID, scope.BranchID, turnID, sourceRevision)
	if err != nil {
		switch {
		case errors.Is(err, interactive.ErrStorySceneNotFound):
			return platform.StoryScene{}, platformStoryError("NOT_FOUND", err.Error())
		case errors.Is(err, interactive.ErrStorySceneRevisionConflict):
			return platform.StoryScene{}, platformStoryError("DOCUMENT_CONFLICT", err.Error())
		default:
			return platform.StoryScene{}, err
		}
	}
	result := platform.StoryScene{Turn: platformStoryTurns([]interactive.TurnEvent{scene.Turn})[0], State: platform.StoryState{StoryID: scope.StoryID, BranchID: scope.BranchID, TurnID: turnID, SourceRevision: scene.State.SourceRevision, State: scene.State.State, StateSchema: platformStoryStateSchema(scene.State.Schema)}}
	if scene.PreviousTurn != nil {
		previous := platformStoryTurns([]interactive.TurnEvent{*scene.PreviousTurn})[0]
		result.PreviousTurn = &previous
	}
	return result, nil
}
