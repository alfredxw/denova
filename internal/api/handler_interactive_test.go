package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/interactive"
)

func TestInteractiveStoriesAndTellersAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	listResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories", nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list stories status = %d body=%s", listResp.Code, listResp.Body.String())
	}
	var initial struct {
		CurrentStoryID string `json:"current_story_id"`
		Stories        []any  `json:"stories"`
	}
	decodeResponse(t, listResp.Body.Bytes(), &initial)
	if initial.CurrentStoryID != "" || len(initial.Stories) != 0 {
		t.Fatalf("initial stories should be empty: %#v", initial)
	}

	createResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]string{
		"title":           "末日开端",
		"origin":          "主角醒来发现世界已末日",
		"story_teller_id": "classic",
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create story status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		ID            string `json:"id"`
		Title         string `json:"title"`
		StoryTellerID string `json:"story_teller_id"`
	}
	decodeResponse(t, createResp.Body.Bytes(), &created)
	if created.ID == "" || created.Title != "末日开端" || created.StoryTellerID != "classic" {
		t.Fatalf("created story mismatch: %#v", created)
	}

	snapshotResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+created.ID+"/snapshot", nil)
	if snapshotResp.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d body=%s", snapshotResp.Code, snapshotResp.Body.String())
	}
	var snapshot struct {
		StoryID            string                          `json:"story_id"`
		BranchID           string                          `json:"branch_id"`
		Turns              []any                           `json:"turns"`
		DirectorPlanStatus *interactive.DirectorPlanStatus `json:"director_plan_status"`
	}
	decodeResponse(t, snapshotResp.Body.Bytes(), &snapshot)
	if snapshot.StoryID != created.ID || snapshot.BranchID != "main" || len(snapshot.Turns) != 0 {
		t.Fatalf("snapshot mismatch: %#v", snapshot)
	}
	if snapshot.DirectorPlanStatus == nil || snapshot.DirectorPlanStatus.Status != interactive.DirectorPlanStatusWaitingOpening || snapshot.DirectorPlanStatus.Blocking {
		t.Fatalf("new story snapshot should expose waiting director status without blocking: %#v", snapshot.DirectorPlanStatus)
	}
	var rawSnapshot map[string]json.RawMessage
	decodeResponse(t, snapshotResp.Body.Bytes(), &rawSnapshot)
	if _, ok := rawSnapshot["director_plan"]; ok {
		t.Fatalf("snapshot must not expose full director plan docs: %s", snapshotResp.Body.String())
	}

	if _, err := application.AppendInteractiveTurn(created.ID, "", "我推开酒馆的门", "门后传来低沉的风声。"); err != nil {
		t.Fatal(err)
	}
	memoryCreateResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+created.ID+"/memory", map[string]any{
		"branch_id":  "main",
		"title":      "酒馆风声",
		"summary":    "门后传来低沉风声。",
		"people":     []string{"主角"},
		"places":     []string{"酒馆"},
		"tags":       []string{"线索"},
		"importance": 4,
	})
	if memoryCreateResp.Code != http.StatusOK {
		t.Fatalf("create memory status = %d body=%s", memoryCreateResp.Code, memoryCreateResp.Body.String())
	}
	var memoryEntry struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Manual bool   `json:"manual"`
	}
	decodeResponse(t, memoryCreateResp.Body.Bytes(), &memoryEntry)
	if memoryEntry.ID == "" || memoryEntry.Title != "酒馆风声" || !memoryEntry.Manual {
		t.Fatalf("memory entry mismatch: %#v", memoryEntry)
	}
	memoryListResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+created.ID+"/memory?branch=main", nil)
	if memoryListResp.Code != http.StatusOK {
		t.Fatalf("list memory status = %d body=%s", memoryListResp.Code, memoryListResp.Body.String())
	}
	var memoryList struct {
		Entries []struct {
			ID string `json:"id"`
		} `json:"entries"`
	}
	decodeResponse(t, memoryListResp.Body.Bytes(), &memoryList)
	if len(memoryList.Entries) != 1 || memoryList.Entries[0].ID != memoryEntry.ID {
		t.Fatalf("memory list mismatch: %#v", memoryList)
	}
	archiveResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+created.ID+"/memory/"+memoryEntry.ID+"/archive", map[string]bool{"archived": true})
	if archiveResp.Code != http.StatusOK {
		t.Fatalf("archive memory status = %d body=%s", archiveResp.Code, archiveResp.Body.String())
	}
	snapshotResp = performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+created.ID+"/snapshot", nil)
	decodeResponse(t, snapshotResp.Body.Bytes(), &snapshot)
	if len(snapshot.Turns) != 1 {
		t.Fatalf("chat should persist one turn: %#v", snapshot)
	}

	branchResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+created.ID+"/branches", map[string]string{
		"parent_event_id": snapshot.Turns[0].(map[string]any)["id"].(string),
		"title":           "换条路走",
	})
	if branchResp.Code != http.StatusOK {
		t.Fatalf("branch status = %d body=%s", branchResp.Code, branchResp.Body.String())
	}
	var branch struct {
		ID string `json:"id"`
	}
	decodeResponse(t, branchResp.Body.Bytes(), &branch)
	if branch.ID == "" {
		t.Fatalf("branch id should not be empty: %#v", branch)
	}

	patchResp := performJSONRequest(t, server, http.MethodPatch, "/api/interactive/stories/"+created.ID, map[string]string{
		"title":           "新标题",
		"story_teller_id": "grimdark",
	})
	if patchResp.Code != http.StatusOK {
		t.Fatalf("patch status = %d body=%s", patchResp.Code, patchResp.Body.String())
	}

	deleteResp := performJSONRequest(t, server, http.MethodDelete, "/api/interactive/stories/"+created.ID, nil)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", deleteResp.Code, deleteResp.Body.String())
	}

	tellersResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/tellers", nil)
	if tellersResp.Code != http.StatusOK {
		t.Fatalf("list tellers status = %d body=%s", tellersResp.Code, tellersResp.Body.String())
	}
	var tellersBody struct {
		Tellers []struct {
			ID string `json:"id"`
		} `json:"tellers"`
	}
	decodeResponse(t, tellersResp.Body.Bytes(), &tellersBody)
	if len(tellersBody.Tellers) < 3 {
		t.Fatalf("expected built-in tellers: %#v", tellersBody.Tellers)
	}

	classicResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/tellers/classic", nil)
	if classicResp.Code != http.StatusOK {
		t.Fatalf("get teller status = %d body=%s", classicResp.Code, classicResp.Body.String())
	}
	var classic struct {
		ID    string `json:"id"`
		Slots []struct {
			ID      string `json:"id"`
			Target  string `json:"target"`
			Content string `json:"content"`
		} `json:"slots"`
	}
	decodeResponse(t, classicResp.Body.Bytes(), &classic)
	if classic.ID != "classic" || len(classic.Slots) == 0 || classic.Slots[0].Content == "" {
		t.Fatalf("classic teller mismatch: %#v", classic)
	}
}

func TestInteractiveDirectorAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	createResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]string{
		"title":           "导演接口",
		"origin":          "主角准备参加学院大比",
		"story_teller_id": "classic",
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create story status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	decodeResponse(t, createResp.Body.Bytes(), &created)

	statusResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+created.ID+"/director/status", nil)
	if statusResp.Code != http.StatusOK {
		t.Fatalf("get director status endpoint = %d body=%s", statusResp.Code, statusResp.Body.String())
	}
	var status interactive.DirectorPlanStatus
	decodeResponse(t, statusResp.Body.Bytes(), &status)
	if status.Status != interactive.DirectorPlanStatusWaitingOpening || status.Blocking || status.StartReady || status.CompletedDocs != 0 || status.PlannedDocs != 1 {
		t.Fatalf("initial director status mismatch: %#v", status)
	}

	getResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+created.ID+"/director", nil)
	if getResp.Code != http.StatusOK {
		t.Fatalf("get director status = %d body=%s", getResp.Code, getResp.Body.String())
	}
	type directorResponse struct {
		Docs struct {
			Plan string `json:"plan"`
		} `json:"docs"`
		Metadata struct {
			Revision string `json:"revision"`
			LastRun  struct {
				Status string `json:"status"`
			} `json:"last_run"`
		} `json:"metadata"`
	}
	var director directorResponse
	decodeResponse(t, getResp.Body.Bytes(), &director)
	if director.Metadata.LastRun.Status != interactive.DirectorPlanStatusWaitingOpening || !strings.Contains(director.Docs.Plan, "正文Agent可读") {
		t.Fatalf("default director plan mismatch: %#v", director)
	}

	nextDocs := director.Docs
	nextDocs.Plan += "\n\n手动设置主线：学院逆袭主线。"
	patchResp := performJSONRequest(t, server, http.MethodPatch, "/api/interactive/stories/"+created.ID+"/director", map[string]any{
		"docs":          nextDocs,
		"base_revision": director.Metadata.Revision,
		"summary":       "手动设置主线",
	})
	if patchResp.Code != http.StatusOK {
		t.Fatalf("patch director status = %d body=%s", patchResp.Code, patchResp.Body.String())
	}
	decodeResponse(t, patchResp.Body.Bytes(), &director)
	if !strings.Contains(director.Docs.Plan, "学院逆袭主线") || director.Metadata.LastRun.Status != "ready" {
		t.Fatalf("director plan patch mismatch: %#v", director)
	}

	rebuildResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+created.ID+"/director/rebuild", nil)
	if rebuildResp.Code != http.StatusOK {
		t.Fatalf("rebuild director status = %d body=%s", rebuildResp.Code, rebuildResp.Body.String())
	}
	director = directorResponse{}
	decodeResponse(t, rebuildResp.Body.Bytes(), &director)
	if !strings.Contains(director.Docs.Plan, "正文Agent可读") || director.Metadata.LastRun.Status != "ready" {
		t.Fatalf("rebuilt director plan mismatch: %#v", director)
	}

	if _, err := application.AppendInteractiveTurn(created.ID, "", "我报名学院大比", "报名弟子把他的名字写进木牌。"); err != nil {
		t.Fatal(err)
	}
	if _, err := application.InteractiveDirectorPlanStatus(created.ID, "main"); err != nil {
		t.Fatal(err)
	}
	runResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+created.ID+"/director/run", map[string]string{"branch_id": "main"})
	if runResp.Code != http.StatusOK {
		t.Fatalf("run director status = %d body=%s", runResp.Code, runResp.Body.String())
	}
	status = waitForDirectorStatusAPI(t, server, created.ID, interactive.DirectorPlanStatusReady)
	if !status.StartReady || status.Blocking || status.CompletedDocs != status.PlannedDocs {
		t.Fatalf("manual director run should become ready: %#v", status)
	}
}

