package interactiveapp

import (
	"context"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

// Output-boundary tests use a narrator-only model. Accept its fixture modules
// after real input materialization, with the runtime's selected cycle identity.
type gameSubmissionFixture struct {
	*agent.BaseMiddleware
	t            *testing.T
	conversation *Conversation
	intent, goal string
}

func (fixture gameSubmissionFixture) BeforeAgent(ctx context.Context, run *agent.RunContext) (context.Context, *agent.RunContext, error) {
	submitTestTurnResult(fixture.t, fixture.conversation, fixture.intent, fixture.goal)
	return ctx, run, nil
}

func gameSubmissionForTest(t *testing.T, conversation *Conversation, intent, goal string) agent.Middleware {
	return gameSubmissionFixture{BaseMiddleware: &agent.BaseMiddleware{}, t: t, conversation: conversation, intent: intent, goal: goal}
}
