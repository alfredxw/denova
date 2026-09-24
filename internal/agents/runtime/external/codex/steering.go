package codex

import (
	"context"
	"errors"
	"strings"

	"denova/internal/agents/runtime/external"
	agent "github.com/alfredxw/denova/agent"
)

func userInput(input external.Input) ([]map[string]any, error) {
	content := []map[string]any{{"type": "text", "text": input.Text, "text_elements": []any{}}}
	for _, attachment := range input.Attachments {
		if !agent.IsNativeImageMediaType(attachment.MediaType) {
			continue
		}
		url, err := agent.AttachmentDataURL(attachment)
		if err != nil {
			return nil, err
		}
		content = append(content, map[string]any{"type": "image", "url": url})
	}
	return content, nil
}

func (c *Client) steer(ctx context.Context, threadID, turnID string, input external.Input) error {
	content, err := userInput(input)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, infrastructureTimeout)
	defer cancel()
	var accepted struct {
		TurnID string `json:"turnId"`
	}
	err = c.call(ctx, "turn/steer", map[string]any{"threadId": threadID, "expectedTurnId": turnID, "input": content}, &accepted)
	var rpc *rpcError
	if errors.As(err, &rpc) && (strings.Contains(strings.ToLower(rpc.Message), "no active turn") || strings.Contains(strings.ToLower(rpc.Message), "no active task")) {
		return external.ErrSteerUnavailable
	}
	if err == nil && accepted.TurnID != turnID {
		return errors.New("App Server accepted guidance for a different turn")
	}
	return err
}