func TestInteractiveStoryCreateRollsBackWhenStateSchemaInitializationFails(t *testing.T) {
	application := newTestApplication(t)
	calls := 0
	restoreDirector := application.SetInteractiveDirectorGeneratorForTest(func(context.Context, *config.Config, *book.State, agent.InteractiveStoryToolContext, string) (string, error) {
		calls++
		return "", errors.New("director unavailable")
	})
	t.Cleanup(restoreDirector)
	server := NewServer(application, "0")

	createResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]string{
		"title":           "失败回滚",
		"origin":          "主角准备出发",
		"story_teller_id": "classic",
	})
	if createResp.Code != http.StatusBadRequest {
		t.Fatalf("create story should fail when state schema initialization fails status=%d body=%s", createResp.Code, createResp.Body.String())
	}
	if calls != 1 {
		t.Fatalf("state schema initializer should run once during story creation, calls=%d", calls)
	}

	listResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories", nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list stories status = %d body=%s", listResp.Code, listResp.Body.String())
	}
	var list struct {
		Stories []struct {
			ID string `json:"id"`
		} `json:"stories"`
	}
	decodeResponse(t, listResp.Body.Bytes(), &list)
	if len(list.Stories) != 0 {
		t.Fatalf("failed schema initialization must not persist a partial story: %#v", list)
	}
}

