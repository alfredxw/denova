package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"nova/internal/session"
)

// appendAssistantIfAny 将已生成的正文持久化，避免异常中断后刷新丢失输出。
func appendAssistantIfAny(conversation Conversation, content, thinking *strings.Builder) string {
	if content == nil || content.Len() == 0 {
		return ""
	}
	generated := content.String()
	reasoning := ""
	if thinking != nil && thinking.Len() > 0 {
		reasoning = thinking.String()
	}
	if appender, ok := conversation.(interface {
		AppendAssistantWithThinking(content, thinking string) error
	}); ok {
		if err := appender.AppendAssistantWithThinking(generated, reasoning); err != nil {
			log.Printf("[agent-run] persist assistant message failed err=%v", err)
		}
	} else if err := conversation.AppendAssistant(generated); err != nil {
		log.Printf("[agent-run] persist assistant message failed err=%v", err)
	}
	log.Printf("[agent-run] persisted assistant message bytes=%d thinking_bytes=%d", len(generated), len(reasoning))
	content.Reset()
	if thinking != nil {
		thinking.Reset()
	}
	return generated
}

type displayEventAppender interface {
	AppendDisplayEvent(event session.DisplayEvent) error
	UpdateDisplayToolStatus(id, name, status string) error
}

type displayToolArgsAppender interface {
	AppendDisplayToolArgs(id, name, delta string) error
}

type displayToolResultUpdater interface {
	UpdateDisplayToolResult(id, name, status, result string) error
}

type displayEventRecorder struct {
	appender       displayEventAppender
	thinking       strings.Builder
	pendingToolIDs map[string]string
}

func newDisplayEventRecorder(conversation Conversation) *displayEventRecorder {
	appender, _ := conversation.(displayEventAppender)
	return &displayEventRecorder{
		appender:       appender,
		pendingToolIDs: make(map[string]string),
	}
}

func (r *displayEventRecorder) Record(ev Event) {
	if r == nil || r.appender == nil {
		return
	}
	switch ev.Type {
	case "thinking":
		r.thinking.WriteString(eventDataString(ev.Data, "content"))
	case "chunk":
		r.flushThinking()
	case "tool_call":
		r.flushThinking()
		id := eventDataString(ev.Data, "id")
		name := eventDataString(ev.Data, "name")
		args := eventDataString(ev.Data, "args")
		if strings.TrimSpace(name) == "" {
			name = "unknown_tool"
		}
		if err := r.appender.AppendDisplayEvent(session.DisplayEvent{
			ID:      id,
			Role:    "tool_call",
			Content: name,
			Name:    name,
			Args:    args,
			Status:  "running",
		}); err != nil {
			log.Printf("[agent-run] persist display tool_call failed name=%s id=%s err=%v", name, id, err)
			return
		}
		if id != "" {
			r.pendingToolIDs[id] = name
		}
	case "tool_args_delta":
		id := eventDataString(ev.Data, "id")
		name := eventDataString(ev.Data, "name")
		delta := eventDataString(ev.Data, "delta")
		argsAppender, ok := r.appender.(displayToolArgsAppender)
		if !ok {
			return
		}
		if err := argsAppender.AppendDisplayToolArgs(id, name, delta); err != nil {
			log.Printf("[agent-run] persist display tool_args_delta failed name=%s id=%s err=%v", name, id, err)
		}
	case "tool_result":
		r.flushThinking()
		id := eventDataString(ev.Data, "id")
		name := eventDataString(ev.Data, "name")
		result := eventDataString(ev.Data, "content")
		if resultUpdater, ok := r.appender.(displayToolResultUpdater); ok {
			if err := resultUpdater.UpdateDisplayToolResult(id, name, "success", result); err != nil {
				log.Printf("[agent-run] persist display tool_result failed name=%s id=%s err=%v", name, id, err)
			}
		} else if err := r.appender.UpdateDisplayToolStatus(id, name, "success"); err != nil {
			log.Printf("[agent-run] persist display tool_result status failed name=%s id=%s err=%v", name, id, err)
		}
		if id != "" {
			delete(r.pendingToolIDs, id)
		}
	case "error", "aborted":
		r.flushThinking()
		for id, name := range r.pendingToolIDs {
			if err := r.appender.UpdateDisplayToolStatus(id, name, "error"); err != nil {
				log.Printf("[agent-run] persist display tool_error failed name=%s id=%s err=%v", name, id, err)
			}
		}
		r.pendingToolIDs = make(map[string]string)
	case "done":
		r.flushThinking()
	}
}

func (r *displayEventRecorder) flushThinking() {
	if r == nil || r.appender == nil || r.thinking.Len() == 0 {
		return
	}
	content := r.thinking.String()
	r.thinking.Reset()
	if strings.TrimSpace(content) == "" {
		return
	}
	if err := r.appender.AppendDisplayEvent(session.DisplayEvent{
		Role:    "thinking",
		Content: content,
	}); err != nil {
		log.Printf("[agent-run] persist display thinking failed bytes=%d err=%v", len(content), err)
	}
}

func eventDataString(data interface{}, key string) string {
	switch typed := data.(type) {
	case map[string]string:
		return typed[key]
	case map[string]interface{}:
		if value, ok := typed[key]; ok {
			return fmt.Sprint(value)
		}
	}
	return ""
}

func parseWriteLoreItemsToolResult(toolName, content string) ([]string, []string) {
	if toolName != "write_lore_items" {
		return nil, nil
	}
	var itemIDs []string
	var deletedIDs []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if raw, ok := strings.CutPrefix(line, "item_ids:"); ok {
			_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &itemIDs)
			continue
		}
		if raw, ok := strings.CutPrefix(line, "deleted_ids:"); ok {
			_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &deletedIDs)
		}
	}
	return itemIDs, deletedIDs
}
