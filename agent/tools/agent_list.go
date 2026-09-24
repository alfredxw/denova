package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"

	agent "github.com/alfredxw/denova/agent"
)

type ListAgentsInput struct {
	Kind   string `json:"kind,omitempty" jsonschema:"enum=instances,enum=definitions" jsonschema_description:"Default instances: existing children in the caller's delegation scope. definitions lists types available for delegation."`
	Cursor string `json:"cursor,omitempty" jsonschema:"maxLength=8192" jsonschema_description:"Opaque next_cursor from the same kind and caller scope."`
	Limit  int    `json:"limit,omitempty" jsonschema:"minimum=1,maximum=100" jsonschema_description:"Default 20, maximum 100."`
}
type AgentSummary struct {
	Ref            TaskRef           `json:"ref"`
	ActiveRun      *agentRunSnapshot `json:"active_run,omitempty"`
	LastSettledRun *agentRunSnapshot `json:"last_settled_run,omitempty"`
	QueuedCount    int               `json:"queued_count"`
}
type AgentDefinition struct {
	Agent                string `json:"agent"`
	Description          string `json:"description"`
	DescriptionTruncated bool   `json:"description_truncated"`
}
type ListAgentsOutput struct {
	Kind        string            `json:"kind"`
	Self        TaskRef           `json:"self,omitempty"`
	Agents      []AgentSummary    `json:"agents,omitempty"`
	Definitions []AgentDefinition `json:"definitions,omitempty"`
	NextCursor  string            `json:"next_cursor,omitempty"`
}

func (output ListAgentsOutput) MarshalJSON() ([]byte, error) {
	if output.Kind == "definitions" {
		return json.Marshal(struct {
			Kind        string            `json:"kind"`
			Definitions []AgentDefinition `json:"definitions"`
			NextCursor  string            `json:"next_cursor,omitempty"`
		}{output.Kind, output.Definitions, output.NextCursor})
	}
	return json.Marshal(struct {
		Kind       string         `json:"kind"`
		Self       TaskRef        `json:"self"`
		Agents     []AgentSummary `json:"agents"`
		NextCursor string         `json:"next_cursor,omitempty"`
	}{output.Kind, output.Self, output.Agents, output.NextCursor})
}

func newListAgentsDefinition(executor TaskExecutor) (agent.ToolDefinition, error) {
	tool, err := agent.InferTool("list_agents", "Discover existing child Agent instances or available delegation definitions in this caller's authorized scope. Listing never starts or resumes work. Instances expose active and latest settled Run refs plus queue counts, not transcripts. Follow next_cursor with the same kind to continue.",
		func(ctx context.Context, input ListAgentsInput) (agent.ToolResult, error) {
			output, err := executor.ListAgents(ctx, input)
			if err != nil {
				return agentFailure(err)
			}
			return JSONResult(output)
		})
	if err != nil {
		return agent.ToolDefinition{}, err
	}
	descriptor := readDescriptor()
	descriptor.Source, descriptor.Capability = agent.ToolSourceOther, "delegation"
	descriptor.ResultRecoveryKind = agent.ToolResultRecoveryRerun
	descriptor.Presentation = agent.UniformToolPresentation(agent.ToolPresentationDelegation)
	return agent.ToolDefinition{Tool: tool, Descriptor: descriptor}, nil
}

type agentListCursor struct{ Scope, Kind, After string }

