package agent

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	adk "github.com/alfredxw/denova/adk"

	"denova/config"
)

// summarizeContextInLayers keeps every compactor provider request below the
// same hard input boundary as normal Agents. Oversized history is folded in
// ordered batches; no source bytes are discarded.
func summarizeContextInLayers(
	ctx context.Context,
	cfg *config.Config,
	agentKind string,
	existingCheckpoint string,
	source []*adk.Message,
	referenceContext string,
	sourceTokens int,
	policy contextCompactionPolicy,
	emitDelta func(attempt int, delta string),
) (string, int, error) {
	inputChars := contextCompactionInputChars(existingCheckpoint, source, referenceContext)
	resolved := config.ResolveAgentContext(cfg, config.AgentKindContextCompaction)
	maxBytes := resolved.MaxProviderInputBytes
	composition, err := composeBuiltinSystemInstruction(cfg, config.AgentKindContextCompaction, "context_compaction", workspaceForPrompt(cfg, nil), "builtin_base", "上下文压缩规则", "define the bounded context compaction task", contextCompactionSystemInstruction())
	if err != nil {
		return "", inputChars, err
	}
	probe := []*adk.Message{
		adk.SystemMessage(composition.Instruction()),
		adk.UserMessage(buildContextCompactionTranscript(source, existingCheckpoint, referenceContext, sourceTokens, inputChars, policy)),
	}
	modelWindow := config.ResolveAgentModel(cfg, config.AgentKindContextCompaction).ContextWindowTokens
	if validateProviderInput(config.AgentKindContextCompaction, probe, nil, maxBytes, modelWindow) == nil {
		return summarizeContextForCompaction(ctx, cfg, agentKind, existingCheckpoint, source, referenceContext, sourceTokens, policy, emitDelta)
	}

	batches := compactionSourceBatches(existingCheckpoint, referenceContext, source, maxBytes, modelWindow)
	if len(batches) == 0 {
		return "", inputChars, fmt.Errorf("context compaction source is empty after layering")
	}
	rolling := ""
	for index, batch := range batches {
		if err := ctx.Err(); err != nil {
			return "", inputChars, err
		}
		layerEmit := emitDelta
		if index < len(batches)-1 {
			layerEmit = nil
		}
		summary, _, err := summarizeContextForCompaction(
			ctx, cfg, agentKind, rolling, batch, "", EstimateContextTokens(batch, nil), policy, layerEmit,
		)
		if err != nil {
			return "", inputChars, fmt.Errorf("context compaction layer %d/%d: %w", index+1, len(batches), err)
		}
		rolling = strings.TrimSpace(summary)
	}
	return rolling, inputChars, nil
}

func compactionSourceBatches(existingCheckpoint, referenceContext string, source []*adk.Message, maxProviderBytes, maxProviderTokens int) [][]*adk.Message {
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

	var batches [][]*adk.Message
	var batch []*adk.Message
	batchBytes := 0
	appendPart := func(part string) {
		message := adk.UserMessage(part)
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
