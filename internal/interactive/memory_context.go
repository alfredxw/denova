package interactive

import (
	"strings"
	"unicode/utf8"
)

func (s *Store) StoryMemoryContextSummary(storyID, branchID string, limit int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return "", err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return "", err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return "", err
	}
	structures, source := s.storyMemoryStructuresForStoryLocked(meta, book)
	structures = runtimeStoryMemoryStructures(structures, source)
	records := visibleStoryMemoryRecords(book.Records, branchID, eventPathSet(branch.Head, lines), false)
	return formatStoryMemoryContextSummary(structures, records, limit), nil
}

func (s *Store) StoryMemoryCompactionContext(storyID, branchID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return "", err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return "", err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return "", err
	}
	structures, source := s.storyMemoryStructuresForStoryLocked(meta, book)
	structures = runtimeStoryMemoryStructures(structures, source)
	records := visibleStoryMemoryRecords(book.Records, branchID, eventPathSet(branch.Head, lines), false)
	return formatStoryMemoryCompactionContext(structures, records), nil
}

func (s *Store) StoryMemorySchemaContext(storyID string, limit int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return "", err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return "", err
	}
	structures, source := s.storyMemoryStructuresForStoryLocked(meta, book)
	structures = runtimeStoryMemoryStructures(structures, source)
	return formatStoryMemorySchemaContext(structures, limit), nil
}

func formatStoryMemoryContextSummary(structures []StoryMemoryStructure, records []StoryMemoryRecord, limit int) string {
	if limit <= 0 || limit > maxMemoryTextBytes {
		limit = maxMemoryTextBytes
	}
	return formatStoryMemoryContext(structures, records, limit, maxMemoryListItems, true)
}

func formatStoryMemoryCompactionContext(structures []StoryMemoryStructure, records []StoryMemoryRecord) string {
	return formatStoryMemoryContext(structures, records, 0, 0, false)
}

func formatStoryMemoryContext(structures []StoryMemoryStructure, records []StoryMemoryRecord, limit, itemLimit int, bounded bool) string {
	if len(enabledStoryMemoryStructures(structures)) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("来源: interactive/memory/story-{story_id}.json 的当前分支可见故事记忆\n")
	if bounded {
		sb.WriteString("边界: 已按调用方上下文预算裁剪\n")
	} else {
		sb.WriteString("边界: 不截断，用于上下文压缩 Agent\n")
	}
	count := 0
	for _, structure := range structures {
		if !storyMemoryStructureEnabled(structure) {
			continue
		}
		items := make([]StoryMemoryRecord, 0)
		for _, record := range records {
			if record.StructureID == structure.ID {
				items = append(items, record)
			}
		}
		if len(items) == 0 {
			continue
		}
		sb.WriteString("\n## ")
		sb.WriteString(structure.Name)
		sb.WriteString("\n")
		for _, record := range items {
			if bounded && (count >= itemLimit || sb.Len() >= limit) {
				sb.WriteString("\n(后续故事记忆已截断)\n")
				return limitMemoryText(sb.String(), limit)
			}
			if record.Key != "" {
				sb.WriteString("- ")
				sb.WriteString(record.Key)
				sb.WriteString(": ")
			} else {
				sb.WriteString("- ")
			}
			parts := make([]string, 0, len(structure.Fields))
			for _, field := range structure.Fields {
				if !storyMemoryFieldEnabled(field) {
					continue
				}
				if value := strings.TrimSpace(record.Values[field.ID]); value != "" {
					parts = append(parts, field.Name+"="+value)
				}
			}
			if len(parts) == 0 {
				for key, value := range record.Values {
					parts = append(parts, key+"="+value)
				}
			}
			sb.WriteString(strings.Join(parts, "；"))
			sb.WriteString("\n")
			count++
		}
	}
	if bounded {
		return limitMemoryText(sb.String(), limit)
	}
	return strings.TrimSpace(sb.String())
}

