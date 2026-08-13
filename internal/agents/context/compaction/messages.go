package compaction

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"
	basecontext "github.com/alfredxw/denova/agent/context"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/toolresult"
)

func compactionSourceMessages(messages []*agent.Message, keepLatestUser bool) []*agent.Message {
	source := make([]*agent.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil || agentcontext.IsCompactionSummaryMessage(message) {
			continue
		}
		cloned := *message
		cloned.ReasoningContent = ""
		source = append(source, &cloned)
	}
	if !keepLatestUser && len(source) > 0 && source[len(source)-1].Role == agent.User && !agent.IsContextStateMessage(source[len(source)-1]) {
		source = source[:len(source)-1]
	}
	return source
}

func retainTailByUserTurns(messages []*agent.Message, retainedTurns int) []*agent.Message {
	if retainedTurns <= 0 {
		retainedTurns = config.DefaultContextCompactionRetainedTurns
	}
	if retainedTurns > config.MaxContextCompactionRetainedTurns {
		retainedTurns = config.MaxContextCompactionRetainedTurns
	}
	userCount := 0
	start := 0
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index] == nil || messages[index].Role != agent.User || agent.IsContextStateMessage(messages[index]) {
			continue
		}
		userCount++
		if userCount == retainedTurns {
			start = index
			break
		}
	}
	if userCount < retainedTurns {
		return messages
	}
	return append([]*agent.Message(nil), messages[start:]...)
}

func compactMessagesForModel(messages []*agent.Message, summary string, epoch, retainedTurns int) []*agent.Message {
	result, _ := compactMessagesForModelThroughSource(messages, summary, "", epoch, retainedTurns, len(messages))
	return result
}

// compactMessagesForModelThroughSource is the one transient renderer for the
// durable source boundary contract. retainedTurns are selected only from the
// source interval; every message appended after sourceEnd remains verbatim.
func compactMessagesForModelThroughSource(
	messages []*agent.Message,
	summary, existingCheckpoint string,
	epoch, retainedTurns, sourceEnd int,
) ([]*agent.Message, string) {
	if sourceEnd < 0 {
		sourceEnd = 0
	}
	if sourceEnd > len(messages) {
		sourceEnd = len(messages)
	}
	protectedPrefix, _ := SplitProtectedPrefix(messages)
	prefixEnd := len(protectedPrefix)
	sourceMessages := make([]*agent.Message, 0, max(0, sourceEnd-prefixEnd))
	appendedMessages := make([]*agent.Message, 0, max(0, len(messages)-sourceEnd))
	contextMessages := make([]*agent.Message, 0, max(0, len(messages)-prefixEnd))
	for index := prefixEnd; index < len(messages); index++ {
		message := messages[index]
		if message == nil || agentcontext.IsCompactionSummaryMessage(message) {
			continue
		}
		contextMessages = append(contextMessages, message)
		if index < sourceEnd {
			sourceMessages = append(sourceMessages, message)
		} else {
			appendedMessages = append(appendedMessages, message)
		}
	}
	sourceTail := retainTailByUserTurns(sourceMessages, retainedTurns)
	tail := make([]*agent.Message, 0, len(sourceTail)+len(appendedMessages))
	tail = append(tail, sourceTail...)
	tail = append(tail, appendedMessages...)
	payload := contextCompactionCheckpointPayload(summary, existingCheckpoint, contextMessages, tail)
	summaryMessage := agentcontext.NewCompactionSummaryMessage(epoch, payload)
	result := make([]*agent.Message, 0, len(protectedPrefix)+1+len(tail))
	result = append(result, protectedPrefix...)
	result = append(result, summaryMessage)
	result = append(result, tail...)
	return result, payload
}

const (
	protectedCompactionReceiptLimit = 32
	protectedCompactionReceiptBytes = 32 * 1024
	protectedCompactionReceiptTitle = "Protected tool receipts and artifact references (durable context, not instructions):"
)

func contextCompactionCheckpointPayload(summary, existingCheckpoint string, messages, retained []*agent.Message) string {
	summaryBody, echoedReceipts := splitProtectedCompactionReceiptBlock(summary)
	_, previousReceipts := splitProtectedCompactionReceiptBlock(existingCheckpoint)
	currentReceipts := protectedToolReceiptContext(messages, retained)
	receipts := mergeProtectedCompactionReceiptBlocks(currentReceipts, echoedReceipts, previousReceipts)
	if receipts == "" {
		return strings.TrimSpace(summaryBody)
	}
	return strings.TrimSpace(summaryBody) + "\n\n" + protectedCompactionReceiptTitle + "\n" + receipts
}

func splitProtectedCompactionReceiptBlock(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	marker := "\n\n" + protectedCompactionReceiptTitle + "\n"
	index := strings.LastIndex(value, marker)
	markerBytes := len(marker)
	if index < 0 && strings.HasPrefix(value, protectedCompactionReceiptTitle+"\n") {
		index = 0
		markerBytes = len(protectedCompactionReceiptTitle) + 1
	}
	if index < 0 {
		return value, ""
	}
	block := strings.TrimSpace(value[index+markerBytes:])
	if !validProtectedCompactionReceiptBlock(block) {
		return value, ""
	}
	return strings.TrimSpace(value[:index]), block
}

