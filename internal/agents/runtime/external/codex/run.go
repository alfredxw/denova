package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	agent "github.com/alfredxw/denova/agent"
)

type threadReply struct {
	Thread struct {
		ID string `json:"id"`
	} `json:"thread"`
	ReasoningEffort string `json:"reasoningEffort"`
}

func (c *Client) Version() string { return c.version }

type turnState struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Run resumes aligned threads and forks read-only evaluations. Model and question waits
// are bounded only by ctx; setup, interruption and cleanup have I/O deadlines.
func (c *Client) Run(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
	if input.Mode != external.OperationTurn {
		ctx = external.WithSteering(ctx, nil)
	}
	if input.Selection.Kind != config.RuntimeCodex || input.Selection.Codex == nil || host == nil {
		return external.Result{}, errors.New("Codex attempt requires its own model settings and host")
	}
	toolSet := make(map[string]bool, len(input.Tools))
	specs := make([]map[string]any, 0, len(input.Tools))
	for _, tool := range input.Tools {
		if tool.Name == "" || toolSet[tool.Name] || !json.Valid(tool.Schema) {
			return external.Result{}, errors.New("invalid or duplicate host tool definition")
		}
		toolSet[tool.Name] = true
		specs = append(specs, map[string]any{"name": tool.Name, "description": tool.Description, "inputSchema": tool.Schema})
	}
	if err := ctx.Err(); err != nil {
		return external.Result{}, err
	}
	// Admission RPCs retain their replies after a user cancellation so cleanup
	// still knows which remote thread/turn it must release. This deadline bounds
	// infrastructure only, never model execution or the user's answer wait.
	setup, cancelSetup := context.WithTimeout(context.WithoutCancel(ctx), infrastructureTimeout)
	defer cancelSetup()
	var thread threadReply
	model := input.Selection.Codex.Model
	if input.Selection.ModelProfileID() != "" {
		model = c.apiModel
	}
	if model == "" {
		return external.Result{}, errors.New("runtime API model was not resolved")
	}
	cwd := input.Directory
	if cwd == "" {
		cwd = c.cwd
	}
	method := "thread/start"
	params := map[string]any{
		"model": model, "cwd": cwd, "ephemeral": input.Directory == "",
		"sandbox": input.Selection.Codex.EffectiveSandbox(), "approvalPolicy": "never", "baseInstructions": input.Instructions,
		// Host tools own project access and receipts; the engine cwd is scratch space.
		"developerInstructions": "Use the provided host tools for all project reads and changes. The process working directory is temporary scratch space, not the project. Host tools enforce the selected permissions and report actual access failures; do not infer that a host tool is read-only from the process sandbox. Do not use built-in file or shell tools to bypass the host tools.",
		"dynamicTools":          specs,
		"config":                map[string]any{"tools.update_plan.enabled": input.Mode == external.OperationTurn},
	}
	if input.Mode != external.OperationTurn {
		// Fork retains the source thread's dynamic tool schemas. The empty host
		// allowlist rejects their execution; the sandbox also blocks built-in writes.
		params["sandbox"] = config.CodexReadOnly
		params["developerInstructions"] = "This is read-only context maintenance. Use only the supplied conversation and instructions. Do not call tools or ask questions."
	}
	if input.SessionID != "" {
		method = "thread/resume"
		params["threadId"] = input.SessionID
		delete(params, "ephemeral")
		if input.Mode == external.OperationEvaluate {
			method = "thread/fork"
			delete(params, "dynamicTools") // Fork has no dynamicTools override.
			params["ephemeral"] = true
			params["excludeTurns"] = true
		}
	}
	if err := c.call(setup, method, params, &thread); err != nil {
		var rpc *rpcError
		if input.SessionID != "" && errors.As(err, &rpc) && (strings.Contains(strings.ToLower(rpc.Message), "not found") || strings.Contains(strings.ToLower(rpc.Message), "no rollout")) {
			return external.Result{}, fmt.Errorf("%w: %v", external.ErrSessionUnavailable, err)
		}
		return external.Result{}, err
	}
	if thread.Thread.ID == "" {
		return external.Result{}, errors.New("App Server returned an empty thread ID")
	}
	threadID := thread.Thread.ID
	var turnID string
	completed := false
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), infrastructureTimeout)
		defer cancel()
		if !completed && turnID == "" {
			// A transport timeout can lose the turn/start reply even though the
			// server accepted it. Read the disposable thread before releasing it.
			var state struct {
				Thread struct {
					Turns []turnState `json:"turns"`
				} `json:"thread"`
			}
			if err := c.call(cleanup, "thread/read", map[string]any{"threadId": threadID, "includeTurns": true}, &state); err == nil {
				for _, turn := range state.Thread.Turns {
					if turn.Status == "inProgress" {
						turnID = turn.ID
					}
				}
			}
		}
		if !completed && turnID != "" {
			if err := c.call(cleanup, "turn/interrupt", map[string]string{"threadId": threadID, "turnId": turnID}, nil); err != nil {
				slog.Warn("[external-runtime] interrupt engine turn failed", "error", err)
			}
		}
		if err := c.call(cleanup, "thread/unsubscribe", map[string]string{"threadId": threadID}, nil); err != nil {
			slog.Warn("[external-runtime] release engine thread failed", "error", err)
		}
	}()
	if err := ctx.Err(); err != nil {
		return external.Result{}, err
	}
	if len(input.History) != 0 {
		items := make([]map[string]any, 0, len(input.History))
		for _, message := range input.History {
			if message.Role != "user" && message.Role != "assistant" {
				return external.Result{}, errors.New("external history must contain public user or assistant content")
			}
			kind := "input_text"
			if message.Role == "assistant" {
				kind = "output_text"
			}
			content := []map[string]string{{"type": kind, "text": message.Text}}
			for _, attachment := range append(append([]agent.Attachment(nil), message.Attachments...), message.ToolImages...) {
				if !agent.IsNativeImageMediaType(attachment.MediaType) {
					continue
				}
				url, err := agent.AttachmentDataURL(attachment)
				if err != nil {
					return external.Result{}, err
				}
				content = append(content, map[string]string{"type": "input_image", "image_url": url})
			}
			items = append(items, map[string]any{"type": "message", "role": message.Role, "content": content})
		}
		if err := c.call(setup, "thread/inject_items", map[string]any{"threadId": threadID, "items": items}, nil); err != nil {
			return external.Result{}, err
		}
	}
	sub, release, err := c.subscribe(threadID)
	if err != nil {
		return external.Result{}, err
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return external.Result{}, err
	}
	if input.Mode == external.OperationCompact {
		if err := c.call(setup, "thread/compact/start", map[string]string{"threadId": threadID}, nil); err != nil {
			return external.Result{}, err
		}
		cancelSetup()
		result, err := c.runTurn(ctx, sub, threadID, "", map[string]bool{}, host)
		completed = err == nil
		result.SessionID = threadID
		result = turnUsage(result, input.CumulativeUsage)
		return result, err
	}
	content, err := userInput(input)
	if err != nil {
		return external.Result{}, err
	}
	params = map[string]any{"threadId": threadID, "input": content}
	if input.Selection.Codex.Effort != "" {
		params["effort"] = input.Selection.Codex.Effort
	}
	effort := input.Selection.Codex.Effort
	if effort == "" {
		effort = thread.ReasoningEffort
	}
	slog.InfoContext(ctx, "[external-runtime] starting Codex model turn", "model", model, "effort", effort, "resumed", input.SessionID != "")
	var started struct {
		Turn turnState `json:"turn"`
	}
	if err := c.call(setup, "turn/start", params, &started); err != nil {
		return external.Result{}, err
	}
	turnID = started.Turn.ID
	if turnID == "" {
		return external.Result{}, errors.New("App Server returned an empty turn ID")
	}
	cancelSetup()
	if err := ctx.Err(); err != nil {
		return external.Result{}, err
	}
	result, err := c.runTurn(ctx, sub, threadID, turnID, toolSet, host)
	completed = result.Settled
	result.SessionID = threadID
	result = turnUsage(result, input.CumulativeUsage)
	return result, err
}

