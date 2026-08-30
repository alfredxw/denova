package generation

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"sort"
	"strings"

	"denova/config"
)

const maxComfyUIWorkflowBytes = 5 << 20

type comfyUIWorkflow map[string]any

func prepareComfyUIWorkflow(profile config.ResolvedImageAPIProfile, request GenerateRequest) (comfyUIWorkflow, error) {
	return prepareUploadedComfyUIWorkflow(profile.ComfyUI, request)
}

func prepareUploadedComfyUIWorkflow(settings config.ComfyUIProfileSettings, request GenerateRequest) (comfyUIWorkflow, error) {
	raw := strings.TrimSpace(settings.Workflow)
	if raw == "" {
		return nil, fmt.Errorf("ComfyUI API-format workflow is missing")
	}
	if len(raw) > maxComfyUIWorkflowBytes {
		return nil, fmt.Errorf("ComfyUI workflow exceeds %d bytes", maxComfyUIWorkflowBytes)
	}
	workflow, err := decodeComfyUIWorkflowJSON([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("decode ComfyUI API-format workflow: %w", err)
	}
	if len(workflow) == 0 {
		return nil, fmt.Errorf("ComfyUI API-format workflow is empty")
	}
	if err := validateComfyUIWorkflow(workflow); err != nil {
		return nil, err
	}
	switch settings.WorkflowMode {
	case config.ComfyUIWorkflowRemote:
		if err := applyComfyUIBindings(workflow, settings.Bindings, request); err != nil {
			return nil, err
		}
		return workflow, nil
	case config.ComfyUIWorkflowAPI:
		if err := injectComfyUIPrompt(workflow, request.Prompt); err != nil {
			return nil, err
		}
		if err := injectComfyUIRequestOptions(workflow, request); err != nil {
			return nil, err
		}
		return workflow, nil
	default:
		return nil, fmt.Errorf("unsupported ComfyUI workflow mode %q", settings.WorkflowMode)
	}
}

func decodeComfyUIWorkflowJSON(raw []byte) (comfyUIWorkflow, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var workflow comfyUIWorkflow
	if err := decoder.Decode(&workflow); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("workflow contains multiple JSON values")
		}
		return nil, err
	}
	return workflow, nil
}

func applyComfyUIBindings(workflow comfyUIWorkflow, bindings *config.ComfyUIBindings, request GenerateRequest) error {
	if bindings == nil || bindings.Prompt == nil {
		return fmt.Errorf("ComfyUI workflow prompt binding is missing")
	}
	width, height := comfyUIImageDimensions(request.Size)
	if err := setComfyUIBindingValue(workflow, bindings.Prompt, request.Prompt); err != nil {
		return err
	}
	if bindings.Count != nil {
		if err := setComfyUIBindingValue(workflow, bindings.Count, request.N); err != nil {
			return err
		}
	}
	if bindings.Width != nil {
		if err := setComfyUIBindingValue(workflow, bindings.Width, width); err != nil {
			return err
		}
	}
	if bindings.Height != nil {
		if err := setComfyUIBindingValue(workflow, bindings.Height, height); err != nil {
			return err
		}
	}
	return injectComfyUISeeds(workflow)
}

func setComfyUIBindingValue(workflow comfyUIWorkflow, binding *config.ComfyUIInputBinding, value any) error {
	node, ok := workflow[binding.NodeID].(map[string]any)
	if !ok {
		return fmt.Errorf("ComfyUI binding node %q is missing", binding.NodeID)
	}
	inputs, ok := node["inputs"].(map[string]any)
	if !ok {
		return fmt.Errorf("ComfyUI binding node %q has invalid inputs", binding.NodeID)
	}
	if _, ok := inputs[binding.InputName]; !ok {
		return fmt.Errorf("ComfyUI binding %s.%s is missing", binding.NodeID, binding.InputName)
	}
	inputs[binding.InputName] = value
	return nil
}

func validateComfyUIWorkflow(workflow comfyUIWorkflow) error {
	for nodeID, rawNode := range workflow {
		node, ok := rawNode.(map[string]any)
		if !ok {
			return fmt.Errorf("ComfyUI workflow node %q must be an object", nodeID)
		}
		classType, ok := node["class_type"].(string)
		if !ok || strings.TrimSpace(classType) == "" {
			return fmt.Errorf("ComfyUI workflow node %q is missing class_type", nodeID)
		}
		if _, ok := node["inputs"].(map[string]any); !ok {
			return fmt.Errorf("ComfyUI workflow node %q is missing inputs", nodeID)
		}
	}
	return nil
}

