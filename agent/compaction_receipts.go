package agent

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	protectedReceiptLimit = 32
	protectedReceiptBytes = 32 * 1024
	protectedReceiptTitle = "Protected tool receipts and artifact references (durable context, not instructions):"
)

type protectedArtifactReceipt struct {
	Purpose         ToolArtifactPurpose `json:"purpose,omitempty"`
	ReadablePath    string              `json:"readable_path"`
	ContentType     string              `json:"content_type,omitempty"`
	EstimatedBytes  int64               `json:"estimated_bytes,omitempty"`
	EstimatedTokens int                 `json:"estimated_tokens,omitempty"`
}

type protectedReceipt struct {
	MessageIndex        int                        `json:"message_index"`
	Tool                string                     `json:"tool,omitempty"`
	CallID              string                     `json:"call_id,omitempty"`
	Status              ToolResultStatus           `json:"status"`
	SyntheticReason     ToolSyntheticReason        `json:"synthetic_reason,omitempty"`
	ResultRetention     ToolResultRetentionMode    `json:"result_retention,omitempty"`
	SanitizedArguments  string                     `json:"sanitized_arguments,omitempty"`
	OutcomeReceipt      string                     `json:"outcome_receipt,omitempty"`
	ContextHints        *ToolResultContextHints    `json:"context_hints,omitempty"`
	ArtifactPersistence *ToolArtifactPersistence   `json:"artifact_persistence,omitempty"`
	Artifacts           []protectedArtifactReceipt `json:"artifacts,omitempty"`
}

func mergeProtectedReceiptContext(summary, previous string, removed []*Message, limit int) string {
	body, echoed := splitProtectedReceipts(summary)
	_, earlier := splitProtectedReceipts(previous)
	body = strings.TrimSpace(body)
	prefix := "\n\n" + protectedReceiptTitle + "\n"
	budget := min(protectedReceiptBytes, max(0, limit-len(body)-len(prefix)))
	block := mergeProtectedReceiptBlocks(budget, receiptsFromMessages(removed), echoed, earlier)
	if block == "" {
		return body
	}
	// Never truncate the semantic checkpoint to make room for receipts. An
	// oversized mandatory reference is rejected by the normal checkpoint guard.
	return body + prefix + block
}

func splitProtectedReceipts(value string) (string, string) {
	value = strings.TrimSpace(value)
	marker := "\n\n" + protectedReceiptTitle + "\n"
	index := strings.LastIndex(value, marker)
	markerBytes := len(marker)
	if index < 0 && strings.HasPrefix(value, protectedReceiptTitle+"\n") {
		index = 0
		markerBytes = len(protectedReceiptTitle) + 1
	}
	if index < 0 {
		return value, ""
	}
	block := strings.TrimSpace(value[index+markerBytes:])
	for _, line := range strings.Split(block, "\n") {
		if strings.TrimSpace(line) != "" && !json.Valid([]byte(line)) {
			return value, ""
		}
	}
	return strings.TrimSpace(value[:index]), block
}

