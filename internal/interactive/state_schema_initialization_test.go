package interactive

import "testing"

func TestBuildStateSchemaMigrationPreservesExistingValuesAndSkipsMissingDefaults(t *testing.T) {
	base := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			{ID: "protagonist", Fields: []ActorStateField{{Name: "生命", Type: "number", Default: 100}}},
			{ID: ActorStateStoryContextTemplateID, Fields: []ActorStateField{{Name: "当前地点", Type: "string"}}},
		},
	}
	target := base
	target.Templates = append([]ActorStateTemplate(nil), base.Templates...)
	target.Templates[1].Fields = append(target.Templates[1].Fields, ActorStateField{Name: "未确认的威胁", Type: "string"})
	state := map[string]any{actorStateRoot: map[string]any{
		"protagonist": map[string]any{
			"id": "protagonist", "name": "林风", "template_id": "protagonist",
			"state": map[string]any{"生命": float64(73)},
		},
		DefaultStoryContextActorID: map[string]any{
			"id": DefaultStoryContextActorID, "template_id": ActorStateStoryContextTemplateID,
			"state": map[string]any{},
		},
	}}
	adaptation := ActorStateSchemaAdaptation{
		TemplateOps: []ActorStateTemplateSchemaOp{{
			Op: "fields", TemplateID: ActorStateStoryContextTemplateID,
			FieldOps: []ActorStateFieldSchemaOp{{Op: "add", Field: ActorStateField{Name: "未确认的威胁", Type: "string"}}},
		}},
		InitialActorOps: []ActorStateInitialActorSchemaOp{{
			Op: "replace", ActorID: "protagonist",
			Actor: ActorStateInitialActor{ID: "protagonist", TemplateID: "protagonist", State: map[string]any{"生命": nil}},
		}},
	}
	_, actorOps, _, _, err := buildStateSchemaMigration(base, target, state, adaptation, "turn-opening")
	if err != nil {
		t.Fatal(err)
	}
	sets := map[string]ActorStateOp{}
	for _, op := range actorOps {
		if op.Op != "set" {
			continue
		}
		if op.Value == nil {
			t.Fatalf("schema migration must not generate set operations without a value: %#v", actorOps)
		}
		key := op.ActorID + "/" + op.FieldID
		if previous, exists := sets[key]; exists {
			t.Fatalf("schema migration emitted duplicate sets for %s: previous=%#v current=%#v all=%#v", key, previous, op, actorOps)
		}
		sets[key] = op
	}
	if got := sets["protagonist/生命"].Value; got != float64(73) {
		t.Fatalf("null or omitted replacement state must preserve the existing confirmed value, got %#v in %#v", got, actorOps)
	}
	if _, exists := sets[DefaultStoryContextActorID+"/未确认的威胁"]; exists {
		t.Fatalf("a story field with neither a current nor default value must stay absent: %#v", actorOps)
	}
}

func TestBuildStateSchemaMigrationExplicitValueOverridesExistingOnce(t *testing.T) {
	base := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Fields: []ActorStateField{{Name: "生命", Type: "number", Default: 100}},
	}}}
	state := map[string]any{actorStateRoot: map[string]any{
		"protagonist": map[string]any{"template_id": "protagonist", "state": map[string]any{"生命": float64(73)}},
	}}
	adaptation := ActorStateSchemaAdaptation{InitialActorOps: []ActorStateInitialActorSchemaOp{{
		Op: "replace", ActorID: "protagonist",
		Actor: ActorStateInitialActor{ID: "protagonist", TemplateID: "protagonist", State: map[string]any{"生命": float64(61)}},
	}}}
	_, actorOps, _, _, err := buildStateSchemaMigration(base, base, state, adaptation, "turn-opening")
	if err != nil {
		t.Fatal(err)
	}
	var lifeSets []ActorStateOp
	for _, op := range actorOps {
		if op.Op == "set" && op.ActorID == "protagonist" && op.FieldID == "生命" {
			lifeSets = append(lifeSets, op)
		}
	}
	if len(lifeSets) != 1 || lifeSets[0].Value != float64(61) {
		t.Fatalf("an explicit non-null replacement must override the current/default value exactly once: %#v", actorOps)
	}
}

