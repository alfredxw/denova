package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"denova/config"
	"denova/internal/agents/runtime/external"
	interaction "github.com/alfredxw/denova/agent/lifecycle/interaction"
)

func (c *Client) nativeConfig(ctx context.Context, input external.Input) (map[string]any, error) {
	settings := map[string]any{"features.default_mode_request_user_input": input.Interactive()}
	if input.Mode == external.OperationTurn && input.AgentKind != config.AgentKindInteractiveStory {
		return settings, nil
	}
	// Noninteractive product turns cannot approve ambient integrations. Disable
	// each configured server explicitly: an empty TOML table merges, not replaces.
	if c.cwd != "" {
		var response struct {
			Config struct {
				Servers map[string]json.RawMessage `json:"mcp_servers"`
			} `json:"config"`
		}
		if err := c.call(ctx, "config/read", map[string]any{"includeLayers": false}, &response); err != nil {
			return nil, err
		}
		servers := map[string]any{}
		for name := range response.Config.Servers {
			servers[name] = map[string]bool{"enabled": false}
		}
		settings["mcp_servers"] = servers
	}
	for _, feature := range []string{"apps", "plugins", "browser_use", "browser_use_external", "computer_use", "in_app_browser", "image_generation", "shell_tool", "unified_exec"} {
		settings["features."+feature] = false
	}
	if input.Mode != external.OperationTurn {
		settings["web_search"] = "disabled"
	}
	return settings, nil
}

func nativeItem(observer *external.NativeObserver, params json.RawMessage, completed bool) error {
	var event struct {
		Item struct {
			ID        string          `json:"id"`
			Type      string          `json:"type"`
			Status    string          `json:"status"`
			Tool      string          `json:"tool"`
			Server    string          `json:"server"`
			Command   string          `json:"command"`
			Cwd       string          `json:"cwd"`
			Arguments json.RawMessage `json:"arguments"`
			Result    json.RawMessage `json:"result"`
			Error     json.RawMessage `json:"error"`
			Output    string          `json:"aggregatedOutput"`
			ExitCode  *int            `json:"exitCode"`
			Query     string          `json:"query"`
			Action    json.RawMessage `json:"action"`
			Path      string          `json:"path"`
		} `json:"item"`
	}
	if err := json.Unmarshal(params, &event); err != nil {
		return err
	}
	item := event.Item
	name, args, result := item.Type, json.RawMessage(`{}`), ""
	success := item.Status != "failed" && item.Status != "declined"
	switch item.Type {
	case "commandExecution":
		args, _ = json.Marshal(map[string]string{"command": item.Command, "cwd": item.Cwd})
		result = item.Output
		if item.ExitCode != nil {
			success = *item.ExitCode == 0
			result += fmt.Sprintf("\nExit code: %d", *item.ExitCode)
		}
	case "mcpToolCall":
		name, args = item.Server+"/"+item.Tool, item.Arguments
		result = external.NativeResultText(item.Result)
		if len(item.Error) > 0 && string(item.Error) != "null" {
			result += "\n" + string(item.Error)
			success = false
		}
	case "webSearch":
		args, _ = json.Marshal(map[string]any{"query": item.Query, "action": item.Action})
		result = "Search completed. Search again after recovery if the cited sources are absent from the assistant response."
	case "imageView":
		args, _ = json.Marshal(map[string]string{"path": item.Path})
		result = "Image viewed by the runtime. Read the source again if visual context is needed after recovery."
	default:
		return nil
	}
	if err := observer.Start(item.ID, name, args); err != nil {
		return err
	}
	if completed {
		return observer.Finish(item.ID, result, success)
	}
	return nil
}

func nativeQuestion(ctx context.Context, host external.Host, input external.Input, id string, params json.RawMessage) (json.RawMessage, error) {
	var request struct {
		Questions []struct {
			ID       string                          `json:"id"`
			Question string                          `json:"question"`
			Secret   bool                            `json:"isSecret"`
			Options  []interaction.InteractionOption `json:"options"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(params, &request); err != nil {
		return nil, err
	}
	questions := make([]external.NativeQuestion, 0, len(request.Questions))
	for _, q := range request.Questions {
		if q.Secret {
			return nil, errors.New("secret input must use the runtime's account interface")
		}
		questions = append(questions, external.NativeQuestion{ID: q.ID, Prompt: q.Question, Options: q.Options})
	}
	answers, err := external.AskNative(ctx, host, input, id, questions)
	if err != nil {
		return nil, err
	}
	values := map[string]any{}
	for key, answer := range answers {
		values[key] = map[string]any{"answers": answer}
	}
	return json.Marshal(map[string]any{"answers": values})
}

func nativeApproval(ctx context.Context, host external.Host, input external.Input, id, method string, params json.RawMessage) (json.RawMessage, error) {
	// Shell/file escalation would bypass product review. Only a network-only
	// request can be approved while retaining the read-only filesystem sandbox.
	var request struct {
		Network json.RawMessage `json:"networkApprovalContext"`
	}
	if err := json.Unmarshal(params, &request); err != nil {
		return nil, err
	}
	if method == "item/permissions/requestApproval" {
		return json.RawMessage(`{"permissions":{},"scope":"turn"}`), nil
	}
	decision := "decline"
	if method == "item/commandExecution/requestApproval" && len(request.Network) > 0 && string(request.Network) != "null" {
		allowed, err := external.ApproveNative(ctx, host, input, id, string(params))
		if err != nil {
			return nil, err
		}
		if allowed {
			decision = "accept"
		}
	}
	return json.Marshal(map[string]string{"decision": decision})
}