func TestInteractiveStoryCreateAdaptsAndFreezesStoryStateSchema(t *testing.T) {
	application := newTestApplication(t)
	var instruction string
	restoreDirector := application.SetInteractiveDirectorGeneratorForTest(func(_ context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, input string) (string, error) {
		if toolContext.MaintenanceTask != "state_schema_initialization" {
			return "测试后台导演完成。", nil
		}
		instruction = input
		return `{
			"summary":"为修仙群像与关系玩法补充长期可计算状态",
			"template_ops":[
				{"op":"fields","template_id":"protagonist","field_ops":[
					{"op":"add","field":{"name":"境界","type":"string","default":"炼气一层","visibility":"visible","description":"主角当前修行境界","order":110}},
					{"op":"add","field":{"name":"修为进度","type":"number","default":0,"min":0,"max":100,"visibility":"visible","description":"突破前的修为积累","order":120}},
					{"op":"add","field":{"name":"法宝","type":"list","default":[],"visibility":"visible","order":130}},
					{"op":"add","field":{"name":"功法","type":"list","default":[],"visibility":"visible","order":140}}
				]},
				{"op":"fields","template_id":"important_character","field_ops":[
					{"op":"add","field":{"name":"好感度","type":"number","default":0,"min":-100,"max":100,"visibility":"spoiler","order":110}},
					{"op":"add","field":{"name":"关系阶段","type":"enum","default":"陌生","options":["陌生","熟悉","暧昧","恋人"],"visibility":"spoiler","order":120}}
				]}
			],
			"initial_actor_ops":[]
		}`, nil
	})
	t.Cleanup(restoreDirector)
	server := NewServer(application, "0")

	createResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]string{
		"title":           "青云问情录",
		"origin":          "主角踏入修仙宗门，将与多名已经成年的重要角色发展不同关系，并通过修炼和法宝探索秘境。",
		"story_teller_id": "classic",
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create adapted story status=%d body=%s", createResp.Code, createResp.Body.String())
	}
	if !strings.Contains(instruction, "青云问情录") || !strings.Contains(instruction, "state_preset") || !strings.Contains(instruction, "max_prompt_bytes") {
		t.Fatalf("initializer instruction must contain bounded sourced context: %s", instruction)
	}
	var created interactive.StorySummary
	decodeResponse(t, createResp.Body.Bytes(), &created)
	snapshotResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+created.ID+"/snapshot", nil)
	if snapshotResp.Code != http.StatusOK {
		t.Fatalf("snapshot status=%d body=%s", snapshotResp.Code, snapshotResp.Body.String())
	}
	var snapshot interactive.Snapshot
	decodeResponse(t, snapshotResp.Body.Bytes(), &snapshot)
	if snapshot.ActorStateSchema == nil || snapshot.ActorStateSchema.Version != interactive.ActorStateSchemaVersion || snapshot.ActorStateSchema.Adaptation == nil {
		t.Fatalf("adapted schema audit missing: %#v", snapshot.ActorStateSchema)
	}
	if snapshot.ActorStateSchema.Adaptation.FieldOps != 6 || snapshot.ActorStateSchema.Adaptation.Source != "director_agent" {
		t.Fatalf("adaptation audit mismatch: %#v", snapshot.ActorStateSchema.Adaptation)
	}
	templateFields := map[string]map[string]bool{}
	for _, template := range snapshot.ActorStateSchema.System.Templates {
		templateFields[template.ID] = map[string]bool{}
		for _, field := range template.Fields {
			templateFields[template.ID][field.Name] = true
		}
	}
	for _, fieldID := range []string{"境界", "修为进度", "法宝", "功法"} {
		if !templateFields["protagonist"][fieldID] {
			t.Fatalf("protagonist schema missing %s: %#v", fieldID, templateFields["protagonist"])
		}
	}
	for _, fieldID := range []string{"好感度", "关系阶段"} {
		if !templateFields["important_character"][fieldID] {
			t.Fatalf("important character schema missing %s: %#v", fieldID, templateFields["important_character"])
		}
	}
	actors, _ := snapshot.State["actors"].(map[string]any)
	protagonist, _ := actors["protagonist"].(map[string]any)
	state, _ := protagonist["state"].(map[string]any)
	if state["境界"] != "炼气一层" || state["修为进度"] != float64(0) {
		t.Fatalf("adapted defaults must materialize with initial actor state: %#v", state)
	}
}

