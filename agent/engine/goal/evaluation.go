package goal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agentmodel "github.com/alfredxw/denova/agent/model"
	agentschema "github.com/alfredxw/denova/agent/schema"
)

const (
	maxGoalEvaluationBytes        = 256 << 10
	maxGoalEvaluationFieldBytes   = 64 << 10
	maxGoalEvaluationOutputTokens = 512
)

// goalEvaluationPrompt is deliberately static. The exact active Goal,
// workspace context, tools, evidence, and final response already exist in the
// forked primary request, so interpolating them again would waste tokens and
// weaken provider prefix-cache reuse.
const goalEvaluationPrompt = `[Goal evaluation request]
This is a one-turn, read-only evaluation side fork. Do not call tools. Evaluate the active goal from the preceding context against all work and evidence available in the conversation.

Return only one JSON object with this exact shape:
{"verdict":"continue|complete|blocked","reason":"concise explanation","next_instruction":"concise instruction"}

Evaluation rules:
- Use "complete" only when the entire active objective is achieved, including required or implied verification. A milestone, partial implementation, unverified claim, or plan is not completion.
- Use "blocked" only when no meaningful in-scope progress remains possible without user input or an external state change. Difficulty, uncertainty, or remaining work is not a blocker.
- Use "continue" in every other case. Set next_instruction to the most useful concrete action for the next autonomous turn.
- For complete or blocked, next_instruction must be an empty string.
- Write reason and next_instruction in the same language as the active objective. Do not include Markdown fences, commentary, or any text outside the JSON object.`

type evaluationPayload struct {
	Verdict         string `json:"verdict"`
	Reason          string `json:"reason"`
	NextInstruction string `json:"next_instruction"`
}

func (manager *standardManager) AfterRun(ctx context.Context, request GoalAfterRunRequest) (GoalAfterRunDecision, error) {
	if !request.Present || !request.State.Active() || request.Result.Status != agentschema.ResultCompleted {
		return GoalAfterRunDecision{}, nil
	}
	if request.ModelRequest == nil || request.Final == nil || request.Final.Role != agentschema.Assistant || len(request.Final.ToolCalls) != 0 {
		return GoalAfterRunDecision{}, errors.New("Goal evaluation requires the exact final model request and canonical assistant result")
	}
	// Preserve the primary request's tool schemas so provider prompt-cache
	// prefixes remain intact (tools precede system and messages). Read-only
	// enforcement is guaranteed by rejecting any returned tool calls below.
	fork := request.ModelRequest.Append(request.Final.Clone(), agentschema.UserMessage(goalEvaluationPrompt)).WithOptions(
		agentmodel.WithMaxTokens(maxGoalEvaluationOutputTokens),
	)
	response, err := executeEvaluationFork(ctx, fork)
	decision := evaluationMetadata(response)
	if err != nil {
		return decision, err
	}
	if response == nil {
		return decision, errors.New("Goal evaluation returned no response")
	}
	if len(response.ToolCalls) != 0 {
		return decision, fmt.Errorf("Goal evaluation denied %d requested tool call(s)", len(response.ToolCalls))
	}
	payload, err := decodeEvaluationPayload(response.Content)
	if err != nil {
		return decision, err
	}
	decision.Verdict = GoalVerdict(payload.Verdict)
	decision.Reason = payload.Reason
	if decision.Verdict == GoalVerdictContinue {
		decision.Input = agentschema.Input{Text: payload.NextInstruction}
	}
	return decision, nil
}

func executeEvaluationFork(ctx context.Context, snapshot *agentmodel.ModelRequestSnapshot) (*agentschema.Message, error) {
	if snapshot == nil {
		return nil, errors.New("Goal evaluation model request is unavailable")
	}
	return snapshot.Generate(ctx)
}

func evaluationMetadata(response *agentschema.Message) GoalAfterRunDecision {
	decision := GoalAfterRunDecision{}
	if response == nil || response.ResponseMeta == nil {
		return decision
	}
	decision.FinishReason = response.ResponseMeta.FinishReason
	if response.ResponseMeta.Usage != nil {
		usage := *response.ResponseMeta.Usage
		decision.Usage = &usage
	}
	return decision
}

func decodeEvaluationPayload(content string) (evaluationPayload, error) {
	content = strings.TrimSpace(strings.ToValidUTF8(content, "\uFFFD"))
	if content == "" || len(content) > maxGoalEvaluationBytes {
		return evaluationPayload{}, fmt.Errorf("Goal evaluation response must contain 1..%d bytes", maxGoalEvaluationBytes)
	}
	start := strings.IndexByte(content, '{')
	if start < 0 {
		return evaluationPayload{}, errors.New("Goal evaluation response contains no JSON object")
	}
	var payload evaluationPayload
	if err := json.NewDecoder(strings.NewReader(content[start:])).Decode(&payload); err != nil {
		return evaluationPayload{}, fmt.Errorf("decode Goal evaluation response: %w", err)
	}
	payload.Verdict = normalizeEvaluationVerdict(payload.Verdict)
	payload.Reason = strings.TrimSpace(payload.Reason)
	payload.NextInstruction = strings.TrimSpace(payload.NextInstruction)
	if payload.Verdict == "" {
		return evaluationPayload{}, errors.New("Goal evaluation response has an invalid verdict")
	}
	if payload.Reason == "" || len(payload.Reason) > maxGoalEvaluationFieldBytes {
		return evaluationPayload{}, fmt.Errorf("Goal evaluation reason must contain 1..%d bytes", maxGoalEvaluationFieldBytes)
	}
	if payload.Verdict == string(GoalVerdictContinue) {
		if payload.NextInstruction == "" || len(payload.NextInstruction) > maxGoalEvaluationFieldBytes {
			return evaluationPayload{}, fmt.Errorf("Goal continuation instruction must contain 1..%d bytes", maxGoalEvaluationFieldBytes)
		}
	} else {
		payload.NextInstruction = ""
	}
	return payload, nil
}

func normalizeEvaluationVerdict(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "continue", "incomplete", "not_complete":
		return string(GoalVerdictContinue)
	case "complete", "completed":
		return string(GoalVerdictComplete)
	case "block", "blocked":
		return string(GoalVerdictBlocked)
	default:
		return ""
	}
}
