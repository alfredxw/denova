package agent

import (
	"fmt"
	"strings"
)

func buildInteractiveStoryInstructionContextParts(content string) []ContextAnalysisPart {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	dynamicStart := strings.Index(content, "[本轮动态上下文]")
	instruction := content
	dynamic := ""
	if dynamicStart >= 0 {
		instruction = strings.TrimSpace(content[:dynamicStart])
		dynamic = strings.TrimSpace(content[dynamicStart:])
	}
	parts := buildInteractiveTurnInstructionParts(instruction)
	parts = append(parts, buildInteractiveDynamicContextParts(dynamic)...)
	return parts
}

func buildInteractiveTurnInstructionParts(content string) []ContextAnalysisPart {
	const (
		actionPrefix        = "用户本回合行动：\n"
		directorRulesPrefix = "\n\n导演本轮上下文规则：\n"
		directorRulesEnd    = "\n\n以上导演规则必须显著影响"
		builtInPrefix       = "\n\n请基于互动故事上下文"
	)
	if !strings.Contains(content, "[互动输入]") {
		return nil
	}
	actionPrefixIndex := strings.Index(content, actionPrefix)
	if actionPrefixIndex < 0 {
		return nil
	}
	actionStart := actionPrefixIndex + len(actionPrefix)
	actionTail := content[actionStart:]
	directorOffset := strings.Index(actionTail, directorRulesPrefix)
	builtInOffset := strings.Index(actionTail, builtInPrefix)
	actionEnd := len(actionTail)
	if directorOffset >= 0 {
		actionEnd = directorOffset
	} else if builtInOffset >= 0 {
		actionEnd = builtInOffset
	}
	parts := []ContextAnalysisPart{
		NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      "interactive_instruction_action",
			Source:  "本轮行动",
			Title:   "当前用户行动",
			Role:    "user",
			Kind:    "body",
			Content: strings.TrimSpace(actionTail[:actionEnd]),
			Note:    "final_user_message",
		}),
	}
	builtInStart := -1
	if directorOffset >= 0 {
		rulesStart := actionStart + directorOffset + len(directorRulesPrefix)
		rulesTail := content[rulesStart:]
		rulesEnd := strings.Index(rulesTail, directorRulesEnd)
		if rulesEnd >= 0 {
			parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
				ID:      "interactive_instruction_director_rules",
				Source:  "StoryTeller.turn_context",
				Title:   "导演本轮上下文规则",
				Kind:    "body",
				Content: strings.TrimSpace(rulesTail[:rulesEnd]),
				Note:    "final_user_message",
			}))
			builtInStart = rulesStart + rulesEnd
		}
	} else if builtInOffset >= 0 {
		builtInStart = actionStart + builtInOffset
	}
	if builtInStart >= 0 {
		if builtIn := strings.TrimSpace(content[builtInStart:]); builtIn != "" {
			parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
				ID:      "interactive_instruction_runtime_rules",
				Source:  "Denova built-in",
				Title:   "互动回合执行规则",
				Kind:    "body",
				Content: builtIn,
				Note:    "final_user_message",
			}))
		}
	}
	return parts
}

type contextAnalysisHeadingSection struct {
	start        int
	contentStart int
	heading      string
}

func buildInteractiveDynamicContextParts(content string) []ContextAnalysisPart {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	content = strings.TrimSpace(strings.TrimPrefix(content, "[本轮动态上下文]"))
	sections := contextAnalysisHeadingSections(content, true)
	if len(sections) == 0 {
		sections = contextAnalysisHeadingSections(content, false)
	}
	parts := make([]ContextAnalysisPart, 0, len(sections)+1)
	preambleEnd := len(content)
	if len(sections) > 0 {
		preambleEnd = sections[0].start
	}
	if preamble := strings.TrimSpace(content[:preambleEnd]); preamble != "" {
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      "interactive_dynamic_rules",
			Source:  "Denova built-in",
			Title:   "动态上下文规则",
			Kind:    "body",
			Content: preamble,
			Note:    "final_user_message",
		}))
	}
	carriedNote := ""
	partIndex := 0
	for index, section := range sections {
		contentEnd := len(content)
		if index+1 < len(sections) {
			contentEnd = sections[index+1].start
		}
		title, source, note := interactiveDynamicContextHeadingMeta(section.heading)
		partContent := strings.TrimSpace(content[section.contentStart:contentEnd])
		note = joinContextAnalysisNotes(carriedNote, note)
		if partContent == "" && index+1 < len(sections) {
			nextTitle, nextSource, _ := interactiveDynamicContextHeadingMeta(sections[index+1].heading)
			if nextTitle == title && nextSource == source {
				carriedNote = note
				continue
			}
		}
		carriedNote = ""
		partIndex++
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      fmt.Sprintf("interactive_dynamic_part_%02d", partIndex),
			Source:  source,
			Title:   title,
			Kind:    "body",
			Content: partContent,
			Note:    joinContextAnalysisNotes(note, "final_user_message"),
		}))
	}
	return parts
}

func contextAnalysisHeadingSections(content string, requireSource bool) []contextAnalysisHeadingSection {
	sections := []contextAnalysisHeadingSection{}
	for offset := 0; offset < len(content); {
		lineEnd := strings.IndexByte(content[offset:], '\n')
		if lineEnd < 0 {
			lineEnd = len(content)
		} else {
			lineEnd += offset
		}
		line := strings.TrimSpace(content[offset:lineEnd])
		if strings.HasPrefix(line, "## ") && (!requireSource || strings.Contains(line, "source:")) {
			contentStart := lineEnd
			if contentStart < len(content) {
				contentStart++
			}
			sections = append(sections, contextAnalysisHeadingSection{
				start:        offset,
				contentStart: contentStart,
				heading:      strings.TrimSpace(strings.TrimPrefix(line, "## ")),
			})
		}
		if lineEnd >= len(content) {
			break
		}
		offset = lineEnd + 1
	}
	return sections
}

func interactiveDynamicContextHeadingMeta(heading string) (title, source, note string) {
	title = strings.TrimSpace(heading)
	source = "本轮动态上下文"
	rawSource := ""
	if before, after, ok := strings.Cut(title, "（source:"); ok {
		title = strings.TrimSpace(before)
		rawSource = strings.TrimSpace(strings.TrimSuffix(after, "）"))
	} else if before, after, ok := strings.Cut(title, "(source:"); ok {
		title = strings.TrimSpace(before)
		rawSource = strings.TrimSpace(strings.TrimSuffix(after, ")"))
	}
	if rawSource != "" {
		source = rawSource
		if sourceValue, qualifier, ok := strings.Cut(rawSource, ","); ok {
			source = strings.TrimSpace(sourceValue)
			note = strings.TrimSpace(qualifier)
		}
	}
	if title == "" {
		title = "动态上下文片段"
	}
	if source == "" {
		source = "本轮动态上下文"
	}
	return title, source, note
}

func joinContextAnalysisNotes(values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, " · ")
}
