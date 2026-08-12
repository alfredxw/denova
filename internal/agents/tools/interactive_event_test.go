package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	agenttools "github.com/alfredxw/denova/agent/tools"

	"denova/config"
	"denova/internal/interactive"
)

func TestEventCardReadAdapterUsesCanonicalURIAndAdapterSpecificSchema(t *testing.T) {
	scope := testEventCardReadScope(9)
	adapter, err := agenttools.NewReadAdapter(
		agent.CapabilityIdentity{Kind: "test.read.event_card", Version: 1},
		"event_card", matchEventCardURI, scope.readCard,
	)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := agenttools.Read([]agenttools.ReadAdapter{adapter})
	if err != nil {
		t.Fatal(err)
	}

	result, err := definition.Tool.Run(context.Background(), `{"path":"event://package/card-1"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.ModelContent, `"path":"event://package/card-1"`) ||
		!strings.Contains(result.ModelContent, `"schema": "interactive.event_card.read.v1"`) ||
		!strings.Contains(result.ModelContent, `"source_turn_id": "turn-1"`) ||
		!strings.Contains(result.ModelContent, `"event_ref": "package/card-1"`) {
		t.Fatalf("event-card result = %s", result.ModelContent)
	}

	if _, err := definition.Tool.Run(context.Background(), `{"path":"event://package/card-1","offset":1}`); err != nil {
		t.Fatalf("event adapter rejected harmless local-text parameters: %v", err)
	}
	if _, err := definition.Tool.Run(context.Background(), `{"path":"event://package/outside"}`); err == nil || !strings.Contains(err.Error(), "outside the frozen Director opportunity") {
		t.Fatalf("event adapter escaped its frozen scope: %v", err)
	}
}

func TestEventCardReadAdapterFreezesOpportunityAtSourceTurn(t *testing.T) {
	store := interactive.NewStore(t.TempDir())
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "source turn", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turns := make([]interactive.TurnEvent, 0, 4)
	for index := range 4 {
		turn, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
			BranchID: "main", User: fmt.Sprintf("action %d", index), Narrative: "result",
		})
		if err != nil {
			t.Fatal(err)
		}
		turns = append(turns, turn)
	}

	early, err := newEventCardReadAdapter(InteractiveContext{
		Store: store, StoryID: story.ID, BranchID: "main", TurnID: turns[2].ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if early != nil {
		t.Fatal("third balanced turn unexpectedly exposed an event-card opportunity")
	}
	due, err := newEventCardReadAdapter(InteractiveContext{
		Store: store, StoryID: story.ID, BranchID: "main", TurnID: turns[3].ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if due == nil {
		t.Fatal("fourth balanced turn did not expose its event-card opportunity")
	}
	definition, err := agenttools.Read([]agenttools.ReadAdapter{due})
	if err != nil {
		t.Fatal(err)
	}
	cards, err := store.DirectorEventCardReadScope(story.ID, "main", turns[3].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) == 0 {
		t.Fatal("source-turn scope did not contain an event card")
	}
	result, err := definition.Tool.Run(context.Background(), `{"path":"event://`+cards[0].ID+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.ModelContent, `"source_turn_id": "`+turns[3].ID+`"`) {
		t.Fatalf("event-card result lost source turn: %s", result.ModelContent)
	}
}

func TestInteractiveDirectorEventReadUsesIndependentCapabilityWithoutOpeningWorkspace(t *testing.T) {
	store := interactive.NewStore(t.TempDir())
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "capability", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	var sourceTurn string
	for index := range 4 {
		turn, appendErr := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
			BranchID: "main", User: fmt.Sprintf("action %d", index), Narrative: "result",
		})
		if appendErr != nil {
			t.Fatal(appendErr)
		}
		sourceTurn = turn.ID
	}
	toolContext := InteractiveContext{
		Store: store, StoryID: story.ID, BranchID: "main", TurnID: sourceTurn, MaintenanceTask: "director_plan_update",
	}
	factory := interactiveDirectorReadAdapterFactory(toolContext)
	disabled, err := factory(config.ResolvedAgentToolSettings{config.AgentToolWorkspaceRead: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(disabled) != 0 {
		t.Fatalf("workspace_read must not authorize the event adapter: %#v", disabled)
	}

	settings := config.ResolvedAgentToolSettings{config.AgentToolEventRead: true}
	enabled, err := factory(settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != 1 {
		t.Fatalf("event adapter bindings = %d, want 1", len(enabled))
	}
	missingWorkspace := t.TempDir() + "/must-not-be-opened"
	definitions, err := NewCatalog(&config.Config{Workspace: missingWorkspace}, nil, RuntimeExecutables{}).Workspace(settings, enabled...)
	if err != nil {
		t.Fatalf("event-only read unexpectedly opened workspace: %v", err)
	}
	if len(definitions) != 1 {
		t.Fatalf("event-only workspace assembly definitions = %d, want 1", len(definitions))
	}
	info, err := definitions[0].Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "read" || definitions[0].Descriptor.Capability != config.AgentToolEventRead {
		t.Fatalf("event read definition = info=%#v descriptor=%#v", info, definitions[0].Descriptor)
	}
}

func TestEventCardReadScopeAllowsEveryFrozenCardAcrossParallelReads(t *testing.T) {
	scope := testEventCardReadScope(24)
	start := make(chan struct{})
	results := make(chan error, len(scope.cards))
	var group sync.WaitGroup
	for ref := range scope.cards {
		group.Add(1)
		go func(ref string) {
			defer group.Done()
			<-start
			_, err := scope.readCard(context.Background(), eventCardReadInput{Path: "event://" + ref})
			results <- err
		}(ref)
	}
	close(start)
	group.Wait()
	close(results)

	succeeded := 0
	for err := range results {
		if err != nil {
			t.Fatalf("unexpected parallel read error: %v", err)
		}
		succeeded++
	}
	if succeeded != len(scope.cards) {
		t.Fatalf("parallel reads: succeeded=%d want=%d", succeeded, len(scope.cards))
	}
}

func TestParseEventCardURIRejectsNonCanonicalResources(t *testing.T) {
	for _, value := range []string{
		"event://package",
		"event:///card",
		"event://package/group/card",
		"event://package/card?full=true",
		"event://package/card#fragment",
		"https://package/card",
	} {
		if _, _, err := parseEventCardURI(value); err == nil {
			t.Fatalf("parseEventCardURI(%q) succeeded", value)
		}
	}
	ref, canonical, err := parseEventCardURI(" EVENT://package/card ")
	if err != nil || ref != "package/card" || canonical != "event://package/card" {
		t.Fatalf("canonical URI = ref %q path %q err %v", ref, canonical, err)
	}
}

func testEventCardReadScope(count int) *eventCardReadScope {
	cards := make(map[string]interactive.DirectorEvent, count)
	for index := 1; index <= count; index++ {
		ref := fmt.Sprintf("package/card-%d", index)
		cards[ref] = interactive.DirectorEvent{ID: ref, Name: fmt.Sprintf("Card %d", index), Enabled: true}
	}
	return &eventCardReadScope{
		storyID: "story-1",
		turnID:  "turn-1",
		cards:   cards,
	}
}
