package external

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	agentrun "denova/internal/agents/run"
	externaljournal "denova/internal/agents/runtime/external/journal"
)

// NativeResultText excludes embedded image bytes from the portable transcript.
// The engine receives the original pixels; recovery is told to capture them again.
func NativeResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return string(raw)
	}
	var clean func(any) any
	clean = func(value any) any {
		switch v := value.(type) {
		case map[string]any:
			if v["type"] == "image" || v["type"] == "input_image" || v["type"] == "image_url" {
				return map[string]string{"type": "text", "text": "[Image observed by the runtime. Capture again if visual context is needed after recovery.]"}
			}
			for key, item := range v {
				v[key] = clean(item)
			}
		case []any:
			for i, item := range v {
				v[i] = clean(item)
			}
		}
		return value
	}
	if text, ok := value.(string); ok {
		return text
	}
	body, _ := json.Marshal(clean(value))
	return string(body)
}

// NativeTool is an observation, never a request to execute a product tool.
// Adapters report public input/output only; private reasoning stays in the engine.
type NativeTool struct {
	ID        string
	Name      string
	Arguments json.RawMessage
	Result    string
	Status    string
}

const nativeResultUnknown = "Native execution ended without a confirmed result. Its external effects are unknown; inspect the current state before retrying."

func (input Input) NativeInstructions() string {
	return input.Instructions + "\n\nUse native tools for general research, file inspection and computation. The working directory is disposable scratch space. Product workspace: " + input.Workspace + ". Use the supplied product tools for managed content edits, configuration changes and story-state submissions; these tools preserve review and transaction semantics. The native filesystem sandbox does not restrict authorized product tool callbacks. Do not bypass product mutations through shell commands, integrations or UI automation. Use the product read tool for virtual resource URIs and portable tool artifacts."
}

// NativeObserver keeps provider tool IDs local to one attempt. Its events use
// the same product journal and tool cards as host tools, without invoking them.
type NativeObserver struct {
	Input    Input
	Host     Host
	calls    map[string]NativeTool
	finished map[string]bool
}

func (observer *NativeObserver) Start(id, name string, arguments json.RawMessage) error {
	if id == "" || name == "" || !json.Valid(arguments) {
		return errors.New("native tool observation lacks identity or arguments")
	}
	if observer.calls == nil {
		observer.calls = map[string]NativeTool{}
		observer.finished = map[string]bool{}
	}
	if _, exists := observer.calls[id]; exists {
		return nil
	}
	args := observer.Input.PortableText(string(arguments))
	if len(args) > nativeObservationLimit {
		body, _ := json.Marshal(map[string]string{"preview": boundedNativeText(args)})
		args = string(body)
	}
	call := NativeTool{ID: id, Name: "native/" + name, Arguments: json.RawMessage(args), Status: "running"}
	if err := observer.Host.Emit(agentrun.Event{Type: "native_tool", Data: call}); err != nil {
		return err
	}
	observer.calls[id] = call
	return nil
}

func (observer *NativeObserver) Finish(id, result string, success bool) error {
	call, exists := observer.calls[id]
	if !exists || observer.finished[id] {
		return nil
	}
	call.Result = boundedNativeText(observer.Input.PortableText(result))
	call.Status = "success"
	if !success {
		call.Status = "error"
	}
	if err := observer.Host.Emit(agentrun.Event{Type: "native_tool", Data: call}); err != nil {
		return err
	}
	observer.finished[id] = true
	return nil
}

// Close records uncertainty, not failure-without-effects. A native tool is never
// replayed by the product; the next model turn receives this observation.
func (observer *NativeObserver) Close() error {
	var err error
	for id := range observer.calls {
		if !observer.finished[id] {
			err = errors.Join(err, observer.Finish(id, nativeResultUnknown, false))
		}
	}
	return err
}

// PortableText removes managed host roots from persisted observations. The
// replacement is descriptive context, never a persisted recovery locator.
func (input Input) PortableText(value string) string {
	for _, root := range []struct{ path, label string }{{input.Workspace, "<workspace>"}, {input.StoreRoot, "<project-store>"}, {input.DataRoot, "<managed-data>"}, {input.Directory, "<runtime-scratch>"}} {
		if root.path == "" {
			continue
		}
		for _, path := range []string{root.path, filepath.ToSlash(root.path)} {
			encoded, _ := json.Marshal(path)
			value = strings.ReplaceAll(value, string(encoded[1:len(encoded)-1]), root.label)
			value = strings.ReplaceAll(value, path, root.label)
		}
	}
	return value
}

// 64 KiB per native observation covers typical command/page results. Larger
// outputs stay in the disposable engine transcript; recovery explicitly knows
// to read them again instead of treating a silently truncated result as complete.
const nativeObservationLimit = 64 << 10

func boundedNativeText(value string) string {
	if len(value) <= nativeObservationLimit {
		return value
	}
	end := nativeObservationLimit
	for !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + "\n[Observation truncated. Read the source again if more detail is needed.]"
}

func (tool NativeTool) ExecutionID(operationID string) string {
	digest := sha256.Sum256([]byte(tool.ID))
	return operationID + "-native-" + hex.EncodeToString(digest[:])
}

func (tool NativeTool) DisplayEvent(operationID string) agentrun.Event {
	data := map[string]any{"id": tool.ExecutionID(operationID), "name": tool.Name}
	if tool.Status == "running" {
		data["args"] = string(tool.Arguments)
		return agentrun.Event{Type: "tool_call", Data: data}
	}
	data["content"], data["status"] = tool.Result, tool.Status
	return agentrun.Event{Type: "tool_result", Data: data}
}

func (operation *Operation) observeNative(event agentrun.Event) error {
	tool, ok := event.Data.(NativeTool)
	if !ok {
		return fmt.Errorf("invalid native tool observation")
	}
	id := tool.ExecutionID(operation.id)
	var err error
	if tool.Status == "running" {
		err = operation.transition(context.WithoutCancel(operation.runContext), externaljournal.ToolStarted, externaljournal.StartedTool{ExecutionID: id, AgentKind: operation.request.ToolPolicy.AgentKind, Tool: tool.Name, Arguments: tool.Arguments, Recovery: externaljournal.NonReplayable})
	} else {
		err = operation.transition(context.WithoutCancel(operation.runContext), externaljournal.ToolFinished, externaljournal.FinishedTool{ExecutionID: id, Success: tool.Status == "success", Result: tool.Result})
	}
	if err != nil {
		return err
	}
	operation.send(tool.DisplayEvent(operation.id))
	return nil
}