func (tasks *LocalTasks) ListAgents(ctx context.Context, input ListAgentsInput) (ListAgentsOutput, error) {
	if input.Kind == "" {
		input.Kind = "instances"
	}
	if input.Limit == 0 {
		input.Limit = 20
	}
	if input.Kind != "instances" && input.Kind != "definitions" || input.Limit < 1 || input.Limit > 100 {
		return ListAgentsOutput{}, fmt.Errorf("%w: invalid list kind or limit", ErrTaskInvalidInput)
	}
	if input.Kind == "instances" && validateTaskSessionRef(tasks.self) != nil {
		return ListAgentsOutput{}, fmt.Errorf("%w: instance discovery requires the caller identity", agent.ErrCapabilityUnsupported)
	}
	selectors := make([]agent.SessionSelector, 0, len(tasks.ordered))
	for _, candidate := range tasks.ordered {
		selectors = append(selectors, taskSessionSelector(tasks.agents[candidate.Name], ""))
	}
	scope := toolsetIdentity("agent.list", struct {
		Self      TaskRef
		Selectors []agent.SessionSelector
	}{tasks.self, selectors}).ConfigHash
	position := agentListCursor{Scope: scope, Kind: input.Kind}
	if input.Cursor != "" {
		encoded, err := base64.RawURLEncoding.DecodeString(input.Cursor)
		if err != nil || json.Unmarshal(encoded, &position) != nil || position.Scope != scope || position.Kind != input.Kind {
			return ListAgentsOutput{}, fmt.Errorf("%w: list cursor belongs to another scope or kind", ErrTaskInvalidInput)
		}
	}
	output := ListAgentsOutput{Kind: input.Kind, Self: tasks.self, Agents: []AgentSummary{}, Definitions: []AgentDefinition{}}
	var names []string
	keys := make(map[string]agent.SessionKey)
	candidates := make(map[string]LocalTaskAgent)
	for _, info := range tasks.ordered {
		if input.Kind == "definitions" {
			names = append(names, info.Name)
			continue
		}
		candidate := tasks.agents[info.Name]
		sessions, err := taskSessionKeys(ctx, candidate, "")
		if err != nil {
			return ListAgentsOutput{}, err
		}
		for _, key := range sessions {
			name := info.Name + "\x00" + key.ID
			if _, duplicate := keys[name]; duplicate {
				return ListAgentsOutput{}, fmt.Errorf("ambiguous child Session %q", key.ID)
			}
			names, keys[name], candidates[name] = append(names, name), key, candidate
		}
	}
	sort.Strings(names)
	start := sort.Search(len(names), func(index int) bool { return names[index] > position.After })
	end := min(start+input.Limit, len(names))
	for offset, name := range names[start:end] {
		if input.Kind == "definitions" {
			description, truncated := boundAgentText(tasks.agents[name].Description, 2048)
			output.Definitions = append(output.Definitions, AgentDefinition{Agent: name, Description: description, DescriptionTruncated: truncated})
			encoded, _ := json.Marshal(output)
			if len(encoded) > tasks.maxResultBytes-2048 {
				output.Definitions = output.Definitions[:len(output.Definitions)-1]
				end = start + offset
				break
			}
			continue
		}
		candidate := candidates[name]
		reader, ok := candidate.Opener.(interface {
			InspectSession(context.Context, agent.SessionKey) (agent.SessionSnapshot, error)
		})
		if !ok {
			return ListAgentsOutput{}, agent.ErrCapabilityUnsupported
		}
		snapshot, err := reader.InspectSession(ctx, keys[name])
		if err != nil {
			return ListAgentsOutput{}, err
		}
		ref := TaskRef{Agent: candidate.Name, Session: keys[name].ID}
		summary := AgentSummary{Ref: ref}
		for _, queued := range snapshot.QueuedRuns {
			if queued.Delivery == agent.DeliveryNextTurn {
				summary.QueuedCount++
			}
		}
		if snapshot.ActiveRunID != "" {
			runRef := ref
			runRef.Run = snapshot.ActiveRunID
			task, err := taskFromSnapshot(runRef, snapshot)
			if err != nil {
				return ListAgentsOutput{}, err
			}
			summary.ActiveRun = &agentRunSnapshot{Ref: runRef, Status: task.Status, Reason: task.Reason}
		}
		if len(snapshot.RecentRuns) > 0 {
			run := snapshot.RecentRuns[len(snapshot.RecentRuns)-1]
			runRef := ref
			runRef.Run = run.ID
			summary.LastSettledRun = &agentRunSnapshot{Ref: runRef, Status: string(run.Status), Reason: run.Reason}
		}
		output.Agents = append(output.Agents, summary)
		encoded, _ := json.Marshal(output)
		if len(encoded) > tasks.maxResultBytes-2048 {
			output.Agents = output.Agents[:len(output.Agents)-1]
			end = start + offset
			break
		}
	}
	if end == start && end < len(names) {
		return ListAgentsOutput{}, ErrAgentResultTooLarge
	}
	if end < len(names) {
		position.After = names[end-1]
		encoded, _ := json.Marshal(position)
		output.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	}
	return output, nil
}