func formatStoryMemorySchemaContext(structures []StoryMemoryStructure, limit int) string {
	if limit <= 0 || limit > maxStoryMemorySchemaBytes {
		limit = maxStoryMemorySchemaBytes
	}
	structures = storyMemorySchemaContextOrder(structures)
	if len(structures) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("来源: interactive/memory/story-{story_id}.json 的故事记忆结构定义\n")
	sb.WriteString("边界: 已按调用方上下文预算裁剪\n")
	sb.WriteString("规则: story_memory_patches 只能使用下列 structure_id 和字段 ID；每条 patch 的 values 必须包含目标结构列出的所有字段，且字段值不能为空；keyed 结构必须提供 key，且 key 应等于 key_field_id 对应字段值；生成时必须遵守 structure 和 field 的 generation_instruction。\n")
	sb.WriteString("\n结构与字段 ID 索引（完整 ID 优先于详细说明）:\n")
	for _, structure := range structures {
		if !storyMemoryStructureEnabled(structure) {
			continue
		}
		sb.WriteString("\n## ")
		sb.WriteString(structure.ID)
		sb.WriteString("\n- mode: ")
		sb.WriteString(firstMemoryText(structure.Mode, "append"))
		if strings.TrimSpace(structure.KeyFieldID) != "" {
			sb.WriteString("; key_field_id: ")
			sb.WriteString(structure.KeyFieldID)
		}
		sb.WriteString("\n- field_ids: ")
		fieldIDs := make([]string, 0, len(structure.Fields))
		for _, field := range structure.Fields {
			if !storyMemoryFieldEnabled(field) {
				continue
			}
			fieldID := field.ID
			if strings.TrimSpace(field.Name) != "" {
				fieldID += "（" + field.Name + "）"
			}
			if field.Required {
				fieldID += " required"
			}
			fieldIDs = append(fieldIDs, fieldID)
		}
		sb.WriteString(strings.Join(fieldIDs, ", "))
		sb.WriteString("\n")
	}
	sb.WriteString("\n详细生成说明（按优先级在剩余预算内保留）:\n")
	for _, structure := range structures {
		if !storyMemoryStructureEnabled(structure) {
			continue
		}
		if sb.Len() >= limit {
			sb.WriteString("\n(后续故事记忆结构已截断)\n")
			return limitMemoryText(sb.String(), limit)
		}
		sb.WriteString("\n## ")
		sb.WriteString(structure.ID)
		if strings.TrimSpace(structure.Name) != "" {
			sb.WriteString("（")
			sb.WriteString(structure.Name)
			sb.WriteString("）")
		}
		sb.WriteString("\n")
		sb.WriteString("- mode: ")
		sb.WriteString(firstMemoryText(structure.Mode, "append"))
		sb.WriteString("\n")
		if strings.TrimSpace(structure.KeyFieldID) != "" {
			sb.WriteString("- key_field_id: ")
			sb.WriteString(structure.KeyFieldID)
			sb.WriteString("\n")
		}
		if strings.TrimSpace(structure.Description) != "" {
			sb.WriteString("- description: ")
			sb.WriteString(structure.Description)
			sb.WriteString("\n")
		}
		if strings.TrimSpace(structure.GenerationInstruction) != "" {
			sb.WriteString("- generation_instruction: ")
			sb.WriteString(structure.GenerationInstruction)
			sb.WriteString("\n")
		}
		sb.WriteString("- fields:\n")
		for _, field := range structure.Fields {
			if !storyMemoryFieldEnabled(field) {
				continue
			}
			if sb.Len() >= limit {
				sb.WriteString("(后续字段已截断)\n")
				return limitMemoryText(sb.String(), limit)
			}
			sb.WriteString("  - ")
			sb.WriteString(field.ID)
			if strings.TrimSpace(field.Name) != "" {
				sb.WriteString("（")
				sb.WriteString(field.Name)
				sb.WriteString("）")
			}
			if field.Required {
				sb.WriteString(" required")
			}
			if strings.TrimSpace(field.Description) != "" {
				sb.WriteString(": ")
				sb.WriteString(field.Description)
			}
			if strings.TrimSpace(field.GenerationInstruction) != "" {
				sb.WriteString("\n    generation_instruction: ")
				sb.WriteString(field.GenerationInstruction)
			}
			sb.WriteString("\n")
		}
	}
	return limitMemoryText(sb.String(), limit)
}

func storyMemorySchemaContextOrder(structures []StoryMemoryStructure) []StoryMemoryStructure {
	out := make([]StoryMemoryStructure, 0, len(structures))
	for _, structure := range structures {
		if !structure.BuiltIn {
			out = append(out, structure)
		}
	}
	for _, structure := range structures {
		if structure.BuiltIn {
			out = append(out, structure)
		}
	}
	return out
}

func limitMemoryText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if suffix := memoryTruncationSuffix(value); suffix != "" && len(suffix)+1 < limit {
		prefix := limitMemoryTextRaw(value, limit-len(suffix)-1)
		return strings.TrimSpace(prefix) + "\n" + suffix
	}
	return limitMemoryTextRaw(value, limit)
}

func limitMemoryTextRaw(value string, limit int) string {
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	if limit <= 0 {
		return ""
	}
	return strings.TrimSpace(value[:limit])
}

func memoryTruncationSuffix(value string) string {
	for _, suffix := range []string{
		"(后续故事记忆已截断)",
		"(后续故事记忆结构已截断)",
		"(后续字段已截断)",
	} {
		if strings.Contains(value, suffix) {
			return suffix
		}
	}
	return ""
}
