package analytics

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/engine/execution"
	agentmodel "github.com/alfredxw/denova/agent/model"
	"github.com/alfredxw/denova/agent/model/providers"
	"github.com/alfredxw/denova/agent/model/stream"
	"github.com/alfredxw/denova/agent/schema"
	agentsession "github.com/alfredxw/denova/agent/session"
)

// Exercise the actual Native lifecycle before recording a product run outcome.
// Classifying a bare provider error would miss a wrapper that discards its cause.
func TestNativeRunFailureAnalytics(t *testing.T) {
	for _, test := range []struct {
		name  string
		cause error
		code  ErrorCode
	}{
		{"authentication", &providers.APIError{StatusCode: 401}, ErrorAuthentication},
		{"rate_limit", &providers.APIError{StatusCode: 429}, ErrorRateLimit},
		{"timeout", context.DeadlineExceeded, ErrorTimeout},
		{"network", &net.OpError{Op: "read", Net: "tcp", Err: errors.New("private diagnostic")}, ErrorNetwork},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			owner, err := agent.New(ctx, agent.Definition{Name: "failure", Model: failingNativeModel{test.cause}, Execution: execution.ExecutionPolicy{ModelMaxAttempts: 1}})
			if err != nil {
				t.Fatal(err)
			}
			defer owner.Close(context.Background())
			session, err := owner.Session(ctx, agentsession.Named("failure"))
			if err != nil {
				t.Fatal(err)
			}
			run, err := session.Run(ctx, schema.Text("private user input"))
			if err != nil {
				t.Fatal(err)
			}
			result, runErr := run.Wait(ctx)
			if result.Status != schema.ResultFailed || runErr == nil {
				t.Fatalf("result=%+v error=%v", result, runErr)
			}

			s, c := openCollector(t, filepath.Join(t.TempDir(), "analytics.json"))
			if _, err := s.SetEnabled(true); err != nil {
				t.Fatal(err)
			}
			// A scheduled run avoids unrelated daily activity events in this assertion.
			s.BeginRun(Automation, Native, Scheduled).Fail(runErr)
			events := awaitEvents(t, c, 2)
			failures := eventsNamed(events, "feature_failed")
			if len(failures) != 1 || failures[0].Properties.ErrorCode != test.code {
				t.Fatalf("expected %s, events=%+v", test.code, events)
			}
			if !errors.Is(runErr, test.cause) {
				t.Fatalf("lost original cause: %v", runErr)
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			for _, body := range c.bodies {
				if strings.Contains(body, "private") {
					t.Fatalf("uploaded diagnostic or content: %s", body)
				}
			}
		})
	}
}

type failingNativeModel struct{ cause error }

func (m failingNativeModel) Generate(context.Context, []*schema.Message, ...agentmodel.ModelOption) (*schema.Message, error) {
	return nil, m.cause
}

func (m failingNativeModel) Stream(context.Context, []*schema.Message, ...agentmodel.ModelOption) (*stream.StreamReader[*schema.Message], error) {
	return nil, m.cause
}
