package interactive

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHistoricalActorStateFieldVisibilityIsIgnored(t *testing.T) {
	var system StoryDirectorActorStateSystem
	if err := json.Unmarshal([]byte(`{
		"templates":[{"id":"protagonist","fields":[{"name":"秘密身份","type":"string","visibility":"hidden"}]}],
		"initial_actors":[{"id":"protagonist","name":"主角","template_id":"protagonist","state":{"秘密身份":"继承人"}}]
	}`), &system); err != nil {
		t.Fatalf("decode historical actor state system: %v", err)
	}

	context := ActorStateRuntimeContext(system, nil, DirectorContextMaxBytes)
	if !strings.Contains(context, "秘密身份") || !strings.Contains(context, "继承人") {
		t.Fatalf("historical visibility must not hide actor state fields from runtime context: %s", context)
	}

	encoded, err := json.Marshal(normalizeActorStateSystem(system))
	if err != nil {
		t.Fatalf("encode normalized actor state system: %v", err)
	}
	if strings.Contains(string(encoded), `"visibility"`) {
		t.Fatalf("historical visibility must not survive normalization: %s", encoded)
	}
}

func TestHistoricalActorTraitVisibilityIsIgnored(t *testing.T) {
	var system StoryDirectorActorStateSystem
	if err := json.Unmarshal([]byte(`{
		"templates":[{"id":"protagonist","trait_rules":[{"pool_id":"nature","draw_count":1}]}],
		"trait_pools":[{"id":"nature","name":"性格","traits":[{"id":"patient","name":"耐心","summary":"善于等待","weight":1,"visibility":"hidden"}]}]
	}`), &system); err != nil {
		t.Fatalf("decode historical actor trait system: %v", err)
	}
	result, err := RollActorTraits(system, ActorTraitRollRequest{
		ActorID: "protagonist", TemplateID: "protagonist",
		Selections: []ActorTraitSelection{{PoolID: "nature", TraitIDs: []string{"patient"}}}, Seed: 1,
	})
	if err != nil {
		t.Fatalf("roll historical actor trait: %v", err)
	}

	state := map[string]any{"actors": map[string]any{"protagonist": map[string]any{
		"name": "主角", "template_id": "protagonist", "state": map[string]any{}, "traits": result.Traits,
	}}}
	context := ActorStateRuntimeContext(system, state, DirectorContextMaxBytes)
	if !strings.Contains(context, "耐心") || !strings.Contains(context, "善于等待") {
		t.Fatalf("historical visibility must not hide assigned traits from runtime context: %s", context)
	}

	encoded, err := json.Marshal(struct {
		System StoryDirectorActorStateSystem `json:"system"`
		Traits []ActorTraitInstance          `json:"traits"`
	}{System: normalizeActorStateSystem(system), Traits: result.Traits})
	if err != nil {
		t.Fatalf("encode normalized actor traits: %v", err)
	}
	if strings.Contains(string(encoded), `"visibility"`) {
		t.Fatalf("historical trait visibility must not survive normalization: %s", encoded)
	}
}