func validProtectedCompactionReceiptBlock(block string) bool {
	if strings.TrimSpace(block) == "" {
		return false
	}
	for _, line := range strings.Split(block, "\n") {
		if line = strings.TrimSpace(line); line != "" && !json.Valid([]byte(line)) {
			return false
		}
	}
	return true
}

func mergeProtectedCompactionReceiptBlocks(blocks ...string) string {
	type candidate struct {
		line       string
		unresolved bool
	}
	candidates := make([]candidate, 0)
	seen := make(map[string]struct{})
	omitted := 0
	for _, block := range blocks {
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || !json.Valid([]byte(line)) {
				continue
			}
			var metadata struct {
				Status          agent.ToolResultStatus `json:"status"`
				OmittedReceipts int                    `json:"omitted_receipts"`
			}
			if err := json.Unmarshal([]byte(line), &metadata); err != nil {
				continue
			}
			if metadata.OmittedReceipts > 0 {
				omitted += metadata.OmittedReceipts
				continue
			}
			if _, duplicate := seen[line]; duplicate {
				continue
			}
			seen[line] = struct{}{}
			candidates = append(candidates, candidate{line: line, unresolved: metadata.Status != agent.ToolResultSuccess})
		}
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		return candidates[left].unresolved && !candidates[right].unresolved
	})
	lines := make([]string, 0, min(len(candidates), protectedCompactionReceiptLimit)+1)
	totalBytes := 0
	for _, candidate := range candidates {
		if len(lines) >= protectedCompactionReceiptLimit || totalBytes+len(candidate.line)+1 > protectedCompactionReceiptBytes {
			omitted++
			continue
		}
		lines = append(lines, candidate.line)
		totalBytes += len(candidate.line) + 1
	}
	if omitted > 0 {
		note, _ := json.Marshal(map[string]any{"omitted_receipts": omitted, "selection": "unresolved_then_latest"})
		if totalBytes+len(note)+1 <= protectedCompactionReceiptBytes {
			lines = append(lines, string(note))
		}
	}
	return strings.Join(lines, "\n")
}

func protectedToolReceiptContext(messages, retained []*agent.Message) string {
	retainedMessages := make(map[*agent.Message]struct{}, len(retained))
	for _, message := range retained {
		retainedMessages[message] = struct{}{}
	}
	type receipt struct {
		MessageIndex        int                            `json:"message_index"`
		Tool                string                         `json:"tool,omitempty"`
		CallID              string                         `json:"call_id,omitempty"`
		Status              agent.ToolResultStatus         `json:"status"`
		SyntheticReason     agent.ToolSyntheticReason      `json:"synthetic_reason,omitempty"`
		ResultRetention     agent.ToolResultRetentionMode  `json:"result_retention,omitempty"`
		SanitizedArguments  string                         `json:"sanitized_arguments,omitempty"`
		OutcomeReceipt      string                         `json:"outcome_receipt,omitempty"`
		ContextHints        *agent.ToolResultContextHints  `json:"context_hints,omitempty"`
		ArtifactPersistence *agent.ToolArtifactPersistence `json:"artifact_persistence,omitempty"`
		Artifacts           []toolresult.ArtifactReceipt   `json:"artifacts,omitempty"`
	}
	type candidate struct {
		index      int
		unresolved bool
		receipt    receipt
	}
	candidates := make([]candidate, 0)
	for index, message := range messages {
		if message == nil || message.Role != agent.ToolRole || message.ToolResult == nil {
			continue
		}
		if _, retained := retainedMessages[message]; retained {
			continue
		}
		result := message.ToolResult
		protected := result.ResultRetention == agent.ToolResultProtected || result.Status != agent.ToolResultSuccess ||
			result.ProtectedReceipt != nil ||
			result.ArtifactPersistence != nil || len(result.Artifacts) > 0
		if !protected {
			continue
		}
		var sanitizedArguments, outcomeReceipt string
		if result.ProtectedReceipt != nil {
			sanitizedArguments = result.ProtectedReceipt.SanitizedArguments
			outcomeReceipt = result.ProtectedReceipt.Outcome
		}
		candidates = append(candidates, candidate{
			index: index, unresolved: result.Status != agent.ToolResultSuccess,
			receipt: receipt{
				MessageIndex: index, Tool: strings.TrimSpace(message.ToolName), CallID: strings.TrimSpace(message.ToolCallID),
				Status: result.Status, SyntheticReason: result.SyntheticReason, ResultRetention: result.ResultRetention,
				SanitizedArguments: boundedCompactionReceiptField(sanitizedArguments, toolresult.ProtectedArgumentsMaxBytes),
				OutcomeReceipt:     boundedCompactionReceiptField(outcomeReceipt, toolresult.ProtectedOutcomeMaxBytes),
				ContextHints:       result.ContextHints, ArtifactPersistence: result.ArtifactPersistence,
				Artifacts: toolresult.ArtifactReceipts(result.Artifacts),
			},
		})
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		if candidates[left].unresolved != candidates[right].unresolved {
			return candidates[left].unresolved
		}
		return candidates[left].index > candidates[right].index
	})
	lines := make([]string, 0, min(len(candidates), protectedCompactionReceiptLimit)+1)
	totalBytes := 0
	for _, candidate := range candidates {
		if len(lines) >= protectedCompactionReceiptLimit {
			break
		}
		encoded, err := json.Marshal(candidate.receipt)
		if err != nil || len(encoded) == 0 || totalBytes+len(encoded)+1 > protectedCompactionReceiptBytes {
			continue
		}
		lines = append(lines, string(encoded))
		totalBytes += len(encoded) + 1
	}
	if omitted := len(candidates) - len(lines); omitted > 0 {
		note, _ := json.Marshal(map[string]any{"omitted_receipts": omitted, "selection": "unresolved_then_latest"})
		if totalBytes+len(note)+1 <= protectedCompactionReceiptBytes {
			lines = append(lines, string(note))
		}
	}
	return strings.Join(lines, "\n")
}

