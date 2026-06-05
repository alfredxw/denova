package app

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"nova/config"
	"nova/internal/agent"
	"nova/internal/book"
)

const (
	versionSummaryMaxChanges      = 8
	versionSummaryMaxSnippetRunes = 900
	versionSummaryMaxPromptRunes  = 7000
)

func (s *WorkspaceRuntimeManager) inferVersionMessage(ctx context.Context, explicitMessage, source string, versionService *book.VersionService, settings book.VersionAutoSettings) string {
	if message := strings.TrimSpace(explicitMessage); message != "" {
		return message
	}
	status, err := versionService.Status(settings)
	if err != nil {
		log.Printf("[versions] 读取变更状态用于生成版本说明失败 source=%s err=%v", source, err)
		return fallbackVersionMessage(source, nil)
	}

	runtimeCfg, workspace := s.versionSummaryConfig()
	instruction := s.buildVersionSummaryInstruction(status, source)
	if instruction != "" {
		summaryCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if summary, err := agent.GenerateVersionSummary(summaryCtx, &runtimeCfg, instruction); err == nil && strings.TrimSpace(summary) != "" {
			return strings.TrimSpace(summary)
		} else if err != nil {
			log.Printf("[versions] LLM 生成版本说明失败 source=%s workspace=%s err=%v", source, workspace, err)
		}
	}
	return fallbackVersionMessage(source, status.Changes)
}

func (s *WorkspaceRuntimeManager) versionSummaryConfig() (config.Config, string) {
	a := s.app
	a.mu.RLock()
	var runtimeCfg config.Config
	if a.cfg != nil {
		runtimeCfg = *a.cfg
	}
	workspace := a.workspace
	novaDir := runtimeCfg.NovaDir
	a.mu.RUnlock()

	runtimeCfg.Workspace = workspace
	if layered, err := config.LoadLayered(novaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
	} else {
		log.Printf("[versions] 加载分层配置用于版本说明失败 workspace=%s err=%v", workspace, err)
	}
	return runtimeCfg, workspace
}

func (s *WorkspaceRuntimeManager) buildVersionSummaryInstruction(status book.VersionStatus, source string) string {
	changes := status.Changes
	if len(changes) == 0 {
		return ""
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })

	var sb strings.Builder
	sb.WriteString("请根据以下 Nova 小说工程变更，推理这次版本保存说明。\n")
	sb.WriteString("要求：只概括对创作内容或工程文件最关键的变化；不要逐文件罗列；不要提到 Git、diff、快照。\n")
	sb.WriteString(fmt.Sprintf("保存来源：%s\n", versionSourceLabel(source)))
	sb.WriteString(fmt.Sprintf("变更数量：%d\n", len(changes)))
	sb.WriteString("变更文件：\n")
	for i, change := range changes {
		if i >= versionSummaryMaxChanges {
			sb.WriteString(fmt.Sprintf("- 另有 %d 个文件变更\n", len(changes)-i))
			break
		}
		sb.WriteString(fmt.Sprintf("- %s %s\n", versionStatusLabel(change.Status), change.Path))
	}

	bookService := s.BookService()
	versionService := s.versionService()
	if bookService == nil || versionService == nil {
		return limitRunes(sb.String(), versionSummaryMaxPromptRunes)
	}

	sb.WriteString("\n变更内容摘要：\n")
	for i, change := range changes {
		if i >= versionSummaryMaxChanges {
			break
		}
		sb.WriteString(versionChangeContext(bookService, versionService, status.Latest, change))
	}
	return limitRunes(sb.String(), versionSummaryMaxPromptRunes)
}

func versionChangeContext(bookService *book.Service, versionService *book.VersionService, latest *book.VersionEntry, change book.VersionChange) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n### %s %s\n", versionStatusLabel(change.Status), change.Path))
	if latest != nil {
		diff, err := versionService.Diff(latest.ID, change.Path)
		if err == nil && diff.Text {
			if diff.Original != "" {
				sb.WriteString("旧内容片段：\n")
				sb.WriteString(limitRunes(diff.Original, versionSummaryMaxSnippetRunes))
				sb.WriteByte('\n')
			}
			if diff.Modified != "" {
				sb.WriteString("新内容片段：\n")
				sb.WriteString(limitRunes(diff.Modified, versionSummaryMaxSnippetRunes))
				sb.WriteByte('\n')
			}
			return sb.String()
		}
	}
	if change.Status == "deleted" {
		sb.WriteString("文件已删除。\n")
		return sb.String()
	}
	content, err := bookService.ReadFile(change.Path)
	if err != nil {
		sb.WriteString(fmt.Sprintf("读取文件失败：%v\n", err))
		return sb.String()
	}
	sb.WriteString("当前内容片段：\n")
	sb.WriteString(limitRunes(content, versionSummaryMaxSnippetRunes))
	sb.WriteByte('\n')
	return sb.String()
}

func fallbackVersionMessage(source string, changes []book.VersionChange) string {
	prefix := map[string]string{
		book.VersionSourceManual:         "手动保存",
		book.VersionSourceTimer:          "定时自动保存",
		book.VersionSourceAgent:          "Agent 自动保存",
		book.VersionSourceRollbackBackup: "回滚前备份",
	}[source]
	if prefix == "" {
		prefix = "保存版本"
	}
	if len(changes) == 0 {
		return prefix
	}
	counts := map[string]int{}
	paths := make([]string, 0, min(len(changes), 3))
	for _, change := range changes {
		counts[change.Status]++
		if len(paths) < 3 {
			paths = append(paths, change.Path)
		}
	}
	parts := []string{}
	if counts["added"] > 0 {
		parts = append(parts, fmt.Sprintf("新增%d个", counts["added"]))
	}
	if counts["modified"] > 0 {
		parts = append(parts, fmt.Sprintf("修改%d个", counts["modified"]))
	}
	if counts["deleted"] > 0 {
		parts = append(parts, fmt.Sprintf("删除%d个", counts["deleted"]))
	}
	if len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("更新%d个", len(changes)))
	}
	return fmt.Sprintf("%s：%s文件（%s）", prefix, strings.Join(parts, "、"), strings.Join(paths, "、"))
}

func versionSourceLabel(source string) string {
	switch source {
	case book.VersionSourceManual:
		return "手动保存"
	case book.VersionSourceTimer:
		return "定时自动保存"
	case book.VersionSourceAgent:
		return "Agent 自动保存"
	case book.VersionSourceRollbackBackup:
		return "回滚前备份"
	default:
		return "保存版本"
	}
}

func versionStatusLabel(status string) string {
	switch status {
	case "added":
		return "新增"
	case "modified":
		return "修改"
	case "deleted":
		return "删除"
	default:
		return status
	}
}

func limitRunes(value string, max int) string {
	if max <= 0 || utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max]) + "\n..."
}
