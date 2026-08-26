package generation

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"denova/config"
)

type comfyUINodeInfo struct {
	Input struct {
		Required map[string]json.RawMessage `json:"required"`
		Optional map[string]json.RawMessage `json:"optional"`
	} `json:"input"`
}

type comfyUIInputOptions struct {
	Min       *float64 `json:"min"`
	Max       *float64 `json:"max"`
	Step      *float64 `json:"step"`
	Multiline bool     `json:"multiline"`
}

func analyzeComfyUIParameters(workflow comfyUIWorkflow, nodeInfo map[string]comfyUINodeInfo) ([]config.ComfyUIParameterSettings, error) {
	roles := inferComfyUIParameterRoles(workflow)
	nodeIDs := make([]string, 0, len(workflow))
	for nodeID := range workflow {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Strings(nodeIDs)
	parameters := make([]config.ComfyUIParameterSettings, 0)
	for _, nodeID := range nodeIDs {
		node, _ := workflow[nodeID].(map[string]any)
		classType := nodeString(node, "class_type")
		inputs, _ := node["inputs"].(map[string]any)
		metadata, _ := node["_meta"].(map[string]any)
		title := nodeString(metadata, "title")
		if title == "" {
			title = classType
		}
		for inputName, value := range inputs {
			if inputName == "filename_prefix" || isComfyUINodeLink(value) {
				continue
			}
			parameter, ok := comfyUIParameterFromDefinition(nodeInfo[classType], inputName)
			if !ok {
				continue
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("encode ComfyUI parameter %s.%s: %w", nodeID, inputName, err)
			}
			parameter.NodeID = nodeID
			parameter.InputName = inputName
			parameter.Label = title + " · " + inputName
			parameter.Role = roles[comfyUIParameterKey(nodeID, inputName)]
			if parameter.Role == "" {
				parameter.Role = config.ComfyUIParameterRoleParameter
			}
			parameter.Value = string(encoded)
			parameters = append(parameters, parameter)
		}
	}
	sort.SliceStable(parameters, func(i, j int) bool {
		leftRuntime := parameters[i].Role != config.ComfyUIParameterRoleParameter
		rightRuntime := parameters[j].Role != config.ComfyUIParameterRoleParameter
		if leftRuntime != rightRuntime {
			return leftRuntime
		}
		return strings.ToLower(parameters[i].Label) < strings.ToLower(parameters[j].Label)
	})
	return parameters, nil
}

func comfyUIParameterFromDefinition(info comfyUINodeInfo, inputName string) (config.ComfyUIParameterSettings, bool) {
	raw, ok := info.Input.Required[inputName]
	if !ok {
		raw, ok = info.Input.Optional[inputName]
	}
	if !ok {
		return config.ComfyUIParameterSettings{}, false
	}
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) != nil || len(parts) == 0 {
		return config.ComfyUIParameterSettings{}, false
	}
	parameter := config.ComfyUIParameterSettings{}
	var valueType string
	if json.Unmarshal(parts[0], &valueType) == nil {
		parameter.Type = strings.ToUpper(strings.TrimSpace(valueType))
	} else {
		var options []string
		if json.Unmarshal(parts[0], &options) != nil || len(options) == 0 {
			return config.ComfyUIParameterSettings{}, false
		}
		parameter.Type = "COMBO"
		parameter.Options = options
	}
	switch parameter.Type {
	case "STRING", "INT", "FLOAT", "BOOLEAN", "COMBO":
	default:
		return config.ComfyUIParameterSettings{}, false
	}
	if len(parts) > 1 {
		var options comfyUIInputOptions
		if json.Unmarshal(parts[1], &options) == nil {
			parameter.Min = options.Min
			parameter.Max = options.Max
			parameter.Step = options.Step
			parameter.Multiline = options.Multiline
		}
	}
	return parameter, true
}

func inferComfyUIParameterRoles(workflow comfyUIWorkflow) map[string]string {
	roles := make(map[string]string)
	positive := make(map[string]struct{})
	negative := make(map[string]struct{})
	for _, rawNode := range workflow {
		node, _ := rawNode.(map[string]any)
		if !strings.Contains(nodeString(node, "class_type"), "KSampler") {
			continue
		}
		inputs, _ := node["inputs"].(map[string]any)
		collectComfyUITextTargets(workflow, inputs["positive"], positive)
		collectComfyUITextTargets(workflow, inputs["negative"], negative)
	}
	if len(positive) == 1 {
		for key := range positive {
			roles[key] = config.ComfyUIParameterRolePrompt
		}
	}
	if len(negative) == 1 {
		for key := range negative {
			if _, shared := positive[key]; !shared {
				roles[key] = config.ComfyUIParameterRoleNegativePrompt
			}
		}
	}

	geometry := map[string][]string{"width": {}, "height": {}, "batch_size": {}}
	for nodeID, rawNode := range workflow {
		node, _ := rawNode.(map[string]any)
		classType := nodeString(node, "class_type")
		inputs, _ := node["inputs"].(map[string]any)
		if strings.Contains(classType, "LatentImage") {
			for inputName := range geometry {
				if value, ok := inputs[inputName]; ok && !isComfyUINodeLink(value) {
					geometry[inputName] = append(geometry[inputName], comfyUIParameterKey(nodeID, inputName))
				}
			}
		}
		if strings.Contains(classType, "KSampler") {
			for _, inputName := range []string{"seed", "noise_seed"} {
				if value, ok := inputs[inputName]; ok && !isComfyUINodeLink(value) {
					roles[comfyUIParameterKey(nodeID, inputName)] = config.ComfyUIParameterRoleSeed
				}
			}
		}
	}
	geometryRoles := map[string]string{
		"width": config.ComfyUIParameterRoleWidth, "height": config.ComfyUIParameterRoleHeight,
		"batch_size": config.ComfyUIParameterRoleBatchSize,
	}
	for inputName, keys := range geometry {
		if len(keys) == 1 {
			roles[keys[0]] = geometryRoles[inputName]
		}
	}
	return roles
}

func collectComfyUITextTargets(workflow comfyUIWorkflow, start any, targets map[string]struct{}) {
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
		if strings.HasPrefix(nodeString(node, "class_type"), "CLIPTextEncode") {
			if value, ok := inputs["text"]; ok && !isComfyUINodeLink(value) {
				targets[comfyUIParameterKey(current, "text")] = struct{}{}
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

func comfyUIParameterKey(nodeID, inputName string) string {
	return nodeID + "\x00" + inputName
}
