package book

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	semanticLoreClassificationMaxBytes  = 64 * 1024
	semanticLoreClassificationBodyBytes = 2 * 1024
)

func applySemanticTavernLoreClassification(ops []LoreOperation, stats *tavernImportStats, classifier LoreSemanticClassifier) error {
	if stats == nil || classifier == nil || len(stats.UncertainOpIndexes) == 0 {
		return nil
	}
	inputs := make([]LoreClassificationInput, 0, len(stats.UncertainOpIndexes))
	indexByToken := map[string]int{}
	usedBytes := 2
	for _, opIndex := range stats.UncertainOpIndexes {
		if opIndex < 0 || opIndex >= len(ops) {
			continue
		}
		item := ops[opIndex].Item
		token := fmt.Sprintf("entry-%d", opIndex)
		input := LoreClassificationInput{
			ID:               token,
			Name:             item.Name,
			Tags:             append([]string(nil), item.Tags...),
			Keywords:         append([]string(nil), item.Keywords...),
			BriefDescription: item.BriefDescription,
			Content:          truncateStringBytes(item.Content, semanticLoreClassificationBodyBytes),
			CurrentType:      item.Type,
		}
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		if usedBytes+len(encoded)+1 > semanticLoreClassificationMaxBytes {
			break
		}
		usedBytes += len(encoded) + 1
		inputs = append(inputs, input)
		indexByToken[token] = opIndex
	}
	if len(inputs) == 0 {
		return nil
	}
	if len(inputs) < len(stats.UncertainOpIndexes) {
		stats.Warnings = append(stats.Warnings, fmt.Sprintf("语义分类输入达到 64 KiB 上限；其余 %d 条保留本地分类结果", len(stats.UncertainOpIndexes)-len(inputs)))
	}
	suggestions, err := classifier(inputs)
	if err != nil {
		return err
	}
	recognized := 0
	seen := map[string]bool{}
	for _, suggestion := range suggestions {
		token := strings.TrimSpace(suggestion.ID)
		opIndex, ok := indexByToken[token]
		if !ok || seen[token] || !validLoreClassificationType(suggestion.Type) {
			continue
		}
		seen[token] = true
		recognized++
		if suggestion.Confidence != LoreClassificationConfidenceHigh && suggestion.Confidence != LoreClassificationConfidenceMedium {
			continue
		}
		item := &ops[opIndex].Item
		item.Type = strings.TrimSpace(suggestion.Type)
		item.TypeSource = LoreTypeSourceSemantic
		item.BriefDescription = tavernLoreSearchBrief(item.Type, item.Name, item.Keywords)
		stats.UncertainTypeCount--
	}
	if recognized == 0 {
		return fmt.Errorf("语义分类没有返回可应用的条目")
	}
	if stats.UncertainTypeCount < 0 {
		stats.UncertainTypeCount = 0
	}
	stats.ClassificationCounts = tavernWorldbookTypeCounts(ops)
	return nil
}

func tavernWorldbookTypeCounts(ops []LoreOperation) map[string]int {
	counts := map[string]int{}
	for _, op := range ops {
		if op.Item.Provenance == nil || op.Item.Provenance.Kind != "tavern_worldbook_entry" {
			continue
		}
		counts[normalizeLoreType(op.Item.Type)]++
	}
	return counts
}

func cloneLoreTypeCounts(value map[string]int) map[string]int {
	result := make(map[string]int, len(value))
	for key, count := range value {
		result[key] = count
	}
	return result
}
