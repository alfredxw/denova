package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
)

func validChoiceInteraction() InteractionRequest {
	return InteractionRequest{
		ID: "ask-choice", Kind: InteractionAsk, AllowOther: true,
		Questions: []InteractionQuestion{{
			ID: "scope", Prompt: "选择范围",
			Options: []InteractionOption{
				{Value: "small", Label: "小", Recommended: true},
				{Value: "large", Label: "大"},
			},
		}},
	}
}

func TestStandardAskInteractionPreservesBoundedChoiceAndOtherContract(t *testing.T) {
	policy := StandardInteraction()
	request := validChoiceInteraction()
	if err := policy.ValidateRequest(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	resolved, err := policy.Resolve(context.Background(), request, InteractionResponse{Answers: []InteractionAnswer{{
		QuestionID: "scope", Text: "Use a staged rollout",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Answers) != 1 || resolved.Answers[0].Text != "Use a staged rollout" {
		t.Fatalf("Other resolution=%#v", resolved)
	}

	tests := []struct {
		name   string
		mutate func(*InteractionRequest)
		want   string
	}{
		{name: "one option", mutate: func(value *InteractionRequest) { value.Questions[0].Options = value.Questions[0].Options[:1] }, want: "two to three"},
		{name: "reserved other", mutate: func(value *InteractionRequest) { value.Questions[0].Options[0].Value = "other" }, want: "reserved"},
		{name: "no recommendation", mutate: func(value *InteractionRequest) { value.Questions[0].Options[0].Recommended = false }, want: "exactly one"},
		{name: "two recommendations", mutate: func(value *InteractionRequest) { value.Questions[0].Options[1].Recommended = true }, want: "exactly one"},
		{name: "oversized id", mutate: func(value *InteractionRequest) {
			value.Questions[0].ID = strings.Repeat("x", maxInteractionStableIDBytes+1)
		}, want: "1..256"},
		{name: "oversized prompt", mutate: func(value *InteractionRequest) {
			value.Questions[0].Prompt = strings.Repeat("x", maxInteractionQuestionTextBytes+1)
		}, want: "exceeds"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validChoiceInteraction()
			test.mutate(&value)
			err := policy.ValidateRequest(context.Background(), value)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestStandardAskInteractionBoundsCustomResponseAndCancellation(t *testing.T) {
	policy := StandardInteraction()
	request := validChoiceInteraction()
	_, err := policy.Resolve(context.Background(), request, InteractionResponse{Answers: []InteractionAnswer{{
		QuestionID: "scope", Text: strings.Repeat("x", maxInteractionAnswerTextBytes+1),
	}}})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized response error=%v", err)
	}
	resolved, err := policy.Resolve(context.Background(), request, InteractionResponse{Cancelled: true})
	if err != nil || !resolved.Cancelled || len(resolved.Answers) != 0 {
		t.Fatalf("cancelled resolution=%#v error=%v", resolved, err)
	}
}

func TestDefinitionEngineResolvesPersistedAskBeforeMaterializedDefinitionCheck(t *testing.T) {
	ctx := context.Background()
	definition := Definition{
		Key: "persisted-ask-definition", Name: "persisted-ask",
		Model: &scriptedModel{}, ModelIdentity: CapabilityIdentity{Kind: "test.model", Version: 1},
	}
	prepared, err := prepareDefinitionBase(ctx, definition, PrepareRequest{
		Session: SessionView{Key: NamedSession("persisted-ask")},
		Run:     RunView{ID: "operation-1", CommandID: "command-1", Cycle: 1},
		Input:   Text("continue"), Reason: TurnReasonInteraction,
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := json.Marshal(engineTranscript{
		Version: engineTranscriptVersion, DefinitionKey: prepared.definitionKey, BehaviorKey: prepared.behaviorKey,
		PreparationStage: enginePreparationMaterialized, MaterializedFingerprint: "previous-materialized-definition",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Presentation fields deliberately use the former localized shape. Answer
	// resolution only depends on the stable IDs and selection constraints that
	// were persisted when the interaction was emitted.
	request := json.RawMessage(`{
		"id":"ask-1","kind":"ask","allow_other":true,
		"questions":[{"id":"scope","prompt":{"zh":"选择范围","en":"Choose scope"},"options":[
			{"value":"small","label":{"zh":"小","en":"Small"},"recommended":true},
			{"value":"large","label":{"zh":"大","en":"Large"}}
		]}]
	}`)
	response := json.RawMessage(`{"answers":[{"question_id":"scope","values":["small"]}]}`)
	engine := &definitionEngine{source: definition, key: NamedSession("persisted-ask")}
	encoded, err := engine.ResolveInteraction(ctx, runstate.InteractionResolveRequest{
		Snapshot: runstate.TurnSnapshot{
			CommandID: "command-1", OperationID: "operation-1", Cycle: 1,
			Input: runstate.UserInput{Text: "continue"}, State: state,
		},
		Interaction: runstate.InteractionSnapshot{
			ID: "ask-1", OperationID: "operation-1", Cycle: 1, Request: request,
		},
		Response: response,
	})
	if err != nil {
		t.Fatal(err)
	}
	var resolution InteractionResolution
	if err := json.Unmarshal(encoded, &resolution); err != nil {
		t.Fatal(err)
	}
	if len(resolution.Answers) != 1 || resolution.Answers[0].QuestionID != "scope" ||
		len(resolution.Answers[0].Values) != 1 || resolution.Answers[0].Values[0] != "small" {
		t.Fatalf("resolution = %#v", resolution)
	}
}

func TestDefinitionEngineKeepsMaterializedDefinitionFenceForPermission(t *testing.T) {
	ctx := context.Background()
	definition := Definition{
		Key: "persisted-permission-definition", Name: "persisted-permission",
		Model: &scriptedModel{}, ModelIdentity: CapabilityIdentity{Kind: "test.model", Version: 1},
	}
	prepared, err := prepareDefinitionBase(ctx, definition, PrepareRequest{
		Session: SessionView{Key: NamedSession("persisted-permission")},
		Run:     RunView{ID: "operation-1", CommandID: "command-1", Cycle: 1},
		Input:   Text("continue"), Reason: TurnReasonInteraction,
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := json.Marshal(engineTranscript{
		Version: engineTranscriptVersion, DefinitionKey: prepared.definitionKey, BehaviorKey: prepared.behaviorKey,
		PreparationStage: enginePreparationMaterialized, MaterializedFingerprint: "previous-materialized-definition",
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := &definitionEngine{source: definition, key: NamedSession("persisted-permission")}
	_, err = engine.ResolveInteraction(ctx, runstate.InteractionResolveRequest{
		Snapshot: runstate.TurnSnapshot{
			CommandID: "command-1", OperationID: "operation-1", Cycle: 1,
			Input: runstate.UserInput{Text: "continue"}, State: state,
		},
		Interaction: runstate.InteractionSnapshot{
			ID: "permission-1", OperationID: "operation-1", Cycle: 1,
			Request: json.RawMessage(`{"id":"permission-1","kind":"permission"}`),
		},
		Response: json.RawMessage(`{"permission":"deny"}`),
	})
	if !errors.Is(err, ErrDefinitionMismatch) || !strings.Contains(err.Error(), "materialized Definition changed") {
		t.Fatalf("error = %v, want materialized Definition mismatch", err)
	}
}
