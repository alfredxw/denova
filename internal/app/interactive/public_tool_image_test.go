package interactiveapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"denova/config"
	"denova/internal/agents/canonicalstore"
	agentchat "denova/internal/agents/chat"
	agentcompaction "denova/internal/agents/context/compaction"
	agentconversation "denova/internal/agents/conversation"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	productsession "denova/internal/agents/session"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"denova/internal/interactive"
	"denova/internal/project"
	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/permission"
	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/toolresult"
	agenttools "github.com/alfredxw/denova/agent/tools"
)

type toolImageProductModel struct {
	config providers.ModelConfig
	image  []byte
	calls  int
}

func (model *toolImageProductModel) InputEstimator() agent.InputEstimator {
	return model.config.InputEstimator()
}

func (model *toolImageProductModel) Generate(_ context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.Message, error) {
	if model.calls == 0 {
		model.calls++
		return agent.AssistantMessage("", []agent.ToolCall{
			{ID: "image-one", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"reference.png"}`}},
			{ID: "image-two", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"reference.png"}`}},
			{ID: "image-missing", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"missing.png"}`}},
		}), nil
	}
	images := 0
	for _, message := range messages {
		for _, attachment := range message.Attachments {
			if message.Role != agent.ToolRole || !fs.ValidPath(attachment.Path) || filepath.IsAbs(attachment.Path) || !filepath.IsAbs(attachment.RuntimePath) {
				return nil, fmt.Errorf("invalid recovered tool image: %+v", attachment)
			}
			data, err := agent.ReadAttachmentImage(attachment)
			if err != nil || !bytes.Equal(data, model.image) {
				return nil, fmt.Errorf("tool image pixels changed after recovery: %v", err)
			}
			encoded, err := json.Marshal(attachment)
			if err != nil || bytes.Contains(encoded, []byte(attachment.RuntimePath)) {
				return nil, fmt.Errorf("runtime image path entered durable metadata")
			}
			images++
		}
	}
	size, err := model.InputEstimator().Estimate(messages, agent.GetCommonOptions(nil, options...).Tools)
	if err != nil || images != 2 || size.Tokens >= 20_000 || providers.NativeImageCount(messages) != 2 {
		return nil, fmt.Errorf("tool image accounting/recovery: images=%d size=%+v err=%v", images, size, err)
	}
	model.calls++
	return agent.AssistantMessage("The captured tool images remain available.", nil), nil
}

func (model *toolImageProductModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	response, err := model.Generate(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{response}), nil
}

func TestProductsRecoverToolImagesAfterSourceDeletionAndColdReopen(t *testing.T) {
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewNRGBA(image.Rect(0, 0, 64, 48))); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{agentrun.AgentKindIDE, agentrun.AgentKindInteractiveStory} {
		t.Run(kind, func(t *testing.T) {
			ctx := t.Context()
			workspace, dataDir := t.TempDir(), t.TempDir()
			source := filepath.Join(workspace, "reference.png")
			if err := os.WriteFile(source, imageData.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			registry := project.NewRegistry(dataDir)
			record, err := registry.Add(workspace, project.TypeGeneral, "Tool image")
			if err != nil {
				t.Fatal(err)
			}
			layout, err := registry.EnsureStore(record)
			if err != nil {
				t.Fatal(err)
			}
			stories := interactive.NewStore(workspace)
			defer func() { _ = stories.Close() }()
			story, err := stories.CreateStory(interactive.CreateStoryRequest{Title: "Tool image", StoryTellerID: "classic"})
			if err != nil {
				t.Fatal(err)
			}
			products, err := productsession.NewStore(layout.SessionsDir())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = products.Close() }()
			writing, err := products.GetOrCreate("tool-image")
			if err != nil {
				t.Fatal(err)
			}
			options := agentrun.Options{ProjectID: record.ID, AgentKind: kind, Workspace: workspace, StateRoot: layout.StoreRoot, SessionID: writing.ID}
			if kind == agentrun.AgentKindInteractiveStory {
				options = publicGameOptions(workspace, story.ID, "main")
				options.ProjectID, options.StateRoot = record.ID, layout.StoreRoot
			}
			cfg := &config.Config{Workspace: workspace, OpenAIContextWindowTokens: 400_000}
			model := &toolImageProductModel{config: providers.ModelConfig{Provider: providers.ProviderAnthropic, Model: "claude-sonnet-4-6"}, image: imageData.Bytes()}
			for turn := range 3 {
				journals, err := canonicalstore.New(dataDir, registry)
				if err != nil {
					t.Fatal(err)
				}
				runtime, err := agentexecution.NewAgentRuntime(ctx, dataDir, agentexecution.WithSessionStore(journals), agentexecution.WithProfiles(publicGameNoopProfile(workspace, story.ID)), agentexecution.WithToolMutationApplier(func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil }))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = runtime.Close(context.Background()) })
				manager, err := agentcompaction.NewAgentManagerForModel(cfg, kind, 400_000)
				if err != nil {
					t.Fatal(err)
				}
				input := "Inspect the local reference image."
				if turn > 0 {
					input = "Continue using the previously captured images."
				}
				definition := agent.Definition{
					Key: "tool-image-product", Name: "tool-image-product", Model: model, ModelIdentity: agent.CapabilityIdentity{Kind: "test.tool-image", Version: 1},
					Tools:      agenttools.Workspace(agenttools.WorkspaceConfig{Root: workspace}),
					Permission: permission.FullAccess(), Compaction: manager, ResultProcessor: toolresult.Standard(toolresult.Policy{MaxBytes: 1024}),
				}
				var conversation agentchat.Conversation = agentconversation.NewSessionConversationForAgent(writing, cfg, kind)
				if kind == agentrun.AgentKindInteractiveStory {
					game := NewConversation(stories, "", workspace, story.ID, "main", input, 800, cfg)
					conversation = game
					definition.Middlewares = append(definition.Middlewares, gameSubmissionForTest(t, game, input, input))
				}
				run, err := runtime.Start(ctx, agentexecution.StartRequest{Cycle: agentexecution.Cycle{Definition: definition, Conversation: conversation, Options: options, Request: agentchat.ChatRequest{CommandID: fmt.Sprintf("tool-image-turn-%d", turn), Message: input}}})
				if err != nil {
					t.Fatal(err)
				}
				if outcome := run.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
					t.Fatalf("tool image turn %d failed: %+v", turn, outcome)
				}
				if err := runtime.Close(ctx); err != nil {
					t.Fatal(err)
				}
				if turn == 0 {
					if err := os.Remove(source); err != nil {
						t.Fatal(err)
					}
				}
				if turn < 2 {
					if err := stories.Close(); err != nil {
						t.Fatal(err)
					}
					stories = interactive.NewStore(workspace)
					if err := products.Close(); err != nil {
						t.Fatal(err)
					}
					products, err = productsession.NewStore(layout.SessionsDir())
					if err != nil {
						t.Fatal(err)
					}
					writing, err = products.GetOrCreate(writing.ID)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			if model.calls != 4 {
				t.Fatalf("unexpected repeated calls: %d", model.calls)
			}
		})
	}
}
