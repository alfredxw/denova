package modeltask

import (
	"context"
	"denova/internal/agents/run"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"

	"denova/config"
	"denova/internal/agents/modelio"
	"denova/internal/agents/prompts"
	"denova/internal/book/lore"
)

const loreClassificationInputMaxBytes = 256 * 1024

type loreClassificationPayload struct {
	Items []lore.ClassificationSuggestion `json:"items"`
}

// ClassifyLoreItems runs one model-only semantic pass over an already bounded
// batch. Callers keep deterministic heuristic results if this call fails.
func ClassifyLoreItems(ctx context.Context, cfg *config.Config, inputs []lore.ClassificationInput) ([]lore.ClassificationSuggestion, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is missing")
	}
	if len(inputs) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(inputs)
	if err != nil {
		return nil, err
	}
	if len(data) > loreClassificationInputMaxBytes {
		return nil, fmt.Errorf("lore classification input exceeds the %d KiB limit", loreClassificationInputMaxBytes/1024)
	}
	traceCtx, finishTrace := agentrun.WithStandaloneTrace(ctx, cfg, config.AgentKindToolAgent, "tool_agent_lore_classification", "generate", map[string]any{"items": len(inputs), "bytes": len(data)})
	var runErr error
	defer func() { finishTrace(runErr) }()
	instruction := "Classify the following lore items semantically. The name is the strongest signal; consult tags, keywords, descriptions, and body excerpts only when the name is ambiguous.\n\nInput JSON:\n" + string(data)
	jsonCfg, err := modelio.ConfigForAgent(cfg, config.AgentKindToolAgent)
	if err != nil {
		runErr = err
		return nil, fmt.Errorf("resolve lore classification model configuration: %w", err)
	}
	jsonCfg = modelio.WithJSONObjectOutput(jsonCfg)
	result, err := generateLoreClassifications(traceCtx, cfg, jsonCfg, instruction, inputs, "json_mode")
	if err == nil {
		return result, nil
	}
	if traceCtx.Err() != nil {
		runErr = err
		return nil, err
	}
	slog.ErrorContext(ctx, fmt.Sprintf("[tool-agent] lore classification json_mode failed, retry without response_format err=%v", err))
	plainCfg, configErr := modelio.ConfigForAgent(cfg, config.AgentKindToolAgent)
	if configErr != nil {
		runErr = configErr
		return nil, fmt.Errorf("resolve lore classification fallback model configuration: %w", configErr)
	}
	result, runErr = generateLoreClassifications(traceCtx, cfg, plainCfg, instruction, inputs, "plain_text_retry")
	return result, runErr
}

func generateLoreClassifications(ctx context.Context, cfg *config.Config, modelCfg providers.ModelConfig, instruction string, inputs []lore.ClassificationInput, attempt string) ([]lore.ClassificationSuggestion, error) {
	cm, err := modelio.NewChatModel(ctx, modelCfg)
	if err != nil {
		return nil, fmt.Errorf("create Tool Agent model: %w", err)
	}
	composition, err := prompts.ComposeBuiltinSystemInstruction(cfg, config.AgentKindToolAgent, "tool_agent", cfg.Workspace, "builtin_base", "Lore Classification Task", "define the structured lore classification task", loreClassificationSystemInstruction())
	if err != nil {
		return nil, err
	}
	messages := []*agent.Message{
		agent.SystemMessage(composition.Instruction()),
		agent.UserMessage(instruction),
	}
	if err := modelio.ValidateConfiguredInput(cfg, config.AgentKindToolAgent, messages, nil); err != nil {
		return nil, err
	}
	mode := "generate_" + attempt
	span, callID, traceCtx := agentrun.BeginLLMCallTrace(ctx, config.AgentKindToolAgent, "tool_agent_lore_classification", mode, modelCfg, messages, nil, false)
	msg, err := cm.Generate(traceCtx, messages)
	if err != nil {
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindToolAgent, "tool_agent_lore_classification", mode, modelCfg.Model, 0, nil, err, nil)
		return nil, fmt.Errorf("Tool Agent lore classification: %w", err)
	}
	if msg == nil {
		err = fmt.Errorf("Tool Agent returned an empty response")
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindToolAgent, "tool_agent_lore_classification", mode, modelCfg.Model, 0, nil, err, nil)
		return nil, err
	}
	agentrun.FinishLLMCallTrace(span, callID, config.AgentKindToolAgent, "tool_agent_lore_classification", mode, modelCfg.Model, 0, msg, nil, nil)
	result, err := parseLoreClassificationContent(msg.Content, inputs)
	if err != nil && strings.TrimSpace(msg.Content) == "" && strings.TrimSpace(msg.ReasoningContent) != "" {
		result, err = parseLoreClassificationContent(msg.ReasoningContent, inputs)
	}
	if err != nil {
		return nil, fmt.Errorf("parse Tool Agent lore-classification output: %w", err)
	}
	slog.InfoContext(ctx, fmt.Sprintf("[tool-agent] lore classification done attempt=%s requested=%d returned=%d", attempt, len(inputs), len(result)))
	return result, nil
}

func parseLoreClassificationContent(content string, inputs []lore.ClassificationInput) ([]lore.ClassificationSuggestion, error) {
	var payload loreClassificationPayload
	if err := json.Unmarshal([]byte(extractJSONContent(content)), &payload); err != nil {
		return nil, err
	}
	allowedIDs := map[string]bool{}
	for _, input := range inputs {
		allowedIDs[strings.TrimSpace(input.ID)] = true
	}
	seen := map[string]bool{}
	result := make([]lore.ClassificationSuggestion, 0, len(payload.Items))
	for _, item := range payload.Items {
		item.ID = strings.TrimSpace(item.ID)
		item.Type = strings.TrimSpace(item.Type)
		item.Confidence = strings.ToLower(strings.TrimSpace(item.Confidence))
		item.Reason = strings.TrimSpace(item.Reason)
		if !allowedIDs[item.ID] || seen[item.ID] {
			return nil, fmt.Errorf("response contains an unknown or duplicate lore ID: %s", item.ID)
		}
		if !isLoreClassificationType(item.Type) {
			return nil, fmt.Errorf("lore item %s has an invalid returned type: %s", item.ID, item.Type)
		}
		switch item.Confidence {
		case lore.ClassificationConfidenceHigh, lore.ClassificationConfidenceMedium, lore.ClassificationConfidenceLow:
		default:
			item.Confidence = lore.ClassificationConfidenceLow
		}
		seen[item.ID] = true
		result = append(result, item)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("response contains no classification results")
	}
	return result, nil
}

func isLoreClassificationType(value string) bool {
	switch value {
	case "character", "world", "location", "faction", "rule", "item", "other":
		return true
	default:
		return false
	}
}

func loreClassificationSystemInstruction() string {
	return strings.Join([]string{
		"Classify Denova lore items.",
		"Output only a JSON object: {\"items\":[{\"id\":\"input id\",\"type\":\"character|world|location|faction|rule|item|other\",\"confidence\":\"high|medium|low\",\"reason\":\"brief rationale\"}]}.",
		"Names take precedence over bodies. Names clearly denoting character details or profiles classify as character; apply the same principle to explicit location, faction, rule, and item names.",
		"Use world only for worldbuilding, history, culture, or era background spanning locations. When classification is uncertain, use other with low confidence instead of guessing.",
		"Return each input id at most once, never invent an id absent from the input, and do not output Markdown.",
	}, "\n")
}
