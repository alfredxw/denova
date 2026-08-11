package compaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"denova/config"
	agentcontext "denova/internal/agents/context"

	agent "github.com/alfredxw/denova/agent"
	publiccompaction "github.com/alfredxw/denova/agent/compaction"
)

type denovaSummarizer struct {
	model         agent.BaseChatModel
	modelIdentity agent.CapabilityIdentity
	agentKind     string
}

func (summarizer denovaSummarizer) Identity() agent.CapabilityIdentity {
	return capabilityIdentity("denova.compaction.summarizer", struct {
		Model     agent.CapabilityIdentity
		AgentKind string
	}{summarizer.modelIdentity, summarizer.agentKind})
}

func (summarizer denovaSummarizer) Summarize(
	ctx context.Context,
	request publiccompaction.SummaryRequest,
) (publiccompaction.Summary, error) {
	if summarizer.model == nil {
		return publiccompaction.Summary{}, errors.New("Denova Compaction model is unavailable")
	}
	// ModelRequest is the exact provider-visible context. It includes custom
	// host context such as interactive story history, while Messages alone is
	// only the Agent-owned transcript range selected for replacement.
	messages := cloneMessages(request.ModelRequest)
	if len(messages) == 0 {
		messages = cloneMessages(request.Messages)
	}
	messages = append(messages, agent.UserMessage(checkpointInstruction(summarizer.agentKind)))
	result, err := summarizer.model.Generate(ctx, messages, agent.WithToolChoice(agent.ToolChoiceForbidden))
	if err != nil {
		return publiccompaction.Summary{}, fmt.Errorf("generate Denova Compaction checkpoint: %w", err)
	}
	if result == nil || strings.TrimSpace(result.Content) == "" {
		return publiccompaction.Summary{}, errors.New("Denova Compaction model returned an empty checkpoint")
	}
	content := strings.TrimSpace(result.Content)
	return publiccompaction.Summary{
		Content: content, TokenEstimate: agentcontext.EstimateStringTokens(content),
	}, nil
}

// NewAgentManager adapts Denova's context policy and model to the public Agent
// Compaction capability. Agent owns checkpoint durability and recovery; this
// package owns only Denova's planning configuration and summary semantics.
func NewAgentManager(
	cfg *config.Config,
	agentKind string,
	model agent.BaseChatModel,
	modelIdentity agent.CapabilityIdentity,
) (agent.CompactionManager, error) {
	settings := config.ResolveAgentContext(cfg, agentKind)
	hardLimit := settings.MaxProviderInputBytes
	if !settings.CompactionEnabled {
		return publiccompaction.Disabled(hardLimit), nil
	}
	modelSettings := config.ResolveAgentModel(cfg, agentKind)
	trigger := int(float64(modelSettings.ContextWindowTokens*4) * settings.CompactionThreshold)
	if trigger <= 0 || trigger >= hardLimit {
		trigger = int(float64(hardLimit) * settings.CompactionThreshold)
	}
	keepRecent := max(64<<10, trigger/5)
	if keepRecent >= trigger {
		keepRecent = trigger / 2
	}
	return publiccompaction.Standard(publiccompaction.StandardConfig{
		Summarizer: denovaSummarizer{
			model: model, modelIdentity: modelIdentity, agentKind: agentKind,
		},
		TriggerBytes: trigger, KeepRecentBytes: keepRecent, HardLimitBytes: hardLimit,
	})
}

func cloneMessages(messages []*agent.Message) []*agent.Message {
	result := make([]*agent.Message, len(messages))
	for index, message := range messages {
		result[index] = message.Clone()
	}
	return result
}

func checkpointInstruction(agentKind string) string {
	mode := "Preserve the user's objective and constraints, current draft or implementation state, file and artifact references, decisions and rationale, verified results, rejected approaches, unresolved risks, and dependency-ordered next actions."
	if agentKind == config.AgentKindInteractiveStory || agentKind == config.AgentKindInteractiveDirector {
		mode = "Preserve event order and causality, source turn IDs, actor-state changes, lore sources, director-plan status, relationships, quests, foreshadowing, secrets, dangers, and countdowns."
	}
	return "Create a durable Markdown context checkpoint from the preceding conversation. " + mode + " Never invent missing evidence. Exclude private reasoning, UI-only logs, streaming fragments, and transport noise. Return only the checkpoint in concise Markdown.\n\n根据前述对话创建可持久恢复的 Markdown 上下文检查点。保留事实、约束、决策、验证结果和后续行动，不得编造缺失信息；排除私有推理、仅 UI 日志、流式片段和传输噪音。只返回简洁的检查点正文。"
}

func capabilityIdentity(kind string, configuration any) agent.CapabilityIdentity {
	encoded, _ := json.Marshal(configuration)
	digest := sha256.Sum256(encoded)
	return agent.CapabilityIdentity{Kind: kind, Version: 1, ConfigHash: hex.EncodeToString(digest[:])}
}

var _ publiccompaction.Summarizer = denovaSummarizer{}