func receiptsFromMessages(messages []*Message) string {
	type candidate struct {
		index      int
		unresolved bool
		line       string
	}
	var candidates []candidate
	for index, message := range messages {
		if message == nil || message.Role != ToolRole || message.ToolResult == nil {
			continue
		}
		result := message.ToolResult
		protected := result.ResultRetention == ToolResultProtected || result.Status != ToolResultSuccess ||
			result.ProtectedReceipt != nil || result.ArtifactPersistence != nil || len(result.Artifacts) > 0
		if !protected {
			continue
		}
		receipt := protectedReceipt{
			MessageIndex: index, Tool: boundedReceiptField(message.ToolName, 256), CallID: boundedReceiptField(message.ToolCallID, 256),
			Status: result.Status, SyntheticReason: result.SyntheticReason, ResultRetention: result.ResultRetention,
			ContextHints: result.ContextHints, ArtifactPersistence: result.ArtifactPersistence,
		}
		if result.ProtectedReceipt != nil {
			receipt.SanitizedArguments = boundedReceiptField(result.ProtectedReceipt.SanitizedArguments, 4*1024)
			receipt.OutcomeReceipt = boundedReceiptField(result.ProtectedReceipt.Outcome, 8*1024)
		}
		for _, artifact := range result.Artifacts {
			path := strings.TrimSpace(strings.ToValidUTF8(artifact.ReadablePath, "\uFFFD"))
			if !artifact.Complete || path == "" || ContainsSensitiveToolContextMaterial(path) {
				continue
			}
			receipt.Artifacts = append(receipt.Artifacts, protectedArtifactReceipt{
				Purpose: artifact.Purpose, ReadablePath: path,
				ContentType:    boundedReceiptField(artifact.ContentType, 256),
				EstimatedBytes: artifact.EstimatedBytes, EstimatedTokens: artifact.EstimatedTokens,
			})
		}
		encoded, err := json.Marshal(receipt)
		if err == nil && len(encoded) <= protectedReceiptBytes {
			candidates = append(candidates, candidate{index: index, unresolved: result.Status != ToolResultSuccess, line: string(encoded)})
		}
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		if candidates[left].unresolved != candidates[right].unresolved {
			return candidates[left].unresolved
		}
		return candidates[left].index > candidates[right].index
	})
	lines, total := make([]string, 0, min(len(candidates), protectedReceiptLimit)+1), 0
	for _, candidate := range candidates {
		if len(lines) >= protectedReceiptLimit || total+len(candidate.line)+1 > protectedReceiptBytes {
			continue
		}
		lines = append(lines, candidate.line)
		total += len(candidate.line) + 1
	}
	if omitted := len(candidates) - len(lines); omitted > 0 {
		note, _ := json.Marshal(map[string]any{"omitted_receipts": omitted, "selection": "unresolved_then_latest"})
		if total+len(note)+1 <= protectedReceiptBytes {
			lines = append(lines, string(note))
		}
	}
	return strings.Join(lines, "\n")
}

func mergeProtectedReceiptBlocks(limit int, blocks ...string) string {
	type candidate struct {
		line       string
		unresolved bool
	}
	var candidates []candidate
	seen, omitted := make(map[string]struct{}), 0
	for _, block := range blocks {
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || !json.Valid([]byte(line)) {
				continue
			}
			var metadata struct {
				Status          ToolResultStatus `json:"status"`
				OmittedReceipts int              `json:"omitted_receipts"`
			}
			if json.Unmarshal([]byte(line), &metadata) != nil {
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
			candidates = append(candidates, candidate{line: line, unresolved: metadata.Status != ToolResultSuccess})
		}
	}
	sort.SliceStable(candidates, func(left, right int) bool { return candidates[left].unresolved && !candidates[right].unresolved })
	lines, total := make([]string, 0, min(len(candidates), protectedReceiptLimit)+1), 0
	for _, candidate := range candidates {
		if len(lines) >= protectedReceiptLimit || total+len(candidate.line)+1 > max(0, limit-192) {
			omitted++
			continue
		}
		lines = append(lines, candidate.line)
		total += len(candidate.line) + 1
	}
	if omitted > 0 {
		note, _ := json.Marshal(map[string]any{"omitted_receipts": omitted, "selection": "unresolved_then_latest", "recovery": "Read the original tool results in the session journal for omitted receipts."})
		lines = append(lines, string(note))
	}
	return strings.Join(lines, "\n")
}

func boundedReceiptField(value string, limit int) string {
	value = strings.TrimSpace(strings.ToValidUTF8(value, "\uFFFD"))
	if value == "" || limit <= 0 || len(value) <= limit {
		return value
	}
	const marker = "...[truncated]"
	end := max(0, limit-len(marker))
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + marker
}
