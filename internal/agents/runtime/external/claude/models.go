package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"denova/internal/agents/runtime/external"
)

// Models asks the CLI's native initialization protocol. Discovery does not
// send a user prompt, start a model turn, or import user hooks and project tools.
func (c *Client) Models(ctx context.Context) (external.Models, error) {
	ctx, cancel := context.WithTimeout(ctx, infrastructureTimeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "denova-claude-models-")
	if err != nil {
		return external.Models{}, err
	}
	defer os.RemoveAll(dir)
	args := streamArguments("")
	args = append(args, "--mcp-config", `{"mcpServers":{}}`, "--no-session-persistence")
	cmd := c.command(ctx, args...)
	cmd.Dir, cmd.Env = dir, runEnvironment(cmd.Env)
	input, err := cmd.StdinPipe()
	if err != nil {
		return external.Models{}, err
	}
	defer input.Close()
	output, err := cmd.StdoutPipe()
	if err != nil {
		return external.Models{}, err
	}
	defer output.Close()
	if err := cmd.Start(); err != nil {
		return external.Models{}, err
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	if err := json.NewEncoder(input).Encode(map[string]any{
		"type": "control_request", "request_id": "models", "request": map[string]any{"subtype": "initialize", "hooks": map[string]any{}},
	}); err != nil {
		return external.Models{}, err
	}
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		var frame struct {
			Type     string `json:"type"`
			Response struct {
				RequestID string          `json:"request_id"`
				Subtype   string          `json:"subtype"`
				Response  json.RawMessage `json:"response"`
			} `json:"response"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			return external.Models{}, err
		}
		if frame.Type != "control_response" || frame.Response.RequestID != "models" {
			continue
		}
		if frame.Response.Subtype != "success" {
			return external.Models{}, errors.New("Claude model discovery was rejected")
		}
		return decodeModels(frame.Response.Response)
	}
	return external.Models{}, fmt.Errorf("Claude omitted its model catalog: %w", errors.Join(scanner.Err(), ctx.Err(), errors.New("initialization did not complete")))
}

func decodeModels(raw json.RawMessage) (external.Models, error) {
	var initialization struct {
		Models []struct {
			Value         string   `json:"value"`
			DisplayName   string   `json:"displayName"`
			Efforts       []string `json:"supportedEffortLevels"`
			DefaultEffort string   `json:"defaultEffort"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &initialization); err != nil {
		return external.Models{}, err
	}
	result := external.Models{Items: []external.Model{}}
	for _, model := range initialization.Models {
		if model.Value == "" || model.DisplayName == "" {
			return external.Models{}, errors.New("Claude returned an invalid model identity")
		}
		result.Items = append(result.Items, external.Model{ID: model.Value, DisplayName: model.DisplayName,
			Efforts: append([]string{}, model.Efforts...), DefaultEffort: model.DefaultEffort})
	}
	if len(result.Items) == 0 {
		return result, errors.New("Claude returned an empty model catalog")
	}
	// The CLI puts its recommended/default choice first. Preserve its identity
	// and ordering; aliases and effort levels are owned by the installed runtime.
	result.DefaultID = result.Items[0].ID
	return result, nil
}
