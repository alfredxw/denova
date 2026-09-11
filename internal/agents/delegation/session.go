package delegation

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

const (
	ParentSessionAttribute = agent.ParentSessionAttribute
	ChildAgentAttribute    = "agent"
)

// ParentAttributes freezes the exact public parent Session identity in a
// child key. The encoded canonical key remains privacy-safe provider-side:
// Denova's cache-key adapter hashes the public identity before model use.
func ParentAttributes(parent agent.SessionKey) (map[string]string, error) {
	return agent.ChildSessionAttributes(parent)
}

// RunBinding is the immutable parent ownership of one child Run. It is
// accepted with Input.HostData; reusing a child Session never rewrites it.
type RunBinding struct {
	ParentRunID string          `json:"parent_run_id"`
	ParentCycle int             `json:"parent_cycle"`
	HostData    *agent.HostData `json:"host_data"`
}

func BindRun(parent agent.RunView, route *agent.HostData) (*agent.HostData, error) {
	if parent.ID == "" || parent.Cycle < 1 || route == nil {
		return nil, errors.New("delegated Run requires current parent Run ownership")
	}
	encoded, err := json.Marshal(RunBinding{ParentRunID: parent.ID, ParentCycle: parent.Cycle, HostData: route})
	if err != nil {
		return nil, err
	}
	return &agent.HostData{Type: "denova.delegated_run", Version: 1, Data: encoded}, nil
}

func DecodeRunBinding(data *agent.HostData) (RunBinding, error) {
	var binding RunBinding
	if data == nil || data.Type != "denova.delegated_run" || data.Version != 1 {
		return binding, errors.New("delegated Run ownership is unavailable")
	}
	if err := json.Unmarshal(data.Data, &binding); err != nil {
		return binding, err
	}
	if binding.ParentRunID == "" || binding.ParentCycle < 1 || binding.HostData == nil || !json.Valid(binding.HostData.Data) {
		return binding, errors.New("delegated Run ownership is invalid")
	}
	return binding, nil
}

// ParentSession decodes the exact parent key from a delegated child Session.
func ParentSession(child agent.SessionKey) (agent.SessionKey, error) {
	if !strings.HasPrefix(child.Namespace, "task.") {
		return agent.SessionKey{}, errors.New("Session is not a delegated task")
	}
	parent, err := agent.ParentSessionKey(child)
	if err != nil {
		return agent.SessionKey{}, fmt.Errorf("decode delegated parent Session: %w", err)
	}
	return parent, nil
}

func ChildName(child agent.SessionKey) (string, error) {
	name := strings.TrimSpace(child.Attributes[ChildAgentAttribute])
	if name == "" || child.Namespace != "task."+name {
		return "", errors.New("delegated task Agent identity is invalid")
	}
	return name, nil
}
