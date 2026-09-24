package platform

import (
	"context"

	"denova/internal/interactive"
	"denova/internal/platform"
)

func platformStoryStateSchema(schema *interactive.ActorStateSchemaSnapshot) *platform.StoryStateSchema {
	if schema == nil {
		return nil
	}
	result := &platform.StoryStateSchema{Version: schema.Version, Revision: schema.Revision, Templates: []platform.StoryStateTemplate{}}
	for _, template := range schema.System.Templates {
		item := platform.StoryStateTemplate{ID: template.ID, Name: template.Name, Description: template.Description, Fields: []platform.StoryStateField{}}
		for _, field := range template.Fields {
			item.Fields = append(item.Fields, platform.StoryStateField{Name: field.Name, Type: field.Type, Default: field.Default, Min: field.Min, Max: field.Max, Options: field.Options, Description: field.Description, Group: field.Group, Display: field.Display})
		}
		result.Templates = append(result.Templates, item)
	}
	return result
}

func platformStoryStateChanges(delta *interactive.StateDelta) []platform.StoryStateChange {
	changes := []platform.StoryStateChange{}
	if delta == nil {
		return changes
	}
	for _, op := range delta.Ops {
		changes = append(changes, platform.StoryStateChange{Op: op.Op, Path: op.Path, Value: op.Value, Reason: op.Reason, SourceTurnID: op.SourceTurnID, SourceKind: op.SourceKind, SourceID: op.SourceID})
	}
	for _, op := range delta.ActorOps {
		changes = append(changes, platform.StoryStateChange{Op: op.Op, ActorID: op.ActorID, FieldID: op.FieldID, Value: op.Value, Reason: op.Reason, SourceTurnID: op.SourceTurnID, SourceKind: op.SourceKind, SourceID: op.SourceID})
	}
	return changes
}

func (h Stories) State(ctx context.Context, scope platform.Scope, turnID string) (platform.StoryState, error) {
	store, err := h.host.OpenStoryStore(ctx, scope.ProjectID)
	if err != nil {
		return platform.StoryState{}, err
	}
	defer store.Close()
	if turnID != "" {
		projection, err := store.ReadStateAtTurn(scope.StoryID, scope.BranchID, turnID)
		if err != nil {
			return platform.StoryState{}, platformStoryError("NOT_FOUND", err.Error())
		}
		return platform.StoryState{StoryID: scope.StoryID, BranchID: projection.BranchID, TurnID: turnID, SourceRevision: projection.SourceRevision, State: projection.State, StateSchema: platformStoryStateSchema(projection.Schema)}, nil
	}
	story, err := store.StoryContext(scope.StoryID, scope.BranchID)
	if err != nil {
		return platform.StoryState{}, err
	}
	result := platform.StoryState{StoryID: scope.StoryID, BranchID: story.Snapshot.BranchID, State: story.Snapshot.State, StateSchema: platformStoryStateSchema(story.Snapshot.ActorStateSchema)}
	if turn := story.Snapshot.CurrentTurn; turn != nil {
		result.TurnID = turn.ID
		result.SourceRevision = interactive.TurnNarrativeRevision(*turn)
	}
	return result, nil
}
