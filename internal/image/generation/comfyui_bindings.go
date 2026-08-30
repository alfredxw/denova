package generation

import (
	"encoding/json"
	"sort"
	"strings"

	"denova/config"
)

// ComfyUIInputCandidate is transient discovery metadata used to repair an
// ambiguous semantic binding. Candidates are never persisted with the profile.
type ComfyUIInputCandidate struct {
	NodeID    string `json:"node_id"`
	InputName string `json:"input_name"`
	Label     string `json:"label"`
}

// ComfyUIBindingCandidates contains only workflow inputs that can implement
// Denova's public image-generation contract. Static workflow values never
// leave the imported graph as editable settings.
type ComfyUIBindingCandidates struct {
	Prompt []ComfyUIInputCandidate `json:"prompt,omitempty"`
	Count  []ComfyUIInputCandidate `json:"count,omitempty"`
	Width  []ComfyUIInputCandidate `json:"width,omitempty"`
	Height []ComfyUIInputCandidate `json:"height,omitempty"`
}

func analyzeComfyUIBindings(workflow comfyUIWorkflow) (*config.ComfyUIBindings, ComfyUIBindingCandidates) {
	positive := make(map[string]config.ComfyUIInputBinding)
	geometry := map[string]map[string]config.ComfyUIInputBinding{
		"width":      {},
		"height":     {},
		"batch_size": {},
	}
	for nodeID, rawNode := range workflow {
		node, _ := rawNode.(map[string]any)
		classType := nodeString(node, "class_type")
		inputs, _ := node["inputs"].(map[string]any)
		if strings.Contains(classType, "KSampler") {
			collectComfyUIPromptTargets(workflow, inputs["positive"], positive)
		}
		for inputName := range geometry {
			if value, ok := inputs[inputName]; ok && !isComfyUINodeLink(value) {
				binding := config.ComfyUIInputBinding{NodeID: nodeID, InputName: inputName}
				geometry[inputName][comfyUIBindingKey(binding)] = binding
			}
		}
	}
	bindings := &config.ComfyUIBindings{
		Prompt: onlyComfyUIBinding(positive),
		Count:  onlyComfyUIBinding(geometry["batch_size"]),
		Width:  onlyComfyUIBinding(geometry["width"]),
		Height: onlyComfyUIBinding(geometry["height"]),
	}
	if bindings.Prompt == nil && bindings.Count == nil && bindings.Width == nil && bindings.Height == nil {
		bindings = nil
	}
	return bindings, ComfyUIBindingCandidates{
		Prompt: comfyUIInputCandidates(workflow, positive),
		Count:  comfyUIInputCandidates(workflow, geometry["batch_size"]),
		Width:  comfyUIInputCandidates(workflow, geometry["width"]),
		Height: comfyUIInputCandidates(workflow, geometry["height"]),
	}
}

func collectComfyUIPromptTargets(workflow comfyUIWorkflow, start any, targets map[string]config.ComfyUIInputBinding) {
	link, ok := start.([]any)
	if !ok || len(link) != 2 {
		return
	}
	nodeID, ok := link[0].(string)
	if !ok {
		return
	}
	queue := []string{nodeID}
	visited := make(map[string]struct{})
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := visited[current]; ok {
			continue
		}
		visited[current] = struct{}{}
		node, _ := workflow[current].(map[string]any)
		inputs, _ := node["inputs"].(map[string]any)
		for _, inputName := range []string{"text", "prompt", "positive_prompt"} {
			if value, ok := inputs[inputName]; ok && !isComfyUINodeLink(value) {
				if _, ok := value.(string); !ok {
					continue
				}
				binding := config.ComfyUIInputBinding{NodeID: current, InputName: inputName}
				targets[comfyUIBindingKey(binding)] = binding
			}
		}
		for _, value := range inputs {
			upstream, ok := value.([]any)
			if !ok || len(upstream) != 2 {
				continue
			}
			if upstreamID, ok := upstream[0].(string); ok {
				queue = append(queue, upstreamID)
			}
		}
	}
}

func comfyUIInputCandidates(workflow comfyUIWorkflow, bindings map[string]config.ComfyUIInputBinding) []ComfyUIInputCandidate {
	if len(bindings) == 0 {
		return nil
	}
	out := make([]ComfyUIInputCandidate, 0, len(bindings))
	for _, binding := range bindings {
		node, _ := workflow[binding.NodeID].(map[string]any)
		metadata, _ := node["_meta"].(map[string]any)
		title := nodeString(metadata, "title")
		if title == "" {
			title = nodeString(node, "class_type")
		}
		out = append(out, ComfyUIInputCandidate{
			NodeID:    binding.NodeID,
			InputName: binding.InputName,
			Label:     title,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Label) < strings.ToLower(out[j].Label)
	})
	return out
}

func onlyComfyUIBinding(bindings map[string]config.ComfyUIInputBinding) *config.ComfyUIInputBinding {
	if len(bindings) != 1 {
		return nil
	}
	for _, binding := range bindings {
		return &config.ComfyUIInputBinding{NodeID: binding.NodeID, InputName: binding.InputName}
	}
	return nil
}

func isComfyUINodeLink(value any) bool {
	link, ok := value.([]any)
	if !ok || len(link) != 2 {
		return false
	}
	if _, ok := link[0].(string); !ok {
		return false
	}
	switch link[1].(type) {
	case json.Number, float64, int, int64:
		return true
	default:
		return false
	}
}

func comfyUIBindingKey(binding config.ComfyUIInputBinding) string {
	return binding.NodeID + "\x00" + binding.InputName
}
