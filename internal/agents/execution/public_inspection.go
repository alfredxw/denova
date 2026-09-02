package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agentchat "denova/internal/agents/chat"
	agentlifecycle "denova/internal/agents/lifecycle"
	agentrun "denova/internal/agents/run"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

type publicInspectionRegistrationContextKey struct{}

type publicInspectionRegistration struct {
	session      string
	registration *publicCycleRegistration
}

// Inspect assembles the exact provider-neutral request for a prospective
// start cycle through the same Profile, Source, Definition, Context, Toolset,
// Middleware, cleanup, and compaction path used by Start. It does not admit a
// command, invoke a provider/tool, publish product events, or retain a cycle
// registration. The public Session must already contain the canonical history
// to inspect; transcript bootstrap remains an explicit lifecycle operation.
func (s *Runtime) Inspect(ctx context.Context, cycle Cycle) (agent.Inspection, error) {
	if s == nil || s.public == nil || s.public.agent == nil {
		return agent.Inspection{}, ErrUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	workspace := cycle.Options.Workspace
	if cycle.BookService != nil {
		workspace = cycle.BookService.Workspace()
	}
	cycle.Options = cycle.Options.Normalize(workspace)
	cycle.Request = agentchat.CaptureChatRequestCallerInput(cycle.Request)
	commandID := strings.TrimSpace(cycle.Request.CommandID)
	if commandID != "" {
		if err := agentrun.ValidateCommandID(commandID); err != nil {
			return agent.Inspection{}, err
		}
	}
	key, err := agentrun.AgentSessionKeyForOptions(cycle.Options)
	if err != nil {
		return agent.Inspection{}, err
	}
	session, err := s.public.agent.Session(ctx, key)
	if err != nil {
		return agent.Inspection{}, err
	}
	if err := loadCanonicalMessages(ctx, session, cycle.Conversation); err != nil {
		return agent.Inspection{}, err
	}
	registration := &publicCycleRegistration{
		cycle: &cycle, request: cycle.Request, options: cycle.Options,
		commandKind: CommandStartTurn,
	}
	canonical, err := agentsession.CanonicalKey(key)
	if err != nil {
		return agent.Inspection{}, err
	}
	ctx = context.WithValue(ctx, publicInspectionRegistrationContextKey{}, publicInspectionRegistration{
		session: canonical, registration: registration,
	})
	input, err := agentlifecycle.TurnInput(agentlifecycle.TurnStart, cycle.Request, cycle.Options)
	if err != nil {
		return agent.Inspection{}, err
	}
	return session.Inspect(ctx, input)
}

func inspectionRegistrationFromContext(
	ctx context.Context,
	key agent.SessionKey,
	data agentlifecycle.TurnHostData,
) (*publicCycleRegistration, error) {
	value, ok := ctx.Value(publicInspectionRegistrationContextKey{}).(publicInspectionRegistration)
	if !ok || value.registration == nil || value.registration.cycle == nil {
		return nil, errors.New("Denova public Agent inspection has no ephemeral cycle registration")
	}
	canonical, err := agentsession.CanonicalKey(key)
	if err != nil {
		return nil, err
	}
	if canonical != value.session {
		return nil, errors.New("Denova public Agent inspection Session changed during preparation")
	}
	cycle := *value.registration.cycle
	want := agentchat.CallerView(cycle.Request)
	got := data.ChatRequest()
	if strings.TrimSpace(want.CommandID) != strings.TrimSpace(data.Caller.CommandID) ||
		RequestSemanticFingerprint(cycle.Request) != RequestSemanticFingerprint(got) ||
		cycle.Request.InputVisibility != data.InputVisibility {
		return nil, fmt.Errorf("Denova public Agent inspection input does not match its ephemeral cycle")
	}
	return value.registration, nil
}
