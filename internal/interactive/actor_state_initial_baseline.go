package interactive

import "fmt"

// applyFrozenMissingInitialActors materializes only initial Actors absent after
// event replay. New stories keep their persisted ActorOps authoritative, while
// legacy stories that froze a schema before ActorOps existed no longer require
// the model to recreate built-in Actors. Applying this after replay also avoids
// current-schema defaults shadowing historical field migrations.
func applyFrozenMissingInitialActors(state map[string]any, snapshot *ActorStateSchemaSnapshot) error {
	if snapshot == nil || len(snapshot.System.InitialActors) == 0 {
		return nil
	}
	return applyMissingInitialActors(state, snapshot.System, "冻结 schema 初始状态")
}

// applyMissingInitialActors projects schema-defined initial Actors only when
// replayed state has no authoritative record for the same stable ID. Callers
// may use this for persisted snapshot completion or an in-memory model view.
func applyMissingInitialActors(state map[string]any, system StoryDirectorActorStateSystem, reason string) error {
	system = normalizeActorStateSystem(system)
	rawActors, _ := state[actorStateRoot].(map[string]any)
	for _, actor := range system.InitialActors {
		if current, exists := rawActors[actor.ID]; exists && current != nil {
			continue
		}
		template := actorStateTemplateByID(system, actor.TemplateID)
		if template.ID == "" {
			return fmt.Errorf("初始 Actor %s 的模板不存在: %s", actor.ID, actor.TemplateID)
		}
		ops, actorOps, _, err := buildNewActorStateOps(template, actor.ID, actor.Name, actor.Role, actor.Description, actor.State, reason, "")
		if err != nil {
			return err
		}
		for _, op := range ops {
			applyStateOp(state, op)
		}
		for _, op := range actorOps {
			applyActorStateOp(state, op)
		}
	}
	return nil
}
