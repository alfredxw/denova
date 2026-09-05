package agentrun

import (
	"encoding/json"

	publictools "github.com/alfredxw/denova/agent/tools"
)

// RunTraceReference is a diagnostic edge between independently owned Runs.
// These stable IDs never participate in canonical Session recovery.
type RunTraceReference struct {
	ID           string `json:"id"`
	SessionID    string `json:"session_id"`
	AgentName    string `json:"agent_name"`
	ParentCallID string `json:"parent_call_id,omitempty"`
}

// TaskRunTraceReferences reads successful task.start items independently, so a
// partial batch failure cannot hide the children that were actually accepted.
func TaskRunTraceReferences(result string) []RunTraceReference {
	var response struct {
		Results []struct {
			Task *publictools.Task `json:"task"`
		} `json:"results"`
	}
	if json.Unmarshal([]byte(result), &response) != nil {
		return nil
	}
	var references []RunTraceReference
	for _, item := range response.Results {
		if item.Task != nil && item.Task.Ref.Run != "" && item.Task.Ref.Session != "" {
			ref := item.Task.Ref
			references = append(references, RunTraceReference{ID: ref.Run, SessionID: ref.Session, AgentName: ref.Agent})
		}
	}
	return references
}

func appendChildRunReference(summary *RunTraceSummary, child RunTraceReference) {
	if child.ID == "" || child.ID == summary.ID {
		return
	}
	for index, existing := range summary.ChildRuns {
		if existing.ID == child.ID {
			if existing.ParentCallID == "" {
				summary.ChildRuns[index].ParentCallID = child.ParentCallID
			}
			return
		}
	}
	summary.ChildRuns = append(summary.ChildRuns, child)
}
