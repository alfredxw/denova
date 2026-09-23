package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	agent "github.com/alfredxw/denova/agent"
)

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
}
type streamMessage struct {
	ID      string        `json:"id"`
	Content contentBlocks `json:"content"`
}

// Local slash-command replies and user mirrors use a string; assistant API
// messages use blocks. Both are public CLI wire variants.
type contentBlocks []contentBlock

func (blocks *contentBlocks) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		*blocks = []contentBlock{{Type: "text", Text: text}}
		return nil
	}
	type plain contentBlocks
	return json.Unmarshal(data, (*plain)(blocks))
}

type streamFrame struct {
	SessionID     string          `json:"session_id"`
	UUID          string          `json:"uuid"`
	Tools         []string        `json:"tools"`
	Type          string          `json:"type"`
	Subtype       string          `json:"subtype"`
	Parent        string          `json:"parent_tool_use_id"`
	IsMeta        bool            `json:"is_meta"`
	ToolUseResult json.RawMessage `json:"tool_use_result"`
	Message       streamMessage   `json:"message"`
	Event         struct {
		Type    string        `json:"type"`
		Index   int           `json:"index"`
		Message streamMessage `json:"message"`
		Delta   contentBlock  `json:"delta"`
		Block   contentBlock  `json:"content_block"`
	} `json:"event"`
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
	Usage   *struct {
		Input      int `json:"input_tokens"`
		Output     int `json:"output_tokens"`
		CacheRead  int `json:"cache_read_input_tokens"`
		CacheWrite int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

// streamOutput merges block deltas with complete assistant wrappers regardless
// of arrival order. Only a root result frame terminates the product attempt.
// Tool observations never execute tools: MCP is the sole invocation path.
type streamOutput struct {
	sessionID        string
	manualCompaction bool
	tools            map[string]bool
	initialized      bool
	current          string
	order            []string
	text             map[string]string
	complete         map[string]bool
	wrapperOffsets   map[string]int
	seenWrappers     map[string]bool
	terminal         bool
	usage            *agent.TokenUsage
	plan             []agent.TodoItem
	planCalls        map[string]contentBlock
	planObserved     bool
}

func (s *streamOutput) feed(line []byte, host external.Host) error {
	var f streamFrame
	if err := json.Unmarshal(line, &f); err != nil {
		return fmt.Errorf("decode Claude stream: %w", err)
	}
	if f.Parent != "" {
		return nil
	}
	if f.SessionID != "" {
		s.sessionID = f.SessionID
	}
	if s.text == nil {
		s.text = map[string]string{}
		s.complete = map[string]bool{}
		s.wrapperOffsets = map[string]int{}
		s.seenWrappers = map[string]bool{}
	}
	if s.terminal {
		return nil
	}
	switch f.Type {
	case "stream_event":
		e := f.Event
		switch e.Type {
		case "message_start":
			s.current = e.Message.ID
		case "content_block_start":
			if e.Block.Type == "text" && e.Block.Text != "" {
				return s.append(host, fmt.Sprintf("%s:%d", s.current, e.Index), e.Block.Text, false)
			}
		case "content_block_delta":
			if e.Delta.Type == "text_delta" {
				return s.append(host, fmt.Sprintf("%s:%d", s.current, e.Index), e.Delta.Text, false)
			}
			// Private thinking is deliberately not copied into product history.
		}
	case "assistant":
		if f.IsMeta {
			return nil
		}
		if f.Message.ID == "" {
			return errors.New("Claude assistant message lacks ID")
		}
		if f.UUID != "" {
			if s.seenWrappers[f.UUID] {
				return nil
			}
			s.seenWrappers[f.UUID] = true
		}
		// CLI wrappers contain newly completed blocks, not a cumulative message.
		// Include thinking and tool blocks in the offset so wrapper positions
		// match the original stream indices. Text equality is not block identity.
		offset := s.wrapperOffsets[f.Message.ID]
		s.wrapperOffsets[f.Message.ID] += len(f.Message.Content)
		for i, block := range f.Message.Content {
			if block.Type == "tool_use" && slices.Contains([]string{"TaskCreate", "TaskUpdate", "TaskList", "TaskGet"}, block.Name) {
				if s.planCalls == nil {
					s.planCalls = map[string]contentBlock{}
				}
				s.planCalls[block.ID] = block
			}
			if block.Type == "text" {
				if err := s.append(host, fmt.Sprintf("%s:%d", f.Message.ID, offset+i), block.Text, true); err != nil {
					return err
				}
			}
		}
	case "result":
		s.terminal = true
		if u := f.Usage; u != nil {
			s.usage = &agent.TokenUsage{PromptTokens: u.Input + u.CacheRead + u.CacheWrite, CompletionTokens: u.Output, TotalTokens: u.Input + u.CacheRead + u.CacheWrite + u.Output}
			s.usage.PromptTokenDetails.CachedTokens = u.CacheRead
		}
		if f.IsError || f.Subtype != "success" {
			return fmt.Errorf("Claude attempt failed (%s)", f.Subtype)
		}
		if len(s.order) == 0 && f.Result != "" {
			return s.append(host, "result", f.Result, true)
		}
	case "system":
		if f.Subtype == "compact_boundary" {
			phase := "model_step"
			if s.manualCompaction {
				phase = "agent"
			}
			return host.Emit(agentrun.Event{Type: "context_compaction", Data: map[string]any{
				"id": f.UUID, "status": "completed", "automatic": !s.manualCompaction, "runtime_managed": true, "phase": phase,
			}})
		}
		if f.Subtype == "init" {
			s.initialized = true
			for _, name := range f.Tools {
				if !s.tools[name] && name != "EndConversation" {
					return fmt.Errorf("Claude exposed an unscoped tool %q", name)
				}
			}
			for name := range s.tools {
				if !slices.Contains(f.Tools, name) {
					return fmt.Errorf("Claude did not expose host tool %q", name)
				}
			}
		}
	case "user":
		for _, block := range f.Message.Content {
			call, found := s.planCalls[block.ToolUseID]
			if !found {
				continue
			}
			delete(s.planCalls, block.ToolUseID)
			if block.IsError {
				continue
			}
			if err := s.observePlan(host, call, f.ToolUseResult); err != nil {
				return err
			}
		}
	case "rate_limit_event":
		// Initialization, tool-result mirrors and advisory limits are not
		// canonical answers or terminal outcomes.
	default:
		// CLI adds diagnostic frames across versions. Completion still requires
		// a recognized root result, so an unknown frame cannot fabricate success.
	}
	return nil
}

func (s *streamOutput) append(host external.Host, key, value string, complete bool) error {
	if strings.HasPrefix(key, ":") {
		return errors.New("Claude text delta lacks message identity")
	}
	if s.complete[key] {
		return nil
	}
	prior, exists := s.text[key]
	if !exists {
		s.order = append(s.order, key)
	}
	delta := value
	if complete {
		if !strings.HasPrefix(value, prior) {
			return errors.New("Claude final text disagrees with streamed text")
		}
		delta = strings.TrimPrefix(value, prior)
		s.complete[key] = true
	}
	s.text[key] = prior + delta
	if delta == "" {
		return nil
	}
	return host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": delta, "display_segment_id": key}})
}

func (s *streamOutput) result() external.Result {
	var text strings.Builder
	for _, key := range s.order {
		text.WriteString(s.text[key])
	}
	result := external.Result{Text: text.String(), Usage: s.usage, SessionID: s.sessionID}
	if s.planObserved {
		result.Plan = &agent.TodoState{Items: append([]agent.TodoItem{}, s.plan...)}
	}
	return result
}
