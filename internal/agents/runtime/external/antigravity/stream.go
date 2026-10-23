package antigravity

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	agentschema "github.com/alfredxw/denova/agent/schema"
)

type tokenUsage struct {
	Input     int `json:"input_tokens"`
	Output    int `json:"output_tokens"`
	Thinking  int `json:"thinking_tokens"`
	CacheRead int `json:"cache_read_tokens"`
	Total     int `json:"total_tokens"`
}

func (usage tokenUsage) tokens() *agentschema.TokenUsage {
	result := &agentschema.TokenUsage{PromptTokens: usage.Input, CompletionTokens: usage.Output, TotalTokens: usage.Total}
	result.PromptTokenDetails.CachedTokens = usage.CacheRead
	result.CompletionTokensDetails.ReasoningTokens = usage.Thinking
	return result
}

type streamFrame struct {
	Event          string `json:"event"`
	ConversationID string `json:"conversation_id"`
	Init           struct {
		Tools []string `json:"tools"`
	} `json:"init"`
	Step struct {
		ConversationID string `json:"conversation_id"`
		Index          int    `json:"step_index"`
		State          string `json:"state"`
		Type           string `json:"step_type"`
		TextDelta      string `json:"text_delta"`
	} `json:"step_update"`
	Result struct {
		ConversationID string      `json:"conversation_id"`
		Status         string      `json:"status"`
		Response       string      `json:"response"`
		Error          string      `json:"error"`
		Usage          *tokenUsage `json:"usage"`
	} `json:"result"`
}

type streamOutput struct {
	initialized bool
	terminal    bool
	streamed    bool
	allowed     map[string]bool
	result      external.Result
}

func (s *streamOutput) feed(line []byte, host external.Host) error {
	var frame streamFrame
	if err := json.Unmarshal(line, &frame); err != nil {
		return fmt.Errorf("decode Antigravity stream: %w", err)
	}
	if s.terminal {
		return errors.New("Antigravity emitted data after its terminal result")
	}
	switch frame.Event {
	case "init":
		if s.initialized || frame.ConversationID == "" {
			return errors.New("invalid Antigravity initialization")
		}
		// Verified on CLI 1.4.2: a scoped agent still receives the full native
		// toolset in init (run_command, write_to_file, browser_*), because the
		// frontmatter mounts components instead of filtering them. Provider tools
		// are therefore an observation this adapter cannot refuse — --sandbox is
		// what contains them — while product effects still reach the journal only
		// through the host dispatcher. A scoped tool that is missing stays fatal:
		// product tools would be unreachable.
		var unscoped []string
		for _, tool := range frame.Init.Tools {
			if !s.allowed[tool] {
				unscoped = append(unscoped, tool)
			}
		}
		if len(unscoped) > 0 {
			slog.Warn("Antigravity advertised tools outside the scoped catalog", "tools", unscoped)
		}
		for name := range s.allowed {
			found := false
			for _, tool := range frame.Init.Tools {
				found = found || name == tool
			}
			if !found {
				return fmt.Errorf("Antigravity did not expose scoped tool %q", name)
			}
		}
		s.initialized, s.result.SessionID = true, frame.ConversationID
	case "step_update":
		if !s.initialized || frame.Step.ConversationID != s.result.SessionID {
			return errors.New("Antigravity step lacks its initialized conversation identity")
		}
		// ACTIVE and DONE carry agent_response deltas. The CLI also reports a
		// step-level terminal state before the attempt verdict: a failed or
		// canceled step is an observation, not the attempt result, so keep
		// streaming and let the result status decide success or failure.
		switch frame.Step.State {
		case "ACTIVE", "DONE":
		case "ERROR", "CANCELED", "INTERRUPTED":
			return nil
		default:
			return fmt.Errorf("unknown Antigravity step state %q", frame.Step.State)
		}
		// Tool events are observations only: MCP is the sole tool execution path.
		// Private reasoning and subagent traces never become product messages.
		if frame.Step.Type == "agent_response" && frame.Step.TextDelta != "" {
			s.streamed = true
			s.result.Text += frame.Step.TextDelta
			return host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": frame.Step.TextDelta, "display_segment_id": fmt.Sprintf("antigravity:%d", frame.Step.Index)}})
		}
	case "result":
		s.terminal = true
		if frame.Result.Usage != nil {
			s.result.CumulativeUsage = frame.Result.Usage.tokens()
		}
		if frame.Result.ConversationID != "" {
			if s.result.SessionID != "" && s.result.SessionID != frame.Result.ConversationID {
				return errors.New("Antigravity result changed conversation identity")
			}
			s.result.SessionID = frame.Result.ConversationID
		}
		if frame.Result.Response != "" || !s.streamed {
			s.result.Text = frame.Result.Response
		}
		switch frame.Result.Status {
		case "SUCCESS":
			if !s.initialized {
				return errors.New("Antigravity omitted initialization")
			}
			if !s.streamed && frame.Result.Response != "" {
				return host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": frame.Result.Response, "display_segment_id": "antigravity:result"}})
			}
		case "ERROR", "CANCELED", "INTERRUPTED", "INVALID", "WAITING", "RUNNING":
			return fmt.Errorf("Antigravity attempt ended with status %s: %s", frame.Result.Status, frame.Result.Error)
		default:
			return fmt.Errorf("unknown Antigravity result status %q", frame.Result.Status)
		}
	default:
		return fmt.Errorf("unknown Antigravity stream event %q", frame.Event)
	}
	return nil
}

// The CLI reports cumulative session usage, including resumed turns. Journal
// accounting records the delta; the new total is only a disposable cache baseline.
func (s *streamOutput) complete(previous *agentschema.TokenUsage) external.Result {
	result := s.result
	if result.CumulativeUsage != nil {
		usage := *result.CumulativeUsage
		if previous != nil {
			usage.PromptTokens = max(0, usage.PromptTokens-previous.PromptTokens)
			usage.CompletionTokens = max(0, usage.CompletionTokens-previous.CompletionTokens)
			usage.TotalTokens = max(0, usage.TotalTokens-previous.TotalTokens)
			usage.PromptTokenDetails.CachedTokens = max(0, usage.PromptTokenDetails.CachedTokens-previous.PromptTokenDetails.CachedTokens)
			usage.CompletionTokensDetails.ReasoningTokens = max(0, usage.CompletionTokensDetails.ReasoningTokens-previous.CompletionTokensDetails.ReasoningTokens)
		}
		result.Usage = &usage
	}
	return result
}