func TestInteractiveStoryCreateCanDisableStateSchemaAdaptation(t *testing.T) {
	application := newTestApplication(t)
	director, err := application.CreateStoryDirector(interactive.StoryDirector{
		ID:         "preset-only-director",
		Name:       "直接使用预设",
		ModuleRefs: interactive.DefaultStoryDirectorModuleRefs(),
		Strategy: interactive.StoryDirectorStrategy{
			Enabled:                   true,
			StateSchemaAdaptationMode: interactive.StateSchemaAdaptationModeOff,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	restoreDirector := application.SetInteractiveDirectorGeneratorForTest(func(context.Context, *config.Config, *book.State, agent.InteractiveStoryToolContext, string) (string, error) {
		calls++
		return "", errors.New("state schema initializer must stay disabled")
	})
	t.Cleanup(restoreDirector)
	server := NewServer(application, "0")

	createResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]string{
		"title":             "原始预设故事",
		"origin":            "直接使用状态预设。",
		"story_teller_id":   "classic",
		"story_director_id": director.ID,
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create preset-only story status=%d body=%s", createResp.Code, createResp.Body.String())
	}
	if calls != 0 {
		t.Fatalf("disabled state schema adaptation must not call Director, calls=%d", calls)
	}
}

func TestInteractiveActorTraitRollAndInitialStateAPI(t *testing.T) {
	application := newTestApplication(t)
	actorState, err := application.CreateActorState(interactive.ActorStateModule{
		ID:   "trait-api-state",
		Name: "词条 API 状态",
		ActorState: interactive.StoryDirectorActorStateSystem{
			Templates: []interactive.ActorStateTemplate{{
				ID: "protagonist", Name: "主角", TraitRules: []interactive.ActorTraitRule{{PoolID: "origin", DrawCount: 1}},
			}},
			TraitPools: []interactive.ActorTraitPool{{
				ID: "origin", Name: "出身", Traits: []interactive.ActorTraitDefinition{
					{ID: "wanderer", Name: "旅人", Weight: 1, Visibility: "visible"},
					{ID: "scholar", Name: "学者", Weight: 1, Visibility: "visible"},
				},
			}},
			InitialActors: []interactive.ActorStateInitialActor{{ID: "protagonist", Name: "主角", TemplateID: "protagonist", Role: "protagonist"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	director, err := application.CreateStoryDirector(interactive.StoryDirector{
		ID:   "trait-api-director",
		Name: "词条 API 导演",
		ModuleRefs: interactive.StoryDirectorModuleRefs{
			NarrativeStyleID: "classic", ActorStateID: actorState.ID,
			EventPackagesDisabled: true, RuleSystemDisabled: true, MemoryStructureDisabled: true, ImagePresetDisabled: true,
		},
		Strategy: interactive.StoryDirectorStrategy{Enabled: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(application, "0")

	rollResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/actor-traits/roll", map[string]any{
		"story_director_id": director.ID,
		"actor_id":          "protagonist",
		"template_id":       "protagonist",
		"seed":              42,
		"selections": []map[string]any{{
			"pool_id": "origin", "trait_ids": []string{"scholar"},
		}},
	})
	if rollResp.Code != http.StatusOK {
		t.Fatalf("actor trait roll status = %d body=%s", rollResp.Code, rollResp.Body.String())
	}
	var rolled struct {
		StoryDirectorID string                           `json:"story_director_id"`
		Seed            int64                            `json:"seed"`
		Traits          []interactive.ActorTraitInstance `json:"traits"`
	}
	decodeResponse(t, rollResp.Body.Bytes(), &rolled)
	if rolled.StoryDirectorID != director.ID || rolled.Seed != 42 || len(rolled.Traits) != 1 || rolled.Traits[0].TraitID != "scholar" {
		t.Fatalf("actor trait roll mismatch: %#v", rolled)
	}
	if strings.Contains(rollResp.Body.String(), "state_ops") {
		t.Fatalf("trait roll API must not expose StateOps: %s", rollResp.Body.String())
	}

	createResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]any{
		"title":             "带主角词条",
		"story_teller_id":   "classic",
		"story_director_id": director.ID,
		"initial_trait_rolls": []map[string]any{{
			"actor_id": "protagonist", "seed": 42,
			"selections": []map[string]any{{"pool_id": "origin", "trait_ids": []string{"scholar"}}},
		}},
		"initial_state_ops": []map[string]any{{
			"op":    "set",
			"path":  "flags.client_injected",
			"value": 18,
		}},
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create story with initial state status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		ID     string `json:"id"`
		Events int    `json:"events"`
	}
	decodeResponse(t, createResp.Body.Bytes(), &created)
	if created.ID == "" || created.Events != 1 {
		t.Fatalf("created story with initial state mismatch: %#v", created)
	}
	snapshotResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+created.ID+"/snapshot", nil)
	if snapshotResp.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d body=%s", snapshotResp.Code, snapshotResp.Body.String())
	}
	var snapshot struct {
		State map[string]any `json:"state"`
	}
	decodeResponse(t, snapshotResp.Body.Bytes(), &snapshot)
	actors, _ := snapshot.State["actors"].(map[string]any)
	protagonist, _ := actors["protagonist"].(map[string]any)
	traits, _ := protagonist["traits"].([]any)
	if len(traits) != 1 || traits[0].(map[string]any)["trait_id"] != "scholar" {
		t.Fatalf("preview and formal creation should preserve the fixed trait: %#v", snapshot.State)
	}
	if _, injected := snapshot.State["flags"]; injected {
		t.Fatalf("clients must not inject arbitrary StateOps: %#v", snapshot.State)
	}

	autoCreateResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]any{
		"title": "后端自动抽取", "story_teller_id": "classic", "story_director_id": director.ID,
	})
	if autoCreateResp.Code != http.StatusOK {
		t.Fatalf("automatic trait creation status=%d body=%s", autoCreateResp.Code, autoCreateResp.Body.String())
	}
	var autoCreated struct {
		ID string `json:"id"`
	}
	decodeResponse(t, autoCreateResp.Body.Bytes(), &autoCreated)
	autoSnapshotResp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+autoCreated.ID+"/snapshot", nil)
	var autoSnapshot struct {
		State map[string]any `json:"state"`
	}
	decodeResponse(t, autoSnapshotResp.Body.Bytes(), &autoSnapshot)
	autoActors, _ := autoSnapshot.State["actors"].(map[string]any)
	autoProtagonist, _ := autoActors["protagonist"].(map[string]any)
	if autoTraits, _ := autoProtagonist["traits"].([]any); len(autoTraits) != 1 {
		t.Fatalf("backend should draw traits when the client makes no selection: %#v", autoSnapshot.State)
	}

	invalidResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/actor-traits/roll", map[string]any{
		"story_director_id": director.ID,
		"actor_id":          "protagonist",
		"template_id":       "missing",
	})
	if invalidResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid template should be rejected status=%d body=%s", invalidResp.Code, invalidResp.Body.String())
	}
	invalidPoolResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/actor-traits/roll", map[string]any{
		"story_director_id": director.ID,
		"actor_id":          "protagonist",
		"template_id":       "protagonist",
		"selections":        []map[string]any{{"pool_id": "forbidden", "trait_ids": []string{"scholar"}}},
	})
	if invalidPoolResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid trait pool should be rejected status=%d body=%s", invalidPoolResp.Code, invalidPoolResp.Body.String())
	}
	invalidTraitResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/actor-traits/roll", map[string]any{
		"story_director_id": director.ID,
		"actor_id":          "protagonist",
		"template_id":       "protagonist",
		"selections":        []map[string]any{{"pool_id": "origin", "trait_ids": []string{"missing"}}},
	})
	if invalidTraitResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid trait should be rejected status=%d body=%s", invalidTraitResp.Code, invalidTraitResp.Body.String())
	}
	if legacyResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/opening/roll", map[string]any{}); legacyResp.Code != http.StatusNotFound {
		t.Fatalf("legacy opening roll route should be removed status=%d body=%s", legacyResp.Code, legacyResp.Body.String())
	}
	if legacyResp := performJSONRequest(t, server, http.MethodGet, "/api/opening-selectors", nil); legacyResp.Code != http.StatusNotFound {
		t.Fatalf("standalone opening selector API should be removed status=%d body=%s", legacyResp.Code, legacyResp.Body.String())
	}
}