func turnUsage(result external.Result, previous *agent.TokenUsage) external.Result {
	result.CumulativeUsage = result.Usage
	if result.Usage == nil || previous == nil {
		return result
	}
	delta := *result.Usage
	delta.PromptTokens -= previous.PromptTokens
	delta.PromptTokenDetails.CachedTokens -= previous.PromptTokenDetails.CachedTokens
	delta.CompletionTokens -= previous.CompletionTokens
	delta.CompletionTokensDetails.ReasoningTokens -= previous.CompletionTokensDetails.ReasoningTokens
	delta.TotalTokens -= previous.TotalTokens
	if delta.PromptTokens < 0 || delta.CompletionTokens < 0 || delta.TotalTokens < 0 {
		// A provider reset its counter. Preserve an unknown increment instead of
		// charging historical consumption a second time.
		result.Usage = nil
	} else {
		result.Usage = &delta
	}
	return result
}

type toolExecution struct {
	call     external.ToolCall
	requests []json.RawMessage
	result   json.RawMessage
}

type toolCompletion struct {
	id     string
	result external.ToolResult
	err    error
}

func (c *Client) runTurn(ctx context.Context, sub *subscription, threadID, turnID string, tools map[string]bool, host external.Host) (result external.Result, runErr error) {
	var usage *agent.TokenUsage
	defer func() { result.Usage = usage }()
	toolContext, cancelTools := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancelTools(); workers.Wait() }()
	finishedTools := make(chan toolCompletion, 64)
	executions := map[string]*toolExecution{}
	output := messageOutput{items: map[string]string{}}
	inFlight := 0
	terminal := false
	manualCompaction := turnID == ""
	cancelled := ctx.Done()
	transport := ctx
	var drainDeadline <-chan struct{}
	controls := external.SteeringFromContext(ctx)
	var steering <-chan struct{}
	if controls != nil {
		steering = controls.Changed
	}
	for {
		if terminal && inFlight == 0 {
			return external.Result{Text: output.text(), Settled: true}, runErr
		}
		if terminal {
			steering = nil
		}
		select {
		case <-steering:
			if err := controls.Deliver(ctx, host, func(ctx context.Context, input external.Input) error {
				return c.steer(ctx, threadID, turnID, input)
			}); err != nil {
				return external.Result{}, fmt.Errorf("steer native turn: %w", err)
			}
		case <-cancelled:
			cancelled = nil
			steering = nil
			runErr = ctx.Err()
			drainContext, releaseDrain := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			defer releaseDrain()
			transport = drainContext
			drainDeadline = transport.Done()
			if err := c.call(transport, "turn/interrupt", map[string]string{"threadId": threadID, "turnId": turnID}, nil); err != nil {
				return external.Result{}, errors.Join(runErr, err)
			}
		case <-drainDeadline:
			return external.Result{}, errors.Join(runErr, errors.New("App Server interrupt did not settle"))
		case <-c.done:
			return external.Result{}, c.err
		case <-sub.done:
			return external.Result{}, sub.err
		case completion := <-finishedTools:
			inFlight--
			if completion.err != nil {
				if ctx.Err() == nil {
					return external.Result{}, completion.err
				}
				completion.result = external.ToolResult{Text: "Execution interrupted; consult confirmed product tool outcomes before continuing."}
			}
			execution := executions[completion.id]
			content := []map[string]string{{"type": "inputText", "text": completion.result.Text}}
			for _, attachment := range completion.result.Images {
				url, err := agent.AttachmentDataURL(attachment)
				if err != nil {
					return external.Result{}, err
				}
				content = append(content, map[string]string{"type": "inputImage", "imageUrl": url})
			}
			body, err := json.Marshal(map[string]any{"success": completion.result.Success, "contentItems": content})
			if err != nil {
				return external.Result{}, err
			}
			execution.result = body
			for _, id := range execution.requests {
				if err := c.send(transport, packet{ID: id, Result: body}); err != nil {
					return external.Result{}, err
				}
			}
			execution.requests = nil
		case msg := <-sub.events:
			var event struct {
				ThreadID     string    `json:"threadId"`
				TurnID       string    `json:"turnId"`
				ItemID       string    `json:"itemId"`
				Delta        string    `json:"delta"`
				SummaryIndex int       `json:"summaryIndex"`
				Turn         turnState `json:"turn"`
				Item         struct {
					ID   string `json:"id"`
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"item"`
				CallID    string          `json:"callId"`
				Tool      string          `json:"tool"`
				Arguments json.RawMessage `json:"arguments"`
				WillRetry bool            `json:"willRetry"`
				Plan      []struct {
					Step   string `json:"step"`
					Status string `json:"status"`
				} `json:"plan"`
				TokenUsage *struct {
					Total struct {
						Input     int `json:"inputTokens"`
						Cached    int `json:"cachedInputTokens"`
						Output    int `json:"outputTokens"`
						Reasoning int `json:"reasoningOutputTokens"`
						Total     int `json:"totalTokens"`
					} `json:"total"`
				} `json:"tokenUsage"`
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(msg.Params, &event); err != nil {
				return external.Result{}, err
			}
			if turnID == "" && msg.Method == "turn/started" {
				turnID = event.Turn.ID
			}
			if event.ThreadID != threadID {
				return external.Result{}, fmt.Errorf("App Server %s callback belongs to another thread", msg.Method)
			}
			callbackTurn := event.TurnID
			if callbackTurn == "" {
				callbackTurn = event.Turn.ID
			}
			if callbackTurn != "" && callbackTurn != turnID {
				if len(msg.ID) != 0 {
					return external.Result{}, fmt.Errorf("App Server %s request belongs to another turn", msg.Method)
				}
				// Resuming a persisted thread replays its last token/status edge.
				// Only the newly admitted turn may produce display or usage facts.
				continue
			}
			switch msg.Method {
			case "item/tool/call":
				if terminal {
					return external.Result{}, errors.New("App Server requested a tool after turn completion")
				}
				if len(msg.ID) == 0 || event.CallID == "" || !tools[event.Tool] || !json.Valid(event.Arguments) {
					return external.Result{}, errors.New("App Server requested an invalid or unregistered host tool")
				}
				call := external.ToolCall{ID: event.CallID, Name: event.Tool, Arguments: event.Arguments}
				if previous := executions[call.ID]; previous != nil {
					if previous.call.Name != call.Name || !equalArguments(previous.call.Arguments, call.Arguments) {
						return external.Result{}, errors.New("App Server reused a call ID with different arguments")
					}
					if previous.result != nil {
						if err := c.send(transport, packet{ID: msg.ID, Result: previous.result}); err != nil {
							return external.Result{}, err
						}
					} else {
						previous.requests = append(previous.requests, msg.ID)
					}
					continue
				}
				if inFlight >= cap(finishedTools) {
					return external.Result{}, errors.New("App Server concurrent tool capacity exceeded")
				}
				executions[call.ID] = &toolExecution{call: call, requests: []json.RawMessage{msg.ID}}
				inFlight++
				workers.Add(1)
				go func() {
					defer workers.Done()
					completion := toolCompletion{id: call.ID}
					defer func() {
						if recovered := recover(); recovered != nil {
							slog.Error("[external-runtime] host tool panicked", "tool", call.Name, "panic", recovered)
							completion.err = errors.New("external host tool panicked")
						}
						finishedTools <- completion
					}()
					completion.result, completion.err = host.CallTool(toolContext, call)
				}()
			case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
				if err := c.send(transport, packet{ID: msg.ID, Result: json.RawMessage(`{"decision":"decline"}`)}); err != nil {
					return external.Result{}, err
				}
			case "item/permissions/requestApproval":
				if err := c.send(transport, packet{ID: msg.ID, Result: json.RawMessage(`{"permissions":{},"scope":"turn"}`)}); err != nil {
					return external.Result{}, err
				}
			case "item/tool/requestUserInput":
				// Questions use the registered ask schema. This endpoint must not
				// turn an upstream permission request into a product question.
				if err := c.send(transport, packet{ID: msg.ID, Error: &rpcError{Code: -32601, Message: "Use the ask host tool for questions; interactive approval is unavailable"}}); err != nil {
					return external.Result{}, err
				}
			case "item/agentMessage/delta":
				if err := output.append(host, event.ItemID, event.Delta); err != nil && ctx.Err() == nil {
					return external.Result{}, err
				}
			case "item/reasoning/summaryTextDelta":
				// Only the provider's public summary is displayable. It belongs
				// to Task replay, never canonical assistant content/model history.
				if event.Delta != "" {
					if err := host.Emit(agentrun.Event{Type: "thinking", Data: map[string]any{
						"content": event.Delta, "display_segment_id": fmt.Sprintf("%s-summary-%s-%d", turnID, event.ItemID, event.SummaryIndex),
					}}); err != nil && ctx.Err() == nil {
						return external.Result{}, err
					}
				}
			case "item/started", "item/completed":
				if event.Item.Type == "contextCompaction" {
					status := "started"
					phase := "model_step"
					if manualCompaction {
						phase = "agent"
					}
					if msg.Method == "item/completed" {
						status = "completed"
					}
					if err := host.Emit(agentrun.Event{Type: "context_compaction", Data: map[string]any{"id": event.Item.ID, "status": status, "automatic": !manualCompaction, "runtime_managed": true, "phase": phase}}); err != nil && ctx.Err() == nil {
						return external.Result{}, err
					}
				}
				if event.Item.Type == "agentMessage" && msg.Method == "item/completed" {
					if err := output.complete(host, event.Item.ID, event.Item.Text); err != nil && ctx.Err() == nil {
						return external.Result{}, err
					}
				}
			case "turn/completed":
				if event.Turn.ID != turnID {
					return external.Result{}, errors.New("App Server completed a different turn")
				}
				switch event.Turn.Status {
				case "completed":
					terminal = true
				case "interrupted":
					terminal, runErr = true, context.Canceled
				case "failed":
					if event.Turn.Error != nil {
						return external.Result{}, fmt.Errorf("App Server turn failed: %s", event.Turn.Error.Message)
					}
					return external.Result{}, errors.New("App Server turn failed")
				default:
					return external.Result{}, fmt.Errorf("unknown App Server terminal status %q", event.Turn.Status)
				}
			case "error":
				if !event.WillRetry {
					return external.Result{}, fmt.Errorf("App Server turn error: %s", event.Error.Message)
				}
			case "thread/tokenUsage/updated":
				if event.TokenUsage != nil {
					// Every operation owns a new thread. Notifications replace the
					// cumulative total; adding them would count earlier requests twice.
					total := event.TokenUsage.Total
					usage = &agent.TokenUsage{PromptTokens: total.Input, CompletionTokens: total.Output, TotalTokens: total.Total}
					usage.PromptTokenDetails.CachedTokens = total.Cached
					usage.CompletionTokensDetails.ReasoningTokens = total.Reasoning
				}
			case "turn/plan/updated":
				items := make([]agent.TodoItem, 0, len(event.Plan))
				for index, step := range event.Plan {
					status := agent.TodoStatus(step.Status)
					if step.Status == "inProgress" {
						status = agent.TodoInProgress
					}
					items = append(items, agent.TodoItem{ID: fmt.Sprint(index + 1), Text: step.Step, Status: status})
				}
				if err := host.Emit(external.PlanEvent(items)); err != nil {
					return external.Result{}, err
				}
			case "turn/started", "thread/status/changed", "turn/diff/updated", "item/reasoning/summaryPartAdded", "item/reasoning/textDelta":
				// These are not completion facts. Private reasoning is omitted.
			default:
				if len(msg.ID) != 0 {
					return external.Result{}, fmt.Errorf("unsupported App Server request %q", msg.Method)
				}
				slog.Debug("[external-runtime] ignored extension notification", "method", msg.Method)
			}
		}
	}
}

func equalArguments(left, right json.RawMessage) bool {
	var a, b any
	if json.Unmarshal(left, &a) != nil || json.Unmarshal(right, &b) != nil {
		return false
	}
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

type messageOutput struct {
	items map[string]string
	order []string
}

func (output *messageOutput) append(host external.Host, id, text string) error {
	if id == "" {
		return errors.New("App Server message has no item ID")
	}
	if _, exists := output.items[id]; !exists {
		if len(output.order) > 0 {
			text = "\n\n" + text
		}
		output.order = append(output.order, id)
		output.items[id] = ""
	}
	output.items[id] += text
	if text == "" {
		return nil
	}
	return host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": text}})
}

func (output *messageOutput) complete(host external.Host, id, full string) error {
	previous := output.items[id]
	if len(output.order) > 0 && output.order[0] != id {
		previous = strings.TrimPrefix(previous, "\n\n")
	}
	if !strings.HasPrefix(full, previous) {
		return errors.New("App Server final message differs from streamed content")
	}
	return output.append(host, id, strings.TrimPrefix(full, previous))
}

func (output *messageOutput) text() string {
	var text strings.Builder
	for _, id := range output.order {
		text.WriteString(output.items[id])
	}
	return text.String()
}
