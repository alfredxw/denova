package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"denova/internal/agents/external"
	agentrun "denova/internal/agents/run"
	agent "github.com/alfredxw/denova/agent"
)

type contentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
}
type streamMessage struct {
	ID      string         `json:"id"`
	Content []contentBlock `json:"content"`
}
type streamFrame struct {
	Tools   []string      `json:"tools"`
	Type    string        `json:"type"`
	Subtype string        `json:"subtype"`
	Parent  string        `json:"parent_tool_use_id"`
	Message streamMessage `json:"message"`
	Event   struct {
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
	tools       map[string]bool
	initialized bool
	current     string
	order       []string
	text        map[string]string
	complete    map[string]bool
	terminal    bool
	usage       *agent.TokenUsage
}

func (s *streamOutput) feed(line []byte, host external.Host) error {
	var f streamFrame
	if err := json.Unmarshal(line, &f); err != nil {
		return fmt.Errorf("decode Claude stream: %w", err)
	}
	if f.Parent != "" {
		return nil
	}
	if s.text == nil {
		s.text = map[string]string{}
		s.complete = map[string]bool{}
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
		if f.Message.ID == "" {
			return errors.New("Claude assistant message lacks ID")
		}
		for i, block := range f.Message.Content {
			if block.Type == "text" {
				if err := s.append(host, fmt.Sprintf("%s:%d", f.Message.ID, i), block.Text, true); err != nil {
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
	case "user", "rate_limit_event":
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
	return external.Result{Text: text.String(), Usage: s.usage}
}
