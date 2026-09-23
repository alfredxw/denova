package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// External Goal evaluation uses a provider's read-only session fork. Native
// evaluation belongs to its own runtime and never enters this adapter.
func evaluateExternalGoal(ctx context.Context, fork func(context.Context, string) (*agent.Message, error)) (agent.GoalAfterRunDecision, error) {
	response, err := fork(ctx, goalEvaluationPrompt)
	var decision agent.GoalAfterRunDecision
	if response != nil && response.ResponseMeta != nil {
		decision.Usage, decision.FinishReason = response.ResponseMeta.Usage, response.ResponseMeta.FinishReason
	}
	if err != nil {
		return decision, err
	}
	if response == nil || len(response.ToolCalls) != 0 {
		return decision, errors.New("Goal evaluation requires a read-only response")
	}
	text := strings.TrimSpace(strings.ToValidUTF8(response.Content, "\uFFFD"))
	start := strings.IndexByte(text, '{')
	if len(text) > 256<<10 || start < 0 {
		return decision, errors.New("Goal evaluation requires a bounded JSON object")
	}
	var payload struct {
		Verdict string `json:"verdict"`
		Reason  string `json:"reason"`
		Next    string `json:"next_instruction"`
	}
	if err := json.NewDecoder(strings.NewReader(text[start:])).Decode(&payload); err != nil {
		return decision, err
	}
	switch strings.ToLower(strings.TrimSpace(payload.Verdict)) {
	case "complete", "completed":
		decision.Verdict = agent.GoalVerdictComplete
	case "block", "blocked":
		decision.Verdict = agent.GoalVerdictBlocked
	case "continue", "incomplete", "not_complete":
		decision.Verdict = agent.GoalVerdictContinue
	default:
		return decision, errors.New("invalid Goal evaluation verdict")
	}
	decision.Reason = strings.TrimSpace(payload.Reason)
	if decision.Verdict == agent.GoalVerdictContinue {
		decision.Input.Text = strings.TrimSpace(payload.Next)
	}
	if decision.Reason == "" || len(decision.Reason) > 64<<10 || (decision.Verdict == agent.GoalVerdictContinue && (decision.Input.Text == "" || len(decision.Input.Text) > 64<<10)) {
		return decision, errors.New("Goal evaluation fields are empty or too large")
	}
	return decision, nil
}

// Static suffix preserves the primary context's cache prefix. Provider forks
// supply evidence; the product, not the model, decides when evaluation may run.
const goalEvaluationPrompt = `[Goal evaluation request]
This is a one-turn, read-only evaluation side fork. Do not call tools. Evaluate the active goal from the preceding context against all work and evidence available in the conversation.
Return only one JSON object: {"verdict":"continue|complete|blocked","reason":"concise explanation","next_instruction":"concise instruction"}.
Use complete only when the entire objective, including required or implied verification, is achieved. A milestone, plan or unverified claim is not completion.
Use blocked only when no meaningful in-scope progress remains possible without user input or an external state change. Difficulty, uncertainty and remaining work are not blockers.
Use continue otherwise, with the most useful concrete next action. For complete or blocked, next_instruction must be empty. Write fields in the language of the objective.`
