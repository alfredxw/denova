package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	agentchatapp "denova/internal/app/agentchat"
	appsettings "denova/internal/app/settings"
)

func TestAutomationAdmissionKeepsNativeWhenForegroundDefaultIsCodex(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	if _, err := application.SettingsService().Patch(appsettings.Global(), config.SettingsLayerUser, []byte(`{
		"openai_api_key":"local-fixture-only", "openai_model":"test-model", "openai_base_url":"http://127.0.0.1:1/v1",
		"agent_runtimes":{"ide":{"selected":"codex","codex":{"model":"unavailable-external-model"}}}
	}`), ""); err != nil {
		t.Fatal(err)
	}
	binding := agentchatapp.Binding{ProjectID: application.ProjectID(), SessionID: "automation-runtime-check"}
	turn, err := application.AgentChat().AcceptTurn(t.Context(), agentchatapp.TurnRequest{
		Binding: binding, ChatRequest: agentchatapp.ChatRequest{CommandID: "automation-runtime-check", Message: "Draft a scene."},
		Policy: agentchatapp.TurnPolicy{Origin: agentchatapp.TurnOriginAutomation},
	})
	if err != nil {
		t.Fatal(err)
	}
	turn.Task().Abort()
	turn.Wait(turn.Task().Context())
	turn.Task().Finish()
	snapshot, err := application.AgentChat().ConversationConfig(t.Context(), binding)
	if err != nil || snapshot.Engine().Kind != config.RuntimeNative {
		t.Fatalf("automation inherited external default: %#v, %v", snapshot, err)
	}
	preview, err := application.AgentChat().ConversationConfig(t.Context(), agentchatapp.Binding{ProjectID: binding.ProjectID, SessionID: "new-foreground"})
	if err != nil || preview.Engine().Kind != config.RuntimeCodex {
		t.Fatalf("foreground default was changed: %#v, %v", preview, err)
	}
}

func TestConversationCodexModelEditsStayLocalAndInitializeDrafts(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	if _, err := application.SettingsService().Patch(appsettings.Global(), config.SettingsLayerUser, []byte(`{"agent_runtimes":{"ide":{"selected":"codex","codex":{"model":"default-model","effort":"high"}}}}`), ""); err != nil {
		t.Fatal(err)
	}
	binding := ConversationConfigBinding{Mode: ConversationModeAgentChat, ProjectID: application.ProjectID(), SessionID: "codex-model-draft"}
	preview, err := application.ConversationConfig(t.Context(), binding)
	if err != nil || preview.Revision != 0 {
		t.Fatalf("preview = %#v, %v", preview, err)
	}
	var patch ConversationConfigPatch
	if err := json.Unmarshal([]byte(`{"codex":{"model":"chosen-model"}}`), &patch); err != nil {
		t.Fatal(err)
	}
	created, err := application.PatchConversationConfig(t.Context(), binding, patch, 0)
	if err != nil || created.Revision != 1 || !reflect.DeepEqual(created.Engine().Codex, patch.Codex) {
		t.Fatalf("draft model edit = %#v, %v", created, err)
	}
	for _, mode := range []string{ConversationModeWriting, ConversationModeAgentChat} {
		binding.Mode = mode
		current, err := application.ConversationConfig(t.Context(), binding)
		if err != nil {
			t.Fatal(err)
		}
		patch.Codex = &config.CodexRuntimeSettings{Model: "chosen-model", Effort: "low"}
		next, err := application.PatchConversationConfig(t.Context(), binding, patch, current.Revision)
		if err != nil || next.Revision != current.Revision+1 || !reflect.DeepEqual(next.Engine().Codex, patch.Codex) {
			t.Fatalf("%s model edit = %#v, %v", mode, next, err)
		}
		if _, err := application.PatchConversationConfig(t.Context(), binding, patch, current.Revision); !errors.Is(err, conversationconfig.ErrRevisionConflict) {
			t.Fatalf("stale model patch accepted: %v", err)
		}
		read, err := application.ConversationConfig(t.Context(), binding)
		if err != nil || !reflect.DeepEqual(read, next) {
			t.Fatalf("model edit not persisted: %#v, %v", read, err)
		}
	}
	binding.Mode, binding.SessionID = ConversationModeAgentChat, "different-codex-draft"
	other, err := application.ConversationConfig(t.Context(), binding)
	if err != nil || !reflect.DeepEqual(other.Engine().Codex, preview.Engine().Codex) {
		t.Fatalf("model edit changed another conversation's defaults: %#v, %v", other, err)
	}
}

func TestExternalConversationUsesSharedGoalAndAllowsDraftRuntimeSelection(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	binding := ConversationConfigBinding{Mode: ConversationModeWriting, ProjectID: application.ProjectID(), SessionID: application.session.ID}
	initial, err := application.ConversationConfig(t.Context(), binding)
	if err != nil {
		t.Fatal(err)
	}
	next := initial.Config
	next.Runtime = &config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "test-external"}}
	if _, err := application.session.SetRuntimeConfig(next, initial.Revision); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{ConversationModeWriting, ConversationModeAgentChat} {
		binding.Mode = mode
		_, err := application.MutateConversationGoal(context.Background(), binding, ConversationGoalMutation{Action: "set", Objective: "Complete the chapter"})
		if err != nil {
			t.Fatalf("%s failed to create a shared Goal: %v", mode, err)
		}
	}
	created, err := application.AgentChat().PatchConversationConfig(t.Context(), agentchatapp.Binding{ProjectID: binding.ProjectID, SessionID: "uncreated"}, conversationconfig.Patch{Runtime: &config.RuntimeSelection{Kind: config.RuntimeNative}}, 0)
	if err != nil || created.Revision == 0 || created.Engine().Kind != config.RuntimeNative {
		t.Fatalf("draft runtime selection failed: %+v, %v", created, err)
	}
}
