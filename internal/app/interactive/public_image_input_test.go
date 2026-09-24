package interactiveapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agents/attachment"
	"denova/internal/agents/canonicalstore"
	agentchat "denova/internal/agents/chat"
	agentcompaction "denova/internal/agents/context/compaction"
	agentconversation "denova/internal/agents/conversation"
	agentexecution "denova/internal/agents/execution"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	productsession "denova/internal/agents/session"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"denova/internal/interactive"
	"denova/internal/project"
	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/permission"
	"github.com/alfredxw/denova/agent/providers"
)

type imageProductModel struct {
	config        providers.ModelConfig
	image         string
	calls         int
	window        int
	pressure      bool
	summaries     int
	summaryImages int
}

func (m *imageProductModel) InputEstimator() agent.InputEstimator { return m.config.InputEstimator() }

func (m *imageProductModel) Generate(_ context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.Message, error) {
	size, err := m.InputEstimator().Estimate(messages, agent.GetCommonOptions(nil, options...).Tools)
	if err != nil {
		return nil, err
	}
	images := 0
	for _, message := range messages {
		for _, file := range message.Attachments {
			encoded, err := agent.AttachmentBase64(file)
			if err != nil || encoded != m.image {
				return nil, fmt.Errorf("original native image was lost: %v", err)
			}
			images++
		}
	}
	for _, message := range messages {
		if strings.Contains(message.Content, "[Runtime context compaction request]") {
			m.summaries++
			m.summaryImages += images
			return agent.AssistantMessage("The reference depicts a blue station. Preserve the current request and latest image.", nil), nil
		}
	}
	if (!m.pressure && images != min(m.calls+1, 2)) || images == 0 || size.Tokens > m.window {
		return nil, fmt.Errorf("unexpected image input: count=%d size=%+v", images, size)
	}
	m.calls++
	response := agent.AssistantMessage("The reference image remains available.", nil)
	response.ResponseMeta = &agent.ResponseMeta{Usage: &agent.TokenUsage{PromptTokens: size.Tokens}}
	return response, nil
}

func (m *imageProductModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	response, err := m.Generate(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{response}), nil
}

// Exercise actual product journals, compaction and the provider hard guard.
// The model checks the original image bytes on both sides of a cold reopen.
func TestProductsAcceptNativeImageAfterColdReopen(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		pixels      int
		compression png.CompressionLevel
		window      int
		turns       int
	}{
		{"large_encoding", 768, png.NoCompression, 400_000, 3},
		{"visual_pressure", 2048, png.DefaultCompression, 32_000, 10},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var pngData bytes.Buffer
			if err := (&png.Encoder{CompressionLevel: scenario.compression}).Encode(&pngData, image.NewNRGBA(image.Rect(0, 0, scenario.pixels, scenario.pixels))); err != nil {
				t.Fatal(err)
			}
			encoded := base64.StdEncoding.EncodeToString(pngData.Bytes())
			for _, kind := range []string{agentrun.AgentKindIDE, agentrun.AgentKindInteractiveStory} {
				t.Run(kind, func(t *testing.T) {
					ctx := t.Context()
					workspace, dataDir := t.TempDir(), t.TempDir()
					registry := project.NewRegistry(dataDir)
					record, err := registry.Add(workspace, project.TypeGeneral, "Native image")
					if err != nil {
						t.Fatal(err)
					}
					layout, err := registry.EnsureStore(record)
					if err != nil {
						t.Fatal(err)
					}
					stories := interactive.NewStore(workspace)
					defer func() { _ = stories.Close() }()
					story, err := stories.CreateStory(interactive.CreateStoryRequest{Title: "Native image", StoryTellerID: "classic"})
					if err != nil {
						t.Fatal(err)
					}
					products, err := productsession.NewStore(layout.SessionsDir())
					if err != nil {
						t.Fatal(err)
					}
					defer func() { _ = products.Close() }()
					writing, err := products.GetOrCreate("native-image")
					if err != nil {
						t.Fatal(err)
					}
					options := agentrun.Options{ProjectID: record.ID, AgentKind: kind, Workspace: workspace, StateRoot: layout.StoreRoot, SessionID: writing.ID}
					scope := attachment.SessionScope(writing.ID)
					if kind == agentrun.AgentKindInteractiveStory {
						options = publicGameOptions(workspace, story.ID, "main")
						options.ProjectID, options.StateRoot = record.ID, layout.StoreRoot
						scope = attachment.StoryScope(story.ID)
					}
					files, err := attachment.Materialize(layout.StoreRoot, scope, "image-input", []attachment.Upload{{Name: "reference.png", MediaType: "image/png", DataURL: "data:image/png;base64," + encoded}})
					if err != nil {
						t.Fatal(err)
					}
					cfg := &config.Config{Workspace: workspace, OpenAIContextWindowTokens: scenario.window}
					model := &imageProductModel{config: providers.ModelConfig{Provider: providers.ProviderAnthropic, Model: "claude-sonnet-4-7"}, image: encoded, window: scenario.window, pressure: scenario.name == "visual_pressure"}
					for turn := range scenario.turns {
						journals, err := canonicalstore.New(dataDir, registry)
						if err != nil {
							t.Fatal(err)
						}
						runtime, err := agentexecution.NewAgentRuntime(ctx, dataDir, agentexecution.WithSessionStore(journals), agentexecution.WithProfiles(publicGameNoopProfile(workspace, story.ID)), agentexecution.WithToolMutationApplier(func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil }))
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = runtime.Close(context.Background()) })
						manager, err := agentcompaction.NewAgentManagerForModel(cfg, kind, scenario.window)
						if err != nil {
							t.Fatal(err)
						}
						input := "Inspect the reference image."
						if turn == scenario.turns-1 {
							input = "Continue using the same reference."
							files = nil
						}
						definition := agent.Definition{Key: "image-product", Name: "image-product", Model: model, ModelIdentity: agent.CapabilityIdentity{Kind: "test.image-product", Version: 1}, AttachmentRoot: layout.StoreRoot, Permission: permission.FullAccess(), Compaction: manager,
							Middlewares: []agent.Middleware{agentrun.NewModelInputLoggingMiddleware(kind, model.config, scenario.window, 4<<20, prompts.SystemPromptComposition{})}}
						var conversation agentchat.Conversation = agentconversation.NewSessionConversationForAgent(writing, cfg, kind)
						if kind == agentrun.AgentKindInteractiveStory {
							game := NewConversation(stories, "", workspace, story.ID, "main", input, 800, cfg)
							conversation = game
							definition.Middlewares = append(definition.Middlewares, gameSubmissionForTest(t, game, input, input))
						}
						run, err := runtime.Start(ctx, agentexecution.StartRequest{Cycle: agentexecution.Cycle{Definition: definition, Conversation: conversation, Options: options, Request: agentchat.ChatRequest{CommandID: fmt.Sprintf("image-turn-%d", turn), Message: input, AttachedFiles: files}}})
						if err != nil {
							t.Fatal(err)
						}
						if outcome := run.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
							t.Fatalf("image turn %d failed: %+v", turn, outcome)
						}
						if err := runtime.Close(ctx); err != nil {
							t.Fatal(err)
						}
						if turn < scenario.turns-1 {
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
					if model.calls != scenario.turns {
						t.Fatalf("unexpected extra model calls or compaction: %d", model.calls)
					}
					if model.pressure && (model.summaries < 2 || model.summaryImages == 0) {
						t.Fatalf("repeated visual pressure was not compacted with native images: summaries=%d images=%d", model.summaries, model.summaryImages)
					}
				})
			}
		})
	}
}
