package app

import (
	"context"
	"denova/config"
	platformapp "denova/internal/app/platform"
	"denova/internal/interactive"
	"denova/internal/interactive/teller"
	"denova/internal/platform"
	"denova/internal/project"
	"denova/internal/style"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Integration fixtures keep native App access without exposing test-only APIs.
type platformTestStoryHost struct {
	*platformapp.Stories
	app *App
}

func newPlatformTestStoryHost(a *App) platformTestStoryHost {
	return platformTestStoryHost{Stories: platformapp.NewStories(platformHost{a}), app: a}
}

func newPlatformStateTestHost(t *testing.T) (platformTestStoryHost, platform.Scope, *interactive.Store) {
	t.Helper()
	root := t.TempDir()
	workspace := filepath.Join(root, "book")
	if err := os.MkdirAll(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	registry := project.NewRegistry(root)
	record, err := registry.Add(workspace, project.TypeBook, "State projection")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.EnsureStore(record); err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStore(workspace)
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "State projection"})
	if err != nil {
		t.Fatal(err)
	}
	a := &App{projectRegistry: registry, workspace: workspace, cfg: &config.Config{ProjectID: record.ID, NovaDir: root}, interactive: store}
	return newPlatformTestStoryHost(a), platform.Scope{ProjectID: record.ID, StoryID: story.ID, InstanceID: "test-state"}, store
}