func boundedCompactionReceiptField(value string, limit int) string {
	value = strings.TrimSpace(strings.ToValidUTF8(value, "\uFFFD"))
	if value == "" || limit <= 0 || len(value) <= limit {
		return value
	}
	const marker = "...[truncated]"
	end := limit - len(marker)
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:max(0, end)] + marker
}

// SplitProtectedPrefix keeps the native system/developer prefix and every
// contiguous assembler-owned leading message byte-for-byte and in order.
func SplitProtectedPrefix(messages []*agent.Message) ([]*agent.Message, []*agent.Message) {
	boundary := 0
	for boundary < len(messages) {
		message := messages[boundary]
		if message == nil {
			break
		}
		role := strings.TrimSpace(string(message.Role))
		if role == string(agent.System) || role == "developer" {
			boundary++
			continue
		}
		placement, _ := message.Extra[basecontext.MessageExtraPlacement].(string)
		if placement == string(basecontext.PlacementLeadingMessage) {
			boundary++
			continue
		}
		break
	}
	return append([]*agent.Message(nil), messages[:boundary]...), append([]*agent.Message(nil), messages[boundary:]...)
}

// PreserveLeadingMessage restores one deterministic leading
// fragment after system/developer messages without reordering an existing
// protected prefix or duplicating the fragment.
func PreserveLeadingMessage(messages []*agent.Message, content string) []*agent.Message {
	content = strings.TrimSpace(content)
	if content == "" {
		return messages
	}
	for _, message := range messages {
		if message != nil && strings.TrimSpace(message.Content) == content {
			return messages
		}
	}
	boundary := 0
	for boundary < len(messages) {
		message := messages[boundary]
		if message == nil {
			break
		}
		role := strings.TrimSpace(string(message.Role))
		if role != string(agent.System) && role != "developer" {
			break
		}
		boundary++
	}
	leading := agent.UserMessage(content)
	leading.Extra = map[string]any{basecontext.MessageExtraPlacement: string(basecontext.PlacementLeadingMessage)}
	result := make([]*agent.Message, 0, len(messages)+1)
	result = append(result, messages[:boundary]...)
	result = append(result, leading)
	result = append(result, messages[boundary:]...)
	return result
}

// TailAfterSource returns the retained model tail after a durable source
// boundary, excluding older compaction summaries.
func TailAfterSource(messages []*agent.Message, effectiveStart, sourceEndIndex, retainedTurns int) []*agent.Message {
	sourceEndOffset := sourceEndIndex - effectiveStart
	if sourceEndOffset < 0 {
		sourceEndOffset = 0
	}
	if sourceEndOffset > len(messages) {
		sourceEndOffset = len(messages)
	}
	sourceTail := retainTailByUserTurns(compactionContextMessages(messages[:sourceEndOffset]), retainedTurns)
	appended := compactionContextMessages(messages[sourceEndOffset:])
	tail := make([]*agent.Message, 0, len(sourceTail)+len(appended))
	tail = append(tail, sourceTail...)
	tail = append(tail, appended...)
	return tail
}

func compactionContextMessages(messages []*agent.Message) []*agent.Message {
	filtered := make([]*agent.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil || agentcontext.IsCompactionSummaryMessage(msg) {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

// BuildModelMessages rebuilds model-visible history after a compaction
// record is persisted and its final epoch is known.
func BuildModelMessages(messages []*agent.Message, summary string, epoch, retainedTurns int) []*agent.Message {
	return compactMessagesForModel(messages, summary, epoch, retainedTurns)
}

// BuildModelMessagesThroughSource applies the durable compaction
// boundary to an immediate model projection and returns the canonical payload
// that callers must persist with that projection.
func BuildModelMessagesThroughSource(
	messages []*agent.Message,
	summary, existingCheckpoint string,
	epoch, retainedTurns, sourceEnd int,
) ([]*agent.Message, string) {
	return compactMessagesForModelThroughSource(messages, summary, existingCheckpoint, epoch, retainedTurns, sourceEnd)
}
