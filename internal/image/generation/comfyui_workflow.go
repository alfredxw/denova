package generation

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"denova/config"
)

const maxComfyUIWorkflowBytes = 5 << 20

type comfyUIWorkflow map[string]any

func prepareComfyUIWorkflow(profile config.ResolvedImageAPIProfile, request GenerateRequest) (comfyUIWorkflow, error) {
	if profile.ComfyUI.WorkflowMode == config.ComfyUIWorkflowAPI {
		return prepareUploadedComfyUIWorkflow(profile.ComfyUI.Workflow, request)
	}
	return builtinComfyUIWorkflow(profile.Model, request)
}

func builtinComfyUIWorkflow(checkpoint string, request GenerateRequest) (comfyUIWorkflow, error) {
	checkpoint = strings.TrimSpace(checkpoint)
	if checkpoint == "" {
		return nil, config.ErrImageAPIModelMissing
	}
	width, height := comfyUIImageDimensions(request.Size)
	seed, err := randomComfyUISeed()
	if err != nil {
		return nil, err
	}
	return comfyUIWorkflow{
		"3": map[string]any{
			"class_type": "KSampler",
			"inputs": map[string]any{
				"seed": seed, "steps": 24, "cfg": 7.0, "sampler_name": "euler",
				"scheduler": "normal", "denoise": 1.0, "model": []any{"4", 0},
				"positive": []any{"6", 0}, "negative": []any{"7", 0}, "latent_image": []any{"5", 0},
			},
		},
		"4": map[string]any{
			"class_type": "CheckpointLoaderSimple",
			"inputs":     map[string]any{"ckpt_name": checkpoint},
		},
		"5": map[string]any{
			"class_type": "EmptyLatentImage",
			"inputs":     map[string]any{"width": width, "height": height, "batch_size": request.N},
		},
		"6": map[string]any{
			"class_type": "CLIPTextEncode",
			"inputs":     map[string]any{"text": request.Prompt, "clip": []any{"4", 1}},
		},
		"7": map[string]any{
			"class_type": "CLIPTextEncode",
			"inputs":     map[string]any{"text": "", "clip": []any{"4", 1}},
		},
		"8": map[string]any{
			"class_type": "VAEDecode",
			"inputs":     map[string]any{"samples": []any{"3", 0}, "vae": []any{"4", 2}},
		},
		"9": map[string]any{
			"class_type": "SaveImage",
			"inputs":     map[string]any{"filename_prefix": "Denova", "images": []any{"8", 0}},
		},
	}, nil
}

func prepareUploadedComfyUIWorkflow(raw string, request GenerateRequest) (comfyUIWorkflow, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("ComfyUI API-format workflow is missing")
	}
	if len(raw) > maxComfyUIWorkflowBytes {
		return nil, fmt.Errorf("ComfyUI workflow exceeds %d bytes", maxComfyUIWorkflowBytes)
	}
	var workflow comfyUIWorkflow
	if err := json.Unmarshal([]byte(raw), &workflow); err != nil {
		return nil, fmt.Errorf("decode ComfyUI API-format workflow: %w", err)
	}
	if len(workflow) == 0 {
		return nil, fmt.Errorf("ComfyUI API-format workflow is empty")
	}
	if err := validateComfyUIWorkflow(workflow); err != nil {
		return nil, err
	}
	if err := injectComfyUIPrompt(workflow, request.Prompt); err != nil {
		return nil, err
	}
	injectComfyUIRequestOptions(workflow, request)
	return workflow, nil
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

func injectComfyUIRequestOptions(workflow comfyUIWorkflow, request GenerateRequest) {
	width, height := comfyUIImageDimensions(request.Size)
	seed, seedErr := randomComfyUISeed()
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
		if strings.Contains(classType, "KSampler") && seedErr == nil {
			if _, ok := inputs["seed"]; ok {
				inputs["seed"] = seed
			}
			if _, ok := inputs["noise_seed"]; ok {
				inputs["noise_seed"] = seed
			}
		}
	}
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