func TestInteractiveDisabledStoryDirectorModulesAPI(t *testing.T) {
	application := newTestApplication(t)
	if _, err := application.CreateStoryDirector(interactive.StoryDirector{
		ID:   "detached",
		Name: "关闭模块导演",
		ModuleRefs: interactive.StoryDirectorModuleRefs{
			NarrativeStyleID:        "non-classic-style",
			NarrativeStyleDisabled:  true,
			EventSystemID:           "default",
			EventSystemDisabled:     true,
			RuleSystemID:            "default",
			RuleSystemDisabled:      true,
			OpeningSelectorID:       "default",
			OpeningSelectorDisabled: true,
			ImagePresetID:           "non-default-image",
			ImagePresetDisabled:     true,
		},
		Strategy: interactive.StoryDirectorStrategy{Enabled: true},
	}); err != nil {
		t.Fatalf("create detached story director failed: %v", err)
	}
	server := NewServer(application, "0")

	createResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]any{
		"title":             "关闭模块故事",
		"story_director_id": "detached",
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create detached story status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		ID              string `json:"id"`
		StoryTellerID   string `json:"story_teller_id"`
		StoryDirectorID string `json:"story_director_id"`
		ImageSettings   struct {
			PresetID string `json:"preset_id"`
		} `json:"image_settings"`
	}
	decodeResponse(t, createResp.Body.Bytes(), &created)
	if created.ID == "" || created.StoryDirectorID != "detached" {
		t.Fatalf("created detached story mismatch: %#v", created)
	}
	if created.StoryTellerID != "classic" {
		t.Fatalf("disabled narrative style should not be inherited, got %#v", created)
	}
	if created.ImageSettings.PresetID != "game-cg" {
		t.Fatalf("disabled image preset should not be inherited, got %#v", created.ImageSettings)
	}

	rebuildResp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/"+created.ID+"/director/rebuild", nil)
	if rebuildResp.Code != http.StatusOK {
		t.Fatalf("rebuild detached director status = %d body=%s", rebuildResp.Code, rebuildResp.Body.String())
	}
	var rebuilt struct {
		Docs struct {
			Plan string `json:"plan"`
		} `json:"docs"`
	}
	decodeResponse(t, rebuildResp.Body.Bytes(), &rebuilt)
	if !strings.Contains(rebuilt.Docs.Plan, "正文Agent可读") {
		t.Fatalf("rebuilt detached director should return plan docs: %#v", rebuilt)
	}
}

