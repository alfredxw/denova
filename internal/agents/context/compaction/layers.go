package compaction

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/modelio"
	"denova/internal/agents/prompts"
)

// summarizeContextInLayers keeps every compactor provider request below the
// same hard input boundary as normal Agents. Oversized history is folded in
// ordered batches; no source bytes are discarded.
func summarizeContextInLayers(
	ctx context.Context,
	cfg *config.Config,
	agentKind string,
	existingCheckpoint string,
	source []*agent.Message,
	providerSource []*agent.Message,
	referenceContext string,
	sourceTokens int,
	policy Policy,
	snapshot *agent.ModelRequestSnapshot,
	coldFallbackReason string,
	summarize SummaryFunc,
	emitDelta func(attempt int, delta string),
) (string, int, contextCompactionSummaryExecution, error) {
	inputChars := contextCompactionInputChars(existingCheckpoint, source, referenceContext)
	if summary, chars, execution, attempted, err := summarizeContextWithPrimaryFork(
		ctx, cfg, agentKind, existingCheckpoint, source, providerSource, referenceContext,
		sourceTokens, policy, snapshot, coldFallbackReason, emitDelta,
	); attempted {
		return summary, chars, execution, err
	} else {
		coldExecution := execution
		coldExecution.Mode = ExecutionLayeredCold
		coldExecution.CacheIdentityStatus = CacheIdentityCold
		coldExecution.CacheUsageStatus = CacheUsageCold
		coldExecution.CacheMissReason = strings.TrimSpace(coldExecution.FallbackReason)
		if coldExecution.CacheMissReason == "" {
			coldExecution.CacheMissReason = CacheUsageCold
		}
		return summarizeContextInColdLayers(ctx, cfg, agentKind, existingCheckpoint, source, referenceContext, sourceTokens, policy, summarize, emitDelta, inputChars, coldExecution)
	}
}

func summarizeContextInColdLayers(
	ctx context.Context,
	cfg *config.Config,
	agentKind string,
	existingCheckpoint string,
	source []*agent.Message,
	referenceContext string,
	sourceTokens int,
	policy Policy,
	summarize SummaryFunc,
	emitDelta func(attempt int, delta string),
	inputChars int,
	execution contextCompactionSummaryExecution,
) (string, int, contextCompactionSummaryExecution, error) {
	resolved := config.ResolveAgentContext(cfg, config.AgentKindContextCompaction)
	maxBytes := resolved.MaxProviderInputBytes
	workspace := ""
	if cfg != nil {
		workspace = cfg.Workspace
	}
	composition, err := prompts.ComposeBuiltinSystemInstruction(cfg, config.AgentKindContextCompaction, "context_compaction", workspace, "builtin_base", "上下文压缩规则", "define the bounded context compaction task", prompts.ContextCompactionSystemInstruction())
	if err != nil {
		return "", inputChars, execution, err
	}
	probe := []*agent.Message{
		agent.SystemMessage(composition.Instruction()),
		agent.UserMessage(buildContextCompactionTranscript(source, existingCheckpoint, referenceContext, sourceTokens, inputChars, policy)),
	}
	modelWindow := config.ResolveAgentModel(cfg, config.AgentKindContextCompaction).ContextWindowTokens
	if modelio.ValidateInput(config.AgentKindContextCompaction, probe, nil, maxBytes, modelWindow) == nil {
		summary, err := executeColdCompactionSummary(ctx, cfg, summarize, SummaryRequest{
			SourceAgentKind: agentKind, Messages: probe, SourceMessages: len(source),
			SourceTokens: sourceTokens, InputChars: inputChars,
		}, emitDelta)
		execution.LayerCount = 1
		execution.InputTokens = agentcontext.EstimateTokens(probe, nil)
		execution.PromptTokens = execution.InputTokens
		return summary, inputChars, execution, err
	}

	batches := compactionSourceBatches(existingCheckpoint, referenceContext, source, maxBytes, modelWindow)
	if len(batches) == 0 {
		return "", inputChars, execution, fmt.Errorf("context compaction source is empty after layering")
	}
	execution.LayerCount = len(batches)
	execution.InputTokens = max(execution.InputTokens, sourceTokens)
	rolling := ""
	for index, batch := range batches {
		if err := ctx.Err(); err != nil {
			return "", inputChars, execution, err
		}
		layerEmit := emitDelta
		if index < len(batches)-1 {
			layerEmit = nil
		}
		layerTokens := agentcontext.EstimateTokens(batch, nil)
		layerChars := contextCompactionInputChars(rolling, batch, "")
		layerInput := []*agent.Message{
			agent.SystemMessage(composition.Instruction()),
			agent.UserMessage(buildContextCompactionTranscript(batch, rolling, "", layerTokens, layerChars, policy)),
		}
		summary, err := executeColdCompactionSummary(ctx, cfg, summarize, SummaryRequest{
			SourceAgentKind: agentKind, Messages: layerInput, SourceMessages: len(batch),
			SourceTokens: layerTokens, InputChars: layerChars,
		}, layerEmit)
		if err != nil {
			return "", inputChars, execution, fmt.Errorf("context compaction layer %d/%d: %w", index+1, len(batches), err)
		}
		rolling = strings.TrimSpace(summary)
	}
	return rolling, inputChars, execution, nil
}

