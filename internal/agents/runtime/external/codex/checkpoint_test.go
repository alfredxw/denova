package codex

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"denova/config"
	"denova/internal/agents/attachment"
	"denova/internal/agents/runtime/external"
)

func TestImageSummaryUsesNativeProtocolAndReadOnlyMaintenance(t *testing.T) {
	fixture := newProtocolFixture(t)
	const url = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aKX0AAAAASUVORK5CYII="
	files, err := attachment.Materialize(t.TempDir(), attachment.SessionScope("summary"), "images", []attachment.Upload{{Name: "map.png", DataURL: url}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		var result error
		defer func() {
			if value := recover(); value != nil {
				result = fmt.Errorf("summary panic: %v", value)
			}
			done <- result
		}()
		_, result = (external.HistoryPreparation{Adapter: fixture.client, Input: external.Input{
			Selection: config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "gpt-5.5", Sandbox: config.CodexFullAccess}},
			History:   []external.Message{{Role: "user", Text: strings.Repeat("Map reference. ", 10000), Attachments: files, ToolImages: files, Cursor: 1}},
		}}).Prepare(t.Context())
	}()
	images, summaries := 0, 0
	for {
		select {
		case err := <-done:
			if err != nil || images != 2 || summaries < 2 {
				t.Fatalf("summary protocol: images=%d calls=%d err=%v", images, summaries, err)
			}
			return
		case request := <-fixture.received:
			response := `{}`
			switch request.Method {
			case "thread/start":
				summaries++
				var start struct {
					Sandbox      string
					DynamicTools []any
					Config       map[string]bool
					Ephemeral    bool
				}
				if err := json.Unmarshal(request.Params, &start); err != nil || start.Sandbox != "read-only" || len(start.DynamicTools) != 0 || start.Config["tools.update_plan.enabled"] || !start.Ephemeral {
					t.Fatalf("summary inherited turn permissions: %s", request.Params)
				}
				response = `{"thread":{"id":"summary-thread"}}`
			case "thread/inject_items":
				var injected struct {
					Items []struct {
						Content []struct {
							Type     string
							ImageURL string `json:"image_url"`
						}
					}
				}
				if err := json.Unmarshal(request.Params, &injected); err != nil {
					t.Fatal(err)
				}
				for _, item := range injected.Items {
					for _, content := range item.Content {
						if content.Type == "input_image" {
							if content.ImageURL != url {
								t.Fatal("native image pixels changed")
							}
							images++
						}
					}
				}
			case "turn/start":
				response = `{"turn":{"id":"summary-turn","status":"inProgress"}}`
			case "thread/unsubscribe":
				response = `{"status":"unsubscribed"}`
			default:
				t.Fatalf("unexpected summary operation: %s", request.Method)
			}
			if err := json.NewEncoder(fixture.server).Encode(packet{ID: request.ID, Result: json.RawMessage(response)}); err != nil {
				t.Fatal(err)
			}
			if request.Method == "turn/start" {
				fixture.send(t, "", "item/completed", map[string]any{"threadId": "summary-thread", "turnId": "summary-turn", "item": map[string]any{"type": "agentMessage", "id": "summary", "text": "Map details preserved."}})
				fixture.send(t, "", "turn/completed", map[string]any{"threadId": "summary-thread", "turn": map[string]string{"id": "summary-turn", "status": "completed"}})
			}
		case <-time.After(3 * time.Second):
			t.Fatal("summary protocol did not complete")
		}
	}
}

func TestInputEstimatorUsesResolvedAPIModel(t *testing.T) {
	client := Client{apiModel: "gpt-5.5"}
	input := external.Input{Selection: config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{ProfileID: "custom-route"}}}
	estimator := client.InputEstimator(input)
	if estimator.ImageTokens == nil || estimator.ImageTokens(256, 256) >= 32*1024 {
		t.Fatal("API route lost its resolved model policy")
	}
	input.Selection.Codex = &config.CodexRuntimeSettings{Model: "unknown-alias"}
	if client.InputEstimator(input).ImageTokens != nil {
		t.Fatal("unknown CLI model did not use the conservative fallback")
	}
}