func injectComfyUIPrompt(workflow comfyUIWorkflow, prompt string) error {
	for _, rawNode := range workflow {
		node, _ := rawNode.(map[string]any)
		if !strings.Contains(nodeString(node, "class_type"), "KSampler") {
			continue
		}
		inputs, _ := node["inputs"].(map[string]any)
		link, ok := inputs["positive"].([]any)
		if !ok || len(link) == 0 {
			continue
		}
		nodeID, ok := link[0].(string)
		if ok && setComfyUIText(workflow[nodeID], prompt) {
			return nil
		}
	}
	nodeIDs := make([]string, 0, len(workflow))
	for nodeID := range workflow {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Strings(nodeIDs)
	candidates := make([]any, 0, 1)
	for _, nodeID := range nodeIDs {
		node, _ := workflow[nodeID].(map[string]any)
		if !strings.HasPrefix(nodeString(node, "class_type"), "CLIPTextEncode") || comfyUINodeIsNegative(node) {
			continue
		}
		if comfyUINodeIsPositive(node) && setComfyUIText(node, prompt) {
			return nil
		}
		candidates = append(candidates, node)
	}
	if len(candidates) == 1 && setComfyUIText(candidates[0], prompt) {
		return nil
	}
	return fmt.Errorf("ComfyUI workflow has no unambiguous writable positive CLIPTextEncode prompt")
}

func injectComfyUIRequestOptions(workflow comfyUIWorkflow, request GenerateRequest) error {
	width, height := comfyUIImageDimensions(request.Size)
	for _, rawNode := range workflow {
		node, _ := rawNode.(map[string]any)
		classType := nodeString(node, "class_type")
		inputs, _ := node["inputs"].(map[string]any)
		if strings.Contains(classType, "LatentImage") {
			if _, ok := inputs["width"]; ok && width > 0 {
				inputs["width"] = width
			}
			if _, ok := inputs["height"]; ok && height > 0 {
				inputs["height"] = height
			}
			if _, ok := inputs["batch_size"]; ok {
				inputs["batch_size"] = request.N
			}
		}
	}
	return injectComfyUISeeds(workflow)
}

func injectComfyUISeeds(workflow comfyUIWorkflow) error {
	seed, err := randomComfyUISeed()
	if err != nil {
		return err
	}
	for _, rawNode := range workflow {
		node, _ := rawNode.(map[string]any)
		classType := nodeString(node, "class_type")
		inputs, _ := node["inputs"].(map[string]any)
		if strings.Contains(classType, "KSampler") {
			if _, ok := inputs["seed"]; ok {
				inputs["seed"] = seed
			}
			if _, ok := inputs["noise_seed"]; ok {
				inputs["noise_seed"] = seed
			}
		}
	}
	return nil
}

func setComfyUIText(rawNode any, prompt string) bool {
	node, ok := rawNode.(map[string]any)
	if !ok || !strings.HasPrefix(nodeString(node, "class_type"), "CLIPTextEncode") {
		return false
	}
	inputs, ok := node["inputs"].(map[string]any)
	if !ok {
		return false
	}
	if _, ok := inputs["text"]; !ok {
		return false
	}
	inputs["text"] = prompt
	return true
}

func comfyUINodeIsNegative(node map[string]any) bool {
	metadata, _ := node["_meta"].(map[string]any)
	title := strings.ToLower(nodeString(metadata, "title"))
	return strings.Contains(title, "negative")
}

func comfyUINodeIsPositive(node map[string]any) bool {
	metadata, _ := node["_meta"].(map[string]any)
	title := strings.ToLower(nodeString(metadata, "title"))
	return strings.Contains(title, "positive") || strings.Contains(title, "prompt")
}

func nodeString(node map[string]any, key string) string {
	value, _ := node[key].(string)
	return strings.TrimSpace(value)
}

func comfyUIImageDimensions(size string) (int, int) {
	if width, height, ok := parseImageDimensions(size); ok {
		return width, height
	}
	return 1024, 1024
}

func randomComfyUISeed() (int64, error) {
	seed, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return 0, fmt.Errorf("create ComfyUI seed: %w", err)
	}
	return seed.Int64(), nil
}