func executeColdCompactionSummary(
	ctx context.Context,
	cfg *config.Config,
	summarize SummaryFunc,
	request SummaryRequest,
	emitDelta func(attempt int, delta string),
) (string, error) {
	if summarize == nil {
		return "", fmt.Errorf("context compaction summarizer is unavailable")
	}
	resolved := config.ResolveAgentContext(cfg, config.AgentKindContextCompaction)
	modelWindow := config.ResolveAgentModel(cfg, config.AgentKindContextCompaction).ContextWindowTokens
	if err := modelio.ValidateInput(config.AgentKindContextCompaction, request.Messages, nil, resolved.MaxProviderInputBytes, modelWindow); err != nil {
		return "", err
	}
	summary, err := summarize(ctx, cfg, request, emitDelta)
	if err != nil {
		return "", err
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return "", fmt.Errorf("context compaction result is empty")
	}
	return summary, nil
}

func compactionSourceBatches(existingCheckpoint, referenceContext string, source []*agent.Message, maxProviderBytes, maxProviderTokens int) [][]*agent.Message {
	if maxProviderBytes <= 0 {
		maxProviderBytes = config.DefaultAgentContextMaxProviderInputBytes
	}
	// Reserve most of the request for the system prompt, rolling checkpoint,
	// JSON envelope, and completion-policy instructions.
	payloadLimit := maxProviderBytes / 3
	if maxProviderTokens > 0 && maxProviderTokens/3 < payloadLimit {
		// Estimated tokens never exceed UTF-8 bytes in our estimator, so this
		// conservative byte allowance also bounds token-heavy CJK source.
		payloadLimit = maxProviderTokens / 3
	}
	if payloadLimit < 4*1024 {
		payloadLimit = maxProviderBytes / 2
	}
	if payloadLimit < 512 {
		payloadLimit = 512
	}
	blocks := make([]string, 0, len(source)+2)
	if value := strings.TrimSpace(existingCheckpoint); value != "" {
		blocks = append(blocks, "[existing_checkpoint]\n"+value)
	}
	if value := strings.TrimSpace(referenceContext); value != "" {
		blocks = append(blocks, "[reference_context]\n"+value)
	}
	for index, message := range source {
		if message != nil {
			blocks = append(blocks, formatCompactionMessage(index+1, message))
		}
	}

	var batches [][]*agent.Message
	var batch []*agent.Message
	batchBytes := 0
	appendPart := func(part string) {
		message := agent.UserMessage(part)
		partBytes := len(part)
		if batchBytes > 0 && batchBytes+partBytes > payloadLimit {
			batches = append(batches, batch)
			batch = nil
			batchBytes = 0
		}
		batch = append(batch, message)
		batchBytes += partBytes
	}
	for _, block := range blocks {
		parts := splitUTF8CompactionBlock(block, payloadLimit)
		for index, part := range parts {
			if len(parts) > 1 {
				part = fmt.Sprintf("[source_part %d/%d]\n%s", index+1, len(parts), part)
			}
			appendPart(part)
		}
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
	}
	return batches
}

func splitUTF8CompactionBlock(value string, maxBytes int) []string {
	value = strings.ToValidUTF8(value, "\uFFFD")
	if len(value) <= maxBytes {
		return []string{value}
	}
	parts := make([]string, 0, (len(value)+maxBytes-1)/maxBytes)
	for len(value) > 0 {
		end := min(len(value), maxBytes)
		for end > 0 && end < len(value) && !utf8.RuneStart(value[end]) {
			end--
		}
		if end == 0 {
			end = len(string([]rune(value)[0]))
		}
		parts = append(parts, value[:end])
		value = value[end:]
	}
	return parts
}
