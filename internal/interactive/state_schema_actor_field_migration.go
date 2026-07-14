package interactive

import (
	"fmt"
	"sort"
	"strings"
)

// splitStateSchemaActorChanges keeps whole-Actor migrations separate from
// field initializers so a field-level set can never replace unrelated state.
func splitStateSchemaActorChanges(adaptation ActorStateSchemaAdaptation) (map[string]ActorStateInitialActorSchemaOp, map[string]map[string]ActorStateRuntimeSchemaOp, error) {
	actorChanges := map[string]ActorStateInitialActorSchemaOp{}
	actorFieldChanges := map[string]map[string]ActorStateRuntimeSchemaOp{}
	for _, op := range adaptation.InitialActorOps {
		actorID := firstNonEmptyString(op.ActorID, op.Actor.ID)
		if actorID != "" {
			actorChanges[actorID] = op
		}
	}
	for _, op := range adaptation.ActorOps {
		actorID := firstNonEmptyString(op.ActorID, op.Actor.ID)
		if actorID == "" {
			continue
		}
		if op.Op == "set" {
			fieldID := normalizeActorStateFieldName(op.FieldID)
			if actorFieldChanges[actorID] == nil {
				actorFieldChanges[actorID] = map[string]ActorStateRuntimeSchemaOp{}
			}
			actorFieldChanges[actorID][fieldID] = op
			continue
		}
		actorChanges[actorID] = ActorStateInitialActorSchemaOp{Op: op.Op, ActorID: actorID, Actor: op.Actor, Reason: op.Reason, ValueSource: op.ValueSource}
	}
	for actorID := range actorFieldChanges {
		if _, exists := actorChanges[actorID]; exists {
			return nil, nil, fmt.Errorf("Actor %s 不能同时执行整体迁移和字段初始化", actorID)
		}
	}
	return actorChanges, actorFieldChanges, nil
}

func sortedStateSchemaActorFieldChangeIDs(changes map[string]ActorStateRuntimeSchemaOp) []string {
	fieldIDs := make([]string, 0, len(changes))
	for fieldID := range changes {
		fieldIDs = append(fieldIDs, fieldID)
	}
	sort.Strings(fieldIDs)
	return fieldIDs
}

func buildStateSchemaActorFieldInitializationOps(template ActorStateTemplate, actorID string, fieldIDs []string, changes map[string]ActorStateRuntimeSchemaOp, sourceTurnID string) ([]ActorStateOp, error) {
	ops := make([]ActorStateOp, 0, len(fieldIDs))
	for _, fieldID := range fieldIDs {
		change := changes[fieldID]
		field, ok := actorStateFieldByID(template, fieldID)
		if !ok {
			return nil, fmt.Errorf("Actor 字段初始化目标不存在: actor=%s template=%s field=%s", actorID, template.ID, fieldID)
		}
		value, err := normalizeActorStateValue(field, change.Value)
		if err != nil {
			return nil, fmt.Errorf("Actor %s 字段初始化失败: %w", actorID, err)
		}
		ops = append(ops, ActorStateOp{
			Op: "set", ActorID: actorID, FieldID: actorStateFieldID(field), Value: value,
			Reason: change.Reason, SourceID: stateSchemaActorValueSourceID(change.ValueSource), SourceTurnID: sourceTurnID,
		})
	}
	return ops, nil
}

func applyStateSchemaActorFieldProvenance(ops []ActorStateOp, changes map[string]ActorStateRuntimeSchemaOp) {
	for index := range ops {
		if change, ok := changes[ops[index].FieldID]; ok {
			ops[index].Reason = change.Reason
			ops[index].SourceID = stateSchemaActorValueSourceID(change.ValueSource)
		}
	}
}

func stateSchemaActorValueSourceID(source *ActorStateSchemaActorValueSource) string {
	if source == nil {
		return ""
	}
	return trimBytes(strings.TrimSpace(source.SourceID), 128)
}
