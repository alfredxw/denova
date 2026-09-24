package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
)

func TestPublicSummaryAndTextStreamBeforeTurnCompletion(t *testing.T) {
	fixture := newProtocolFixture(t)
	host := &testHost{events: make(chan agentrun.Event, 8)}
	done := startFixtureTurn(t, t.Context(), fixture, host)
	fixture.send(t, "", "item/reasoning/textDelta", map[string]any{"threadId": "thread", "turnId": "turn", "itemId": "reason", "delta": "Private reasoning must stay private."})
	for _, event := range []struct{ method, item, delta, kind string }{
		{"item/reasoning/summaryTextDelta", "reason", "Checking the request. ", "thinking"},
		{"item/reasoning/summaryTextDelta", "reason", "Preparing the response.", "thinking"},
		{"item/agentMessage/delta", "message", "First ", "chunk"},
		{"item/agentMessage/delta", "message", "second.", "chunk"},
	} {
		fixture.send(t, "", event.method, map[string]any{"threadId": "thread", "turnId": "turn", "itemId": event.item, "summaryIndex": 0, "delta": event.delta})
		select {
		case got := <-host.events:
			if got.Type != event.kind || got.DataString("content") != event.delta {
				t.Fatalf("unexpected streamed event: %#v", got)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s was not streamed before completion", event.method)
		}
	}
	fixture.send(t, "", "item/completed", map[string]any{"threadId": "thread", "turnId": "turn", "item": map[string]any{"type": "agentMessage", "id": "message", "text": "First second."}})
	fixture.send(t, "", "turn/completed", map[string]any{"threadId": "thread", "turn": map[string]string{"id": "turn", "status": "completed"}})
	result := <-done
	if result.err != nil || result.result.Text != "First second." || host.output.String() != result.result.Text {
		t.Fatalf("completion duplicated text or included reasoning: %#v", result)
	}
}

func TestReportedUsageReplacesCumulativeTotalsAndSurvivesFailure(t *testing.T) {
	for _, status := range []string{"completed", "failed"} {
		t.Run(status, func(t *testing.T) {
			fixture := newProtocolFixture(t)
			done := startFixtureTurn(t, t.Context(), fixture, &testHost{})
			for _, tokens := range []int{100, 200} {
				fixture.send(t, "", "thread/tokenUsage/updated", map[string]any{"threadId": "thread", "turnId": "turn", "tokenUsage": map[string]any{"total": map[string]int{"inputTokens": tokens, "cachedInputTokens": 30, "outputTokens": 50, "reasoningOutputTokens": 10, "totalTokens": tokens + 50}}})
			}
			fixture.send(t, "", "turn/completed", map[string]any{"threadId": "thread", "turn": map[string]string{"id": "turn", "status": status}})
			result := <-done
			if (result.err == nil) != (status == "completed") {
				t.Fatalf("unexpected outcome: %v", result.err)
			}
			u := result.result.Usage
			if u == nil || u.PromptTokens != 200 || u.TotalTokens != 250 || u.PromptTokenDetails.CachedTokens != 30 || u.CompletionTokensDetails.ReasoningTokens != 10 {
				t.Fatalf("reported total changed: %+v", u)
			}
		})
	}
}

func TestCancelBeforeTurnStartReplyInterruptsAcceptedTurn(t *testing.T) {
	fixture := newProtocolFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		defer func() {
			if value := recover(); value != nil {
				done <- fmt.Errorf("fixture run panic: %v", value)
			}
		}()
		_, err := fixture.client.Run(ctx, external.Input{
			Selection: config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "test-model"}},
			Tools:     []external.Tool{{Name: "ask", Schema: json.RawMessage(`{"type":"object"}`)}},
			Text:      "Test cancellation.",
		}, &testHost{})
		done <- err
	}()
	reply := func(method, result string) packet {
		t.Helper()
		request := fixture.receive(t)
		if request.Method != method {
			t.Fatalf("expected %s, got %s", method, request.Method)
		}
		if err := json.NewEncoder(fixture.server).Encode(packet{ID: request.ID, Result: json.RawMessage(result)}); err != nil {
			t.Fatal(err)
		}
		return request
	}
	reply("thread/start", `{"thread":{"id":"thread"}}`)
	start := fixture.receive(t)
	if start.Method != "turn/start" {
		t.Fatalf("expected turn/start, got %s", start.Method)
	}
	cancel()
	if err := json.NewEncoder(fixture.server).Encode(packet{ID: start.ID, Result: json.RawMessage(`{"turn":{"id":"accepted-turn","status":"inProgress"}}`)}); err != nil {
		t.Fatal(err)
	}
	interrupt := reply("turn/interrupt", `{}`)
	var target struct{ ThreadID, TurnID string }
	if err := json.Unmarshal(interrupt.Params, &target); err != nil || target.ThreadID != "thread" || target.TurnID != "accepted-turn" {
		t.Fatalf("interrupted wrong target: %s, %v", interrupt.Params, err)
	}
	reply("thread/unsubscribe", `{"status":"unsubscribed"}`)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation lost: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled run did not settle")
	}
	select {
	case <-fixture.client.done:
		t.Fatal("cancelling one turn closed the shared connection")
	default:
	}
}

func TestMessageOutputMergesRepeatedCompletionByItemID(t *testing.T) {
	output := messageOutput{items: map[string]string{}}
	host := &testHost{}
	for _, id := range []string{"first", "second"} {
		if err := output.append(host, id, "Same "); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := output.complete(host, id, "Same text."); err != nil {
				t.Fatal(err)
			}
		}
	}
	if got := output.text(); got != "Same text.\n\nSame text." || host.output.String() != got {
		t.Fatalf("canonical=%q displayed=%q", got, host.output.String())
	}
}