func TestBuildStateSchemaMigrationAppliesFieldLevelActorInitialization(t *testing.T) {
	base := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{{
			ID: DefaultActorID, Fields: []ActorStateField{{Name: "当前资源", Type: "object"}},
		}},
		InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, TemplateID: DefaultActorID}},
	}
	target := base
	target.Templates = append([]ActorStateTemplate(nil), base.Templates...)
	target.Templates[0].Fields = append([]ActorStateField(nil), base.Templates[0].Fields...)
	target.Templates[0].Fields = append(target.Templates[0].Fields, ActorStateField{Name: "境界", Type: "string"})
	state := map[string]any{actorStateRoot: map[string]any{
		DefaultActorID: map[string]any{
			"id": DefaultActorID, "template_id": DefaultActorID,
			"state": map[string]any{"当前资源": map[string]any{"下品灵石": float64(3)}},
		},
	}}
	source := &ActorStateSchemaActorValueSource{
		SourceID: "state_schema_batch:protagonist-realm", ItemID: "protagonist-realm",
		Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-turn"},
	}
	adaptation := ActorStateSchemaAdaptation{
		TemplateOps: []ActorStateTemplateSchemaOp{{
			Op: "fields", TemplateID: DefaultActorID,
			FieldOps: []ActorStateFieldSchemaOp{{Op: "add", Field: ActorStateField{Name: "境界", Type: "string"}}},
		}},
		ActorOps: []ActorStateRuntimeSchemaOp{{
			Op: "set", ActorID: DefaultActorID, FieldID: "境界", Value: "筑基初期", Reason: "开局正文明确", ValueSource: source,
		}},
	}

	_, actorOps, _, _, err := buildStateSchemaMigration(base, target, state, adaptation, "opening-turn")
	if err != nil {
		t.Fatal(err)
	}
	sets := map[string]ActorStateOp{}
	for _, op := range actorOps {
		if op.Op != "set" || op.ActorID != DefaultActorID {
			continue
		}
		if _, exists := sets[op.FieldID]; exists {
			t.Fatalf("field-level initialization must not duplicate migration sets: %#v", actorOps)
		}
		sets[op.FieldID] = op
	}
	if got := sets["境界"]; got.Value != "筑基初期" || got.SourceID != source.SourceID {
		t.Fatalf("field-level initialization was not materialized with provenance: %#v", actorOps)
	}
	if got := sets["当前资源"].Value; got == nil {
		t.Fatalf("existing state must survive the same schema migration: %#v", actorOps)
	}
}

func TestBuildStateSchemaMigrationKeepsBatchActorValueSourceID(t *testing.T) {
	base := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "npc", Fields: []ActorStateField{{Name: "态度", Type: "string", Default: "中立"}},
	}}}
	source := &ActorStateSchemaActorValueSource{
		SourceID: "state_schema_batch:guide-attitude", ItemID: "guide-attitude",
		Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "turn-opening"},
	}
	adaptation := ActorStateSchemaAdaptation{ActorOps: []ActorStateRuntimeSchemaOp{{
		Op: "add", ActorID: "guide", Actor: ActorStateInitialActor{
			ID: "guide", Name: "向导", TemplateID: "npc", State: map[string]any{"态度": "警觉"},
		}, ValueSource: source,
	}}}
	ops, actorOps, _, _, err := buildStateSchemaMigration(base, base, map[string]any{actorStateRoot: map[string]any{}}, adaptation, "turn-opening")
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) == 0 || len(actorOps) == 0 {
		t.Fatalf("actor creation should emit metadata and state ops: ops=%#v actor_ops=%#v", ops, actorOps)
	}
	for _, op := range ops {
		if op.SourceID != source.SourceID {
			t.Fatalf("metadata op lost Batch provenance: %#v", op)
		}
	}
	for _, op := range actorOps {
		if op.SourceID != source.SourceID {
			t.Fatalf("actor value op lost Batch provenance: %#v", op)
		}
	}
	changes := stateSchemaAdaptationChanges(adaptation)
	if len(changes) != 1 || changes[0].ValueSource == nil || changes[0].ValueSource.SourceID != source.SourceID {
		t.Fatalf("persisted schema adaptation change lost Actor evidence association: %#v", changes)
	}
}

func TestBuildStateSchemaMigrationRejectsLossyFallbackOverExistingValue(t *testing.T) {
	base := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Fields: []ActorStateField{{Name: "境界", Type: "string", Default: "未知"}},
	}}}
	target := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Fields: []ActorStateField{{Name: "境界", Type: "number", Default: 0}},
	}}}
	state := map[string]any{actorStateRoot: map[string]any{
		"protagonist": map[string]any{"template_id": "protagonist", "state": map[string]any{"境界": "筑基中期"}},
	}}
	adaptation := ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{
		Op: "fields", TemplateID: "protagonist",
		FieldOps: []ActorStateFieldSchemaOp{{Op: "replace", FieldID: "境界", Field: ActorStateField{Name: "境界", Type: "number", Default: 0}}},
	}}}
	if _, _, _, _, err := buildStateSchemaMigration(base, target, state, adaptation, "turn-opening"); err == nil {
		t.Fatal("an incompatible confirmed value must require an explicit migration value instead of silently falling back to a default")
	}
}
