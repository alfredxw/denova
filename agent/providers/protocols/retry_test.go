package protocols_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/providers/protocols/anthropicmessages"
	"github.com/alfredxw/denova/agent/providers/protocols/openaichatcompletions"
	"github.com/alfredxw/denova/agent/providers/protocols/openairesponses"
)

func TestAdaptersMakeOneHTTPAttemptAndPreserveRetryHints(t *testing.T) {
	for _, adapter := range []providers.ProtocolAdapter{anthropicmessages.NewAdapter(), openaichatcompletions.NewAdapter(), openairesponses.NewAdapter()} {
		t.Run(string(adapter.ID()), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "4")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":{"message":"temporarily overloaded","type":"overloaded_error","code":"server_error"}}`))
			}))
			defer server.Close()
			model, err := adapter.New(context.Background(), providers.ModelConfig{
				Provider: providers.ProviderOpenAICompatible, Protocol: adapter.ID(), APIKey: "test-key",
				Model: "test-model", BaseURL: server.URL, HTTPClient: server.Client(),
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = model.Generate(context.Background(), []*agent.Message{agent.UserMessage("go")})
			var failure *providers.APIError
			if !errors.As(err, &failure) || !failure.Retryable() || failure.RetryDelay() != 4*time.Second || calls.Load() != 1 {
				t.Fatalf("error=%#v calls=%d", err, calls.Load())
			}
			decision := agent.TransientRetry(context.Background(), agent.RetryContext{Attempt: 1, Err: err, OutputState: agent.ModelOutputNone})
			if decision.Action != agent.RetryAgain || decision.Delay < 4*time.Second {
				t.Fatalf("retry=%#v", decision)
			}
		})
	}
}

func TestPermanentProviderFailuresDoNotRetry(t *testing.T) {
	for _, failure := range []*providers.APIError{
		{StatusCode: 401}, {StatusCode: 403}, {StatusCode: 400},
		{StatusCode: 429, Code: "insufficient_quota"}, {StatusCode: 429, Kind: "billing_hard_limit_reached"},
	} {
		decision := agent.TransientRetry(context.Background(), agent.RetryContext{Attempt: 1, Err: failure})
		if decision.Action != agent.RetryStop {
			t.Fatalf("failure=%#v decision=%#v", failure, decision)
		}
	}
}
