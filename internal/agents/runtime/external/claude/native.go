package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"slices"
	"strings"

	"denova/config"
	"denova/internal/agents/runtime/external"
	interaction "github.com/alfredxw/denova/agent/lifecycle/interaction"
)

func nativeArguments(input external.Input) ([]string, []string) {
	if input.Mode != external.OperationTurn {
		return streamArguments(""), nil
	}
	names := []string{"Read", "Glob", "Grep", "WebFetch", "WebSearch", "TaskCreate", "TaskGet", "TaskList", "TaskUpdate"}
	settings := map[string]any{"disableAllHooks": true, "autoMemoryEnabled": false}
	if input.AgentKind != config.AgentKindInteractiveStory {
		names = append(names, "AskUserQuestion")
		// Native Bash is admitted only with an enforced OS sandbox. Native
		// Windows has no supported sandbox; WSL uses the Linux build/launcher.
		if runtime.GOOS != "windows" {
			names = append(names, "Bash")
			protected := []string{}
			for _, path := range []string{input.Workspace, input.StoreRoot, input.DataRoot} {
				if path != "" {
					protected = append(protected, path)
				}
			}
			settings["sandbox"] = map[string]any{"enabled": true, "failIfUnavailable": true, "autoAllowBashIfSandboxed": true, "allowUnsandboxedCommands": false, "filesystem": map[string]any{"denyWrite": protected}}
		}
	}
	body, _ := json.Marshal(settings)
	args := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--tools", strings.Join(names, ","), "--settings", string(body), "--setting-sources", "", "--permission-mode", "default", "--permission-prompt-tool", "stdio"}
	if input.AgentKind == config.AgentKindInteractiveStory {
		args = append(args, "--strict-mcp-config")
	}
	return args, names
}

func (run *streamRun) control(ctx context.Context, frame streamFrame) ([]byte, error) {
	var request struct {
		Subtype string          `json:"subtype"`
		Tool    string          `json:"tool_name"`
		Input   json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(frame.Request, &request); err != nil {
		return nil, err
	}
	response := map[string]any{"behavior": "deny", "message": "This action is unavailable. Use product tools for managed content changes."}
	switch request.Subtype {
	case "can_use_tool":
		if request.Tool == "AskUserQuestion" {
			var args struct {
				Questions []struct {
					Question string                          `json:"question"`
					Options  []interaction.InteractionOption `json:"options"`
					Multiple bool                            `json:"multiSelect"`
				} `json:"questions"`
			}
			if err := json.Unmarshal(request.Input, &args); err != nil {
				return nil, err
			}
			questions := make([]external.NativeQuestion, 0, len(args.Questions))
			for _, q := range args.Questions {
				questions = append(questions, external.NativeQuestion{ID: q.Question, Prompt: q.Question, Options: q.Options, Multiple: q.Multiple})
			}
			answers, err := external.AskNative(ctx, run.host, run.input, frame.RequestID, questions)
			if err != nil {
				return nil, err
			}
			if answers != nil {
				var updated map[string]any
				if err := json.Unmarshal(request.Input, &updated); err != nil {
					return nil, err
				}
				values := map[string]string{}
				for key, value := range answers {
					values[key] = strings.Join(value, ", ")
				}
				updated["answers"] = values
				response = map[string]any{"behavior": "allow", "updatedInput": updated}
			}
		} else if run.output.tools[request.Tool] || (run.output.ambientMCP && strings.HasPrefix(request.Tool, "mcp__") && !strings.HasPrefix(request.Tool, "mcp__denova__")) {
			var args struct {
				Unsandboxed bool `json:"dangerouslyDisableSandbox"`
			}
			_ = json.Unmarshal(request.Input, &args)
			if !args.Unsandboxed {
				allowed, err := external.ApproveNative(ctx, run.host, run.input, frame.RequestID, request.Tool+"\n"+string(request.Input))
				if err != nil {
					return nil, err
				}
				if allowed {
					response = map[string]any{"behavior": "allow", "updatedInput": request.Input}
				}
			}
		}
	case "elicitation":
		// Claude's control protocol uses snake_case for the schema, unlike MCP
		// and App Server's camelCase envelope.
		var elicitation struct {
			external.NativeElicitation
			Schema json.RawMessage `json:"requested_schema"`
		}
		if err := json.Unmarshal(frame.Request, &elicitation); err != nil {
			return nil, err
		}
		elicitation.NativeElicitation.Schema = elicitation.Schema
		body, err := external.ElicitNative(ctx, run.host, run.input, frame.RequestID, elicitation.NativeElicitation)
		if err != nil {
			return nil, err
		}
		response = nil
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported native control request %q", request.Subtype)
	}
	body, err := json.Marshal(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": frame.RequestID, "response": response}})
	return append(body, '\n'), err
}

func (s *streamOutput) observeNative(f streamFrame) error {
	if s.native == nil || f.IsReplay || f.IsMeta {
		return nil
	}
	for _, block := range f.Message.Content {
		if block.Type == "tool_use" && !strings.HasPrefix(block.Name, "mcp__denova__") && !slices.Contains([]string{"TaskCreate", "TaskGet", "TaskList", "TaskUpdate", "AskUserQuestion"}, block.Name) {
			if err := s.native.Start(block.ID, block.Name, block.Input); err != nil {
				return err
			}
		}
		if block.Type == "tool_result" {
			if err := s.native.Finish(block.ToolUseID, external.NativeResultText(block.Content), !block.IsError); err != nil {
				return err
			}
		}
	}
	return nil
}