func TestPlatformStoryPacingPreservesCommittedTurnsAndState(t *testing.T) {
	host, scope, store := newPlatformStateTestHost(t)
	if _, err := store.AppendTurn(scope.StoryID, interactive.AppendTurnRequest{Narrative: "The confirmed scene"}); err != nil {
		t.Fatal(err)
	}
	before, err := host.Snapshot(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	after, err := host.Tune(context.Background(), scope, platform.StoryPacing{ReplyTargetChars: 2000})
	if err != nil {
		t.Fatal(err)
	}
	if after.Configuration.ReplyTargetChars != 2000 || !reflect.DeepEqual(before.Turns, after.Turns) || !reflect.DeepEqual(before.State, after.State) || !reflect.DeepEqual(before.Branches, after.Branches) {
		t.Fatalf("pacing changed story facts: before=%+v after=%+v", before, after)
	}
	if _, err := host.Tune(context.Background(), scope, platform.StoryPacing{}); err == nil {
		t.Fatal("invalid pacing accepted")
	}
	other := scope
	other.ProjectID = "different-project"
	if _, err := host.Tune(context.Background(), other, platform.StoryPacing{ReplyTargetChars: 3000}); err == nil {
		t.Fatal("pacing escaped project authority")
	}
}

func TestPlatformStoryConfigurationFreezesNativeCastAndPresetSchema(t *testing.T) {
	host, scope, store := newPlatformStateTestHost(t)
	input := platform.StoryConfiguration{
		Origin: "Cultivation story", ModuleRefs: &platform.StoryModuleRefs{ActorStateID: interactive.ActorStateXiuxianID},
		Protagonist:     &platform.StoryProtagonist{Mode: "custom", Name: "Player", Profile: "An adult cultivator"},
		StateSchemaMode: "fixed_template", InitialActors: []platform.StoryInitialActor{{ID: "沈清霜", Name: "沈清霜", TemplateID: "important_character", State: map[string]any{"对主角好感度": float64(7)}}},
	}
	snapshot, err := host.Configure(context.Background(), scope, input)
	if err != nil {
		t.Fatal(err)
	}
	native, err := store.Snapshot(scope.StoryID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot.State, native.State) || snapshot.StateSchema == nil {
		t.Fatalf("public state differs from native state: %+v", snapshot)
	}
	actors := snapshot.State["actors"].(map[string]any)
	actor, exists := actors["沈清霜"].(map[string]any)
	if !exists || actor["template_id"] != "important_character" || actor["name"] != "沈清霜" {
		t.Fatalf("initial Actor mapping was not frozen: %+v", actors)
	}
	if actor["state"].(map[string]any)["对主角好感度"] != float64(7) {
		t.Fatalf("initial affection missing: %+v", actor)
	}
	presets, err := host.Presets(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, preset := range presets {
		kinds[preset.Kind] = true
		if preset.Kind == "state" && preset.ID == interactive.ActorStateXiuxianID {
			state := preset.Content.(interactive.StoryDirectorActorStateSystem)
			for _, initial := range state.InitialActors {
				if initial.ID == "沈清霜" {
					t.Fatal("Story cast mutated the reusable preset")
				}
			}
		}
	}
	if len(kinds) != 6 {
		t.Fatalf("missing preset category: %v", kinds)
	}
	encoded, err := json.Marshal(presets)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"path":`) || strings.Contains(string(encoded), host.app.cfg.DataDir()) {
		t.Fatal("preset projection leaked host paths")
	}
	// Repeating a pre-opening request remains one Story and one Actor per ID.
	if _, err := host.Configure(context.Background(), scope, input); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(scope.StoryID, interactive.AppendTurnRequest{Narrative: "The journey begins"}); err != nil {
		t.Fatal(err)
	}
	if _, err := host.Configure(context.Background(), scope, input); err == nil {
		t.Fatal("configuration rewrote an existing journey")
	}
}

func TestPlatformStoryHistoricalStateUsesNativeCheckpointWithoutFutureValues(t *testing.T) {
	host, scope, store := newPlatformStateTestHost(t)
	_, err := host.Configure(context.Background(), scope, platform.StoryConfiguration{ModuleRefs: &platform.StoryModuleRefs{ActorStateID: interactive.ActorStateXiuxianID}, InitialActors: []platform.StoryInitialActor{{ID: "沈清霜", Name: "沈清霜", TemplateID: "important_character"}}})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := store.AppendTurnWithState(scope.StoryID, interactive.AppendTurnWithStateRequest{BranchID: "main", Narrative: "A promise kept", ActorOps: []interactive.ActorStateOp{{Op: "set", ActorID: "沈清霜", FieldID: "对主角好感度", Value: float64(12), Reason: "Promise kept", SourceKind: "turn_result", SourceID: "promise"}}})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.AppendTurnWithState(scope.StoryID, interactive.AppendTurnWithStateRequest{BranchID: "main", Narrative: "A later sacrifice", ActorOps: []interactive.ActorStateOp{{Op: "set", ActorID: "沈清霜", FieldID: "对主角好感度", Value: float64(25)}}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := host.Snapshot(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	past, err := host.State(context.Background(), scope, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	value := past.State["actors"].(map[string]any)["沈清霜"].(map[string]any)["state"].(map[string]any)["对主角好感度"]
	if value != float64(12) || past.SourceRevision != interactive.TurnNarrativeRevision(first) {
		t.Fatalf("historical state used future values: %+v", past)
	}
	after, err := host.Snapshot(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	left, _ := json.Marshal(before)
	right, _ := json.Marshal(after)
	if string(left) != string(right) {
		t.Fatal("historical read mutated the current branch")
	}
	changes := after.Turns[0].StateChanges
	if len(changes) != 1 || changes[0].ActorID != "沈清霜" || changes[0].FieldID != "对主角好感度" || changes[0].Reason != "Promise kept" || changes[0].SourceID != "promise" {
		t.Fatalf("native provenance lost: %+v", changes)
	}
	branch, err := store.CreateBranch(scope.StoryID, interactive.CreateBranchRequest{ParentEventID: first.ID, Title: "Alternative"})
	if err != nil {
		t.Fatal(err)
	}
	scope.BranchID = branch.ID
	branchSnapshot, err := host.Snapshot(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, projected := range branchSnapshot.Branches {
		if projected.ID == branch.ID && (projected.ParentBranchID != "main" || projected.ParentTurnID != first.ID) {
			t.Fatalf("branch ancestry missing: %+v", projected)
		}
	}
	if _, err := host.State(context.Background(), scope, second.ID); err == nil {
		t.Fatal("historical query accepted a future turn from a sibling branch")
	}
	current, err := host.State(context.Background(), scope, "")
	if err != nil {
		t.Fatal(err)
	}
	currentJSON, _ := json.Marshal(current.State)
	pastJSON, _ := json.Marshal(past.State)
	if string(currentJSON) != string(pastJSON) {
		t.Fatal("historical projection differs from native branch checkpoint")
	}
}

func TestPlatformStoryConfigurationRejectsInvalidCastAndMissingPresetsWithoutMutation(t *testing.T) {
	host, scope, _ := newPlatformStateTestHost(t)
	before, err := host.Snapshot(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	validActor := platform.StoryInitialActor{ID: "沈清霜", Name: "沈清霜", TemplateID: "important_character"}
	cases := []platform.StoryConfiguration{
		{ModuleRefs: &platform.StoryModuleRefs{ActorStateID: "missing-template"}},
		{PlanningTemplateID: "missing-plan"},
		{StateSchemaMode: "unknown"},
		{InitialActors: []platform.StoryInitialActor{validActor, validActor}},
		{InitialActors: []platform.StoryInitialActor{{ID: "bad.id", Name: "Bad", TemplateID: "important_character"}}},
		{InitialActors: []platform.StoryInitialActor{{ID: "沈清霜", Name: "沈清霜", TemplateID: "missing"}}},
		{InitialActors: []platform.StoryInitialActor{{ID: "沈清霜", Name: "沈清霜", TemplateID: "important_character", State: map[string]any{"invented": 9}}}},
		{StateSchemaMode: "generate", InitialActors: []platform.StoryInitialActor{validActor}},
	}
	for index, input := range cases {
		if _, err := host.Configure(context.Background(), scope, input); err == nil {
			t.Fatalf("invalid case %d accepted", index)
		}
	}
	after, err := host.Snapshot(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	left, _ := json.Marshal(before)
	right, _ := json.Marshal(after)
	if string(left) != string(right) {
		t.Fatal("rejected opening configuration mutated Story")
	}
}

func TestPlatformStoryPortablePresetsReuseExactNativeCopies(t *testing.T) {
	host, scope, _ := newPlatformStateTestHost(t)
	recipient, recipientScope, _ := newPlatformStateTestHost(t)
	presets, err := host.Presets(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, preset := range presets {
		if preset.Kind == "state" || preset.Kind == "rules" || seen[preset.Kind] {
			continue
		}
		seen[preset.Kind] = true
		t.Run(preset.Kind, func(t *testing.T) {
			content, _ := json.Marshal(preset.Content)
			input := platform.StoryPresetImport{Kind: preset.Kind, Name: preset.Name, Description: preset.Description, Content: content}
			first, err := host.ImportPreset(context.Background(), scope, input)
			if err != nil {
				t.Fatal(err)
			}
			second, err := host.ImportPreset(context.Background(), scope, input)
			if err != nil {
				t.Fatal(err)
			}
			portable, err := recipient.ImportPreset(context.Background(), recipientScope, input)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(first.ID, "imported-") || len(first.ID) != 73 || first.ID != second.ID || first.Revision != second.Revision || first.ID != portable.ID {
				t.Fatalf("import was not stable across retry and recipient: %+v %+v %+v", first, second, portable)
			}
			if preset.Kind == "planning" {
				library := interactive.NewGamePlanningTemplateLibrary(host.app.cfg.DataDir())
				native, err := library.Get(first.ID)
				if err != nil {
					t.Fatal(err)
				}
				native.Name = "Locally edited"
				if _, err := library.Update(native.ID, native, native.Revision); err != nil {
					t.Fatal(err)
				}
				if _, err := host.ImportPreset(context.Background(), scope, input); err == nil {
					t.Fatal("import silently reused or overwrote changed native content")
				}
				kept, err := library.Get(native.ID)
				if err != nil || kept.Name != "Locally edited" {
					t.Fatalf("import overwrote user content: %+v %v", kept, err)
				}
			}
		})
	}
	if len(seen) != 4 {
		t.Fatalf("expected all four native import libraries, got %v", seen)
	}
}

func TestPlatformStoryPortableNarrativeEmbedsSharedStyleContent(t *testing.T) {
	host, scope, _ := newPlatformStateTestHost(t)
	dataDir := host.app.cfg.DataDir()
	reference, err := style.NewLibrary(dataDir).Create(style.WriteRequest{Name: "Quiet prose", Content: "Use concrete observations and restrained dialogue."})
	if err != nil {
		t.Fatal(err)
	}
	_, err = teller.NewLibrary(dataDir).Create(teller.Definition{ID: "portable-test", Name: "Portable test", StyleRefs: []string{reference.DisplayPath}, StyleRules: []teller.StyleRule{{Scene: "conversation", StyleRefs: []string{reference.DisplayPath}}}, Slots: []teller.PromptSlot{{ID: "system", Name: "System", Target: "system", Enabled: true, Content: "Maintain continuity."}}})
	if err != nil {
		t.Fatal(err)
	}
	presets, err := host.Presets(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, preset := range presets {
		if preset.Kind != "narrative" || preset.ID != "portable-test" {
			continue
		}
		var content struct {
			StyleRefs  []string           `json:"styleRefs"`
			StyleRules []teller.StyleRule `json:"styleRules"`
		}
		encodedContent, err := json.Marshal(preset.Content)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(encodedContent, &content); err != nil {
			t.Fatal(err)
		}
		if len(content.StyleRefs) != 0 || len(content.StyleRules) != 2 || content.StyleRules[0].Scene != "global" {
			t.Fatalf("style rules changed meaning: %+v", content)
		}
		for _, rule := range content.StyleRules {
			if len(rule.StyleRefs) != 0 || len(rule.StyleContents) != 1 || !strings.Contains(rule.StyleContents[0], "restrained dialogue") {
				t.Fatalf("style content not embedded: %+v", rule)
			}
		}
		recipient, recipientScope, _ := newPlatformStateTestHost(t)
		if _, err := recipient.ImportPreset(context.Background(), recipientScope, platform.StoryPresetImport{Kind: "narrative", Name: preset.Name, Content: encodedContent}); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatal("custom narrative preset missing")
}

func TestPlatformStoryPortableStateAndRulesFreezeWithoutAuthorLibraries(t *testing.T) {
	host, scope, store := newPlatformStateTestHost(t)
	library := interactive.NewActorStateLibrary(host.app.cfg.DataDir())
	native, err := library.Get(interactive.ActorStateXiuxianID)
	if err != nil {
		t.Fatal(err)
	}
	state, _ := json.Marshal(native.ActorState)
	input := platform.StoryConfiguration{Origin: "Portable work", StatePreset: state, RulePreset: json.RawMessage(`{"rule_templates":[]}`), ModuleRefs: &platform.StoryModuleRefs{ActorStateID: "author-custom", RuleSystemID: "author-rules"}, InitialActors: []platform.StoryInitialActor{{ID: "沈清霜", Name: "沈清霜", TemplateID: "important_character", State: map[string]any{"对主角好感度": float64(17)}}}}
	result, err := host.Configure(context.Background(), scope, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Configuration.StatePreset) == 0 || len(result.Configuration.RulePreset) == 0 || !result.Configuration.ModuleRefs.RuleSystemDisabled {
		t.Fatalf("configuration omitted frozen snapshots or resurrected default rules: %+v", result.Configuration)
	}
	if got := result.State["actors"].(map[string]any)["沈清霜"].(map[string]any)["state"].(map[string]any)["对主角好感度"]; got != float64(17) {
		t.Fatalf("initial actor was not frozen: %v", got)
	}
	nativeSnapshot, err := store.Snapshot(scope.StoryID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(nativeSnapshot.ActorStateSchema.TRPGSystem.RuleTemplates) != 0 {
		t.Fatal("frozen rule snapshot retained default rules")
	}
	before, _ := json.Marshal(result)
	for _, invalid := range []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`{}`), json.RawMessage(`{"path":"private"}`)} {
		input.StatePreset = invalid
		if _, err := host.Configure(context.Background(), scope, input); err == nil {
			t.Fatalf("invalid state snapshot accepted: %s", invalid)
		}
	}
	after, err := host.Snapshot(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(after)
	if string(before) != string(encoded) {
		t.Fatal("invalid snapshot mutated opening")
	}
}

func TestPlatformStoryPortableImportRejectsRuntimeAndLocalReferenceFields(t *testing.T) {
	host, scope, _ := newPlatformStateTestHost(t)
	for _, input := range []platform.StoryPresetImport{
		{Kind: "state", Name: "Unsupported", Content: json.RawMessage(`{}`)},
		{Kind: "image", Name: "Private path", Content: json.RawMessage(`{"prompt":"x","path":"secret"}`)},
		{Kind: "narrative", Name: "Unresolved", Content: json.RawMessage(`{"styleRefs":["styles/local.md"]}`)},
		{Kind: "planning", Name: "Null", Content: json.RawMessage(`null`)},
	} {
		if _, err := host.ImportPreset(context.Background(), scope, input); err == nil {
			t.Fatalf("unsafe portable input accepted: %+v", input)
		}
	}
}

func TestPlatformStoryPortableRulesPreserveBindingsAndRejectUnknownFields(t *testing.T) {
	host, scope, store := newPlatformStateTestHost(t)
	system, err := interactive.NewActorStateLibrary(host.app.cfg.DataDir()).Get(interactive.ActorStateXiuxianID)
	if err != nil {
		t.Fatal(err)
	}
	state, _ := json.Marshal(system.ActorState)
	rules := interactive.DefaultRuleSystemModule().TRPGSystem
	rules.RuleTemplates[0].StateBindings = []interactive.RuleStateBinding{{ID: "relationship", ActorTemplateID: "important_character", Modifiers: []interactive.RuleStateBindingModifier{{Source: "actor", FieldID: "对主角好感度", Effect: "advantage", Scale: 0.1}}}}
	raw, _ := json.Marshal(rules)
	input := platform.StoryConfiguration{Origin: "Portable rules", StatePreset: state, RulePreset: raw}
	result, err := host.Configure(context.Background(), scope, input)
	if err != nil {
		t.Fatal(err)
	}
	native, err := store.Snapshot(scope.StoryID, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Configuration.ModuleRefs.RuleSystemDisabled || native.ActorStateSchema.TRPGSystem.RuleTemplates[0].StateBindings[0].Modifiers[0].FieldID != "对主角好感度" {
		t.Fatal("native frozen rules lost binding")
	}
	rules.RuleTemplates[0].StateBindings[0].Modifiers[0].FieldID = "Unknown"
	input.RulePreset, _ = json.Marshal(rules)
	if _, err := host.Configure(context.Background(), scope, input); err == nil {
		t.Fatal("broken rule reference was frozen")
	}
	input.RulePreset = json.RawMessage(`{"rule_templates":[],"path":"private"}`)
	if _, err := host.Configure(context.Background(), scope, input); err == nil {
		t.Fatal("private rule fields accepted")
	}
	input.RulePreset = raw
	input.ModuleRefs = &platform.StoryModuleRefs{ActorStateDisabled: true}
	if _, err := host.Configure(context.Background(), scope, input); err == nil {
		t.Fatal("snapshot silently enabled an explicitly disabled state system")
	}
	input.ModuleRefs = &platform.StoryModuleRefs{RuleSystemDisabled: true}
	if _, err := host.Configure(context.Background(), scope, input); err == nil {
		t.Fatal("snapshot silently enabled an explicitly disabled rule system")
	}
}
