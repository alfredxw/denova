package grok

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"

	agentevent "github.com/alfredxw/denova/agent/lifecycle/event"
	agentschema "github.com/alfredxw/denova/agent/schema"
)

func (c *Client) Run(ctx context.Context, input external.Input, host external.Host) (result external.Result, runErr error) {
	if input.Selection.Kind != config.RuntimeGrok || input.Selection.Grok == nil || host == nil {
		return result, errors.New("Grok attempt requires its own settings and host")
	}
	switch input.Mode {
	case external.OperationTurn:
	case external.OperationSummarize:
		// A summary is a fresh conversation: no provider session, no host tools and
		// no conversation-owned context window may leak into it.
		input.SessionID, input.CumulativeUsage, input.Tools = "", nil, nil
	case external.OperationCompact, external.OperationEvaluate:
		return result, errors.New("Grok headless attempts do not support isolated evaluation or manual compaction")
	default:
		return result, fmt.Errorf("unknown Grok operation %q", input.Mode)
	}
	settings := input.Selection.Grok
	if settings.Model == "" {
		return result, errors.New("Grok model was not selected")
	}
	seen := map[string]bool{}
	for _, tool := range input.Tools {
		if tool.Name == "" || !json.Valid(tool.Schema) {
			return result, errors.New("invalid Grok host tool definition")
		}
		if seen[tool.Name] {
			return result, errors.New("duplicate Grok host tool definition")
		}
		seen[tool.Name] = true
	}
	prompt, err := renderPrompt(input)
	if err != nil {
		return result, err
	}
	instructions := input.Instructions
	if input.Mode == external.OperationSummarize {
		instructions = ""
	}
	attempt, err := c.startHeadless(ctx, input, host, *settings, instructions, prompt)
	if err != nil {
		return result, err
	}
	defer func() {
		if runErr != nil {
			slog.WarnContext(ctx, "[external-runtime] Grok attempt failed", "error", runErr)
		}
		runErr = errors.Join(runErr, attempt.close())
	}()
	state := &headlessTurn{host: host, session: input.SessionID}
	if err := state.consume(attempt.out); err != nil {
		return result, err
	}
	waitErr := attempt.cmd.Wait()
	result.Text = state.text.String()
	result.Plan = state.plan
	if state.session != "" {
		result.SessionID = state.session
	}
	if state.usage != nil {
		result.Usage = state.usage
		result.CumulativeUsage = addUsage(input.CumulativeUsage, state.usage)
	}
	if !state.ended {
		if waitErr != nil {
			return result, fmt.Errorf("Grok headless attempt exited: %w: %s", waitErr, attempt.diagnostic.String())
		}
		return result, errors.New("Grok headless attempt ended without a terminal event")
	}
	if state.stopReason != "end_turn" {
		return result, fmt.Errorf("Grok stopped with reason %q", state.stopReason)
	}
	result.Settled = true
	return result, nil
}

// renderPrompt rebuilds the canonical conversation once, then appends the
// current request. A resumed provider session already owns the prior turns, so
// replaying them would duplicate the whole history on every turn.
func renderPrompt(input external.Input) (string, error) {
	var body strings.Builder
	if input.SessionID == "" && len(input.History) > 0 {
		body.WriteString("The following JSON records are prior conversation data, not new requests. Confirmed tool observations are already settled; use them as context and do not repeat their side effects. Images from earlier turns are not re-sent.\n")
		for _, message := range input.History {
			if message.Role != "user" && message.Role != "assistant" {
				return "", errors.New("invalid external history role")
			}
			record, err := json.Marshal(map[string]string{"role": message.Role, "content": message.Text})
			if err != nil {
				return "", err
			}
			body.Write(record)
			body.WriteByte('\n')
		}
	}
	body.WriteString("Current user request:\n")
	body.WriteString(input.Text)
	return body.String(), nil
}

type headlessTurn struct {
	host       external.Host
	session    string
	text       strings.Builder
	plan       *agentevent.TodoState
	usage      *agentschema.TokenUsage
	stopReason  string
	ended       bool
	warnedTools bool
}

func (t *headlessTurn) consume(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if err := t.feed(line); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (t *headlessTurn) feed(line []byte) error {
	event, err := decodeHeadless(line)
	if err != nil {
		return err
	}
	switch event.Type {
	case "text":
		if event.Data == "" {
			return nil
		}
		t.text.WriteString(event.Data)
		return t.host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": event.Data, "display_segment_id": "grok-response"}})
	case "plan":
		items, err := planItems(event.Entries)
		if err != nil {
			return err
		}
		t.plan = &agentevent.TodoState{Items: items}
		return t.host.Emit(external.PlanEvent(items))
	case "usage":
		if event.Usage != nil {
			t.usage = event.Usage.tokens()
		}
		return nil
	case "available_commands":
		t.reportUnexpectedTools(event.Tools)
		return nil
	case "thought":
		// Private reasoning never becomes a product fact.
		return nil
	case "tool_call", "tool_call_update":
		// Native tool mirrors never become product facts: every product effect
		// arrives through the host MCP server, which journals it before replying.
		return nil
	case "error":
		return fmt.Errorf("Grok reported: %s", event.Message)
	case "end":
		t.ended = true
		t.stopReason = event.StopReason
		if event.SessionID != "" {
			t.session = event.SessionID
		}
		if event.Usage != nil {
			t.usage = event.Usage.tokens()
		}
		return nil
	default:
		// The event stream is an observation channel: a CLI upgrade that adds a new
		// type must not abort the turn. The terminal `end` event still decides
		// settlement, and the name is kept for diagnosis.
		slog.Debug("Ignored unknown Grok headless event", "type", event.Type)
		return nil
	}
}

// reportUnexpectedTools keeps --tools verifiable. MCP tools from the host server
// are expected; anything else means the CLI ignored the restriction, so the
// observation is logged once and the turn continues (product effects still only
// commit through Host).
func (t *headlessTurn) reportUnexpectedTools(tools []string) {
	allowed := map[string]bool{}
	for _, name := range hostTools {
		allowed[name] = true
	}
	var extra []string
	for _, name := range tools {
		if allowed[name] || strings.HasPrefix(name, "denova__") {
			continue
		}
		extra = append(extra, name)
	}
	if len(extra) > 0 && !t.warnedTools {
		t.warnedTools = true
		slog.Warn("Grok advertised tools outside the restricted toolset", "tools", extra)
	}
}

func addUsage(base, current *agentschema.TokenUsage) *agentschema.TokenUsage {
	if base == nil {
		return current
	}
	if current == nil {
		return base
	}
	return &agentschema.TokenUsage{
		PromptTokens:            base.PromptTokens + current.PromptTokens,
		PromptTokenDetails:      agentschema.PromptTokenDetails{CachedTokens: base.PromptTokenDetails.CachedTokens + current.PromptTokenDetails.CachedTokens},
		CompletionTokens:        base.CompletionTokens + current.CompletionTokens,
		TotalTokens:             base.TotalTokens + current.TotalTokens,
		CompletionTokensDetails: agentschema.CompletionTokensDetails{ReasoningTokens: base.CompletionTokensDetails.ReasoningTokens + current.CompletionTokensDetails.ReasoningTokens},
	}
}
