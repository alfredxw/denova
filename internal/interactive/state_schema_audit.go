package interactive

// stateSchemaAdaptationChanges builds the bounded user-visible audit trail for
// schema and Actor migrations accepted during one review.
func stateSchemaAdaptationChanges(adaptation ActorStateSchemaAdaptation) []ActorStateSchemaAdaptationChange {
	changes := make([]ActorStateSchemaAdaptationChange, 0, maxActorStateSchemaAdaptationOps)
	for _, templateOp := range adaptation.TemplateOps {
		if len(templateOp.FieldOps) == 0 {
			targetID := templateOp.Template.ID
			changes = append(changes, ActorStateSchemaAdaptationChange{Kind: "template", Op: templateOp.Op, TemplateID: firstNonEmptyString(templateOp.TemplateID, targetID), TargetID: targetID, Reason: templateOp.Reason})
		}
		for _, fieldOp := range templateOp.FieldOps {
			changes = append(changes, ActorStateSchemaAdaptationChange{Kind: "field", Op: fieldOp.Op, TemplateID: templateOp.TemplateID, FieldID: fieldOp.FieldID, TargetID: actorStateFieldID(fieldOp.Field), Reason: fieldOp.Reason})
		}
	}
	for _, actorOp := range adaptation.InitialActorOps {
		changes = append(changes, ActorStateSchemaAdaptationChange{Kind: "actor", Op: actorOp.Op, ActorID: firstNonEmptyString(actorOp.ActorID, actorOp.Actor.ID), TargetID: actorOp.Actor.TemplateID, Reason: actorOp.Reason, ValueSource: actorOp.ValueSource})
	}
	for _, actorOp := range adaptation.ActorOps {
		kind := "actor"
		if actorOp.Op == "set" {
			kind = "actor_field"
		}
		changes = append(changes, ActorStateSchemaAdaptationChange{Kind: kind, Op: actorOp.Op, ActorID: firstNonEmptyString(actorOp.ActorID, actorOp.Actor.ID), FieldID: actorOp.FieldID, TargetID: firstNonEmptyString(actorOp.FieldID, actorOp.Actor.TemplateID), Reason: actorOp.Reason, ValueSource: actorOp.ValueSource})
	}
	if len(changes) > maxActorStateSchemaAdaptationOps {
		changes = changes[:maxActorStateSchemaAdaptationOps]
	}
	return changes
}