func TestPresetUpdateRejectsStaleWorkspaceIdentity(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	resp := performJSONRequest(t, server, http.MethodPatch, "/api/actor-states/default", map[string]any{
		"workspace": filepath.Join(t.TempDir(), "different-workspace"),
	})
	if resp.Code != http.StatusConflict {
		t.Fatalf("stale workspace update status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestStoryMemoryStructuresPresetAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	listResp := performJSONRequest(t, server, http.MethodGet, "/api/story-memory-structures", nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list story memory structures status = %d body=%s", listResp.Code, listResp.Body.String())
	}
	var list struct {
		Items []interactive.StoryMemoryStructureModule `json:"story_memory_structures"`
	}
	decodeResponse(t, listResp.Body.Bytes(), &list)
	if len(list.Items) == 0 || list.Items[0].ID == "" {
		t.Fatalf("list should include built-in memory structure modules: %#v", list)
	}

	enabled := true
	createResp := performJSONRequest(t, server, http.MethodPost, "/api/story-memory-structures", interactive.StoryMemoryStructureModule{
		ID:   "custom-memory-api",
		Name: "API 记忆结构",
		Structures: []interactive.StoryMemoryStructure{{
			ID:      "quest",
			Name:    "任务",
			Mode:    "keyed",
			Enabled: &enabled,
			Fields: []interactive.StoryMemoryField{
				{ID: "name", Name: "名称", Required: true, Order: 10},
				{ID: "status", Name: "状态", Order: 20},
			},
			KeyFieldID: "name",
			Order:      10,
		}},
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create story memory structure status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created interactive.StoryMemoryStructureModule
	decodeResponse(t, createResp.Body.Bytes(), &created)
	if created.ID != "custom-memory-api" || !created.Custom || len(created.Structures) != 1 {
		t.Fatalf("created story memory structure mismatch: %#v", created)
	}

	getResp := performJSONRequest(t, server, http.MethodGet, "/api/story-memory-structures/custom-memory-api", nil)
	if getResp.Code != http.StatusOK {
		t.Fatalf("get story memory structure status = %d body=%s", getResp.Code, getResp.Body.String())
	}
	var loaded interactive.StoryMemoryStructureModule
	decodeResponse(t, getResp.Body.Bytes(), &loaded)
	if loaded.ID != created.ID || loaded.UpdatedAt != created.UpdatedAt {
		t.Fatalf("loaded story memory structure mismatch: %#v", loaded)
	}

	conflictResp := performJSONRequest(t, server, http.MethodPatch, "/api/story-memory-structures/custom-memory-api", map[string]any{
		"id":            "custom-memory-api",
		"name":          "冲突名称",
		"structures":    created.Structures,
		"base_revision": "stale-revision",
	})
	if conflictResp.Code != http.StatusConflict {
		t.Fatalf("stale story memory structure update should conflict, status = %d body=%s", conflictResp.Code, conflictResp.Body.String())
	}

	updateResp := performJSONRequest(t, server, http.MethodPatch, "/api/story-memory-structures/custom-memory-api", map[string]any{
		"id":            "custom-memory-api",
		"name":          "更新后的 API 记忆结构",
		"structures":    created.Structures,
		"base_revision": created.UpdatedAt,
	})
	if updateResp.Code != http.StatusOK {
		t.Fatalf("update story memory structure status = %d body=%s", updateResp.Code, updateResp.Body.String())
	}
	var updated interactive.StoryMemoryStructureModule
	decodeResponse(t, updateResp.Body.Bytes(), &updated)
	if updated.Name != "更新后的 API 记忆结构" || updated.UpdatedAt == created.UpdatedAt {
		t.Fatalf("updated story memory structure mismatch: %#v", updated)
	}

	deleteResp := performJSONRequest(t, server, http.MethodDelete, "/api/story-memory-structures/custom-memory-api", nil)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("delete story memory structure status = %d body=%s", deleteResp.Code, deleteResp.Body.String())
	}
	missingResp := performJSONRequest(t, server, http.MethodGet, "/api/story-memory-structures/custom-memory-api", nil)
	if missingResp.Code != http.StatusNotFound {
		t.Fatalf("deleted story memory structure should be missing, status = %d body=%s", missingResp.Code, missingResp.Body.String())
	}
}

func TestInteractiveChatRequiresStoryID(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	resp := performJSONRequest(t, server, http.MethodPost, "/api/interactive/chat", map[string]string{
		"mode":    "story",
		"message": "我推开酒馆的门",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("chat status = %d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "故事 ID 不能为空") {
		t.Fatalf("unexpected response body: %s", resp.Body.String())
	}
}

func waitForDirectorStatusAPI(t *testing.T, server *Server, storyID, status string) interactive.DirectorPlanStatus {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	var current interactive.DirectorPlanStatus
	for {
		resp := performJSONRequest(t, server, http.MethodGet, "/api/interactive/stories/"+storyID+"/director/status?branch=main", nil)
		if resp.Code != http.StatusOK {
			t.Fatalf("director status polling = %d body=%s", resp.Code, resp.Body.String())
		}
		decodeResponse(t, resp.Body.Bytes(), &current)
		if current.Status == status {
			return current
		}
		select {
		case <-t.Context().Done():
			t.Fatalf("director status did not reach %q before test cancellation: %#v", status, current)
		case <-ticker.C:
		}
	}
}
