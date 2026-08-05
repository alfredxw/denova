package app

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"unicode/utf8"

	"denova/config"
	agentmodeltask "denova/internal/agents/modeltask"
	"denova/internal/agents/session"
	"denova/internal/book"
)

const (
	versionSummaryMaxChanges      = 8
	versionSummaryMaxSnippetRunes = 900
	versionSummaryMaxPromptRunes  = 7000
)

type versionSummaryGeneratorFunc func(context.Context, *config.Config, string) (string, error)

func (a *App) setVersionSummaryGeneratorForTest(generator versionSummaryGeneratorFunc) func() {
	if a == nil {
		return func() {}
	}
	a.mu.Lock()
	previous := a.versionSummaryGenerator
	a.versionSummaryGenerator = generator
	a.mu.Unlock()
	return func() {
		a.mu.Lock()
		a.versionSummaryGenerator = previous
		a.mu.Unlock()
	}
}

func (a *App) versionSummaryGeneratorSnapshot() versionSummaryGeneratorFunc {
	if a == nil {
		return agentmodeltask.GenerateVersionSummary
	}
	a.mu.RLock()
	generator := a.versionSummaryGenerator
	a.mu.RUnlock()
	if generator == nil {
		return agentmodeltask.GenerateVersionSummary
	}
	return generator
}

func (s *workspaceService) inferVersionMessage(ctx context.Context, explicitMessage, source string, runtime *versionCreateRuntime) (string, error) {
	return s.app.inferVersionMessageForResources(ctx, explicitMessage, source, versionSummaryResources{
		workspace: runtime.workspace, cfg: &runtime.cfg, bookService: runtime.bookService,
		versionService: runtime.versionService, sessionStore: runtime.sessionStore, settings: runtime.settings,
	})
}

type versionSummaryResources struct {
	workspace      string
	cfg            *config.Config
	bookService    *book.Service
	versionService *book.VersionService
	sessionStore   *session.Store
	settings       book.VersionAutoSettings
}

func (a *App) inferVersionMessageForResources(ctx context.Context, explicitMessage, source string, runtime versionSummaryResources) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if message := strings.TrimSpace(explicitMessage); message != "" {
		return message, nil
	}
	status, err := runtime.versionService.Status(runtime.settings)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		slog.ErrorContext(ctx, fmt.Sprintf("[versions] 读取变更状态用于生成版本说明失败 source=%s err=%v", source, err))
		return fallbackVersionMessage(source, nil), nil
	}

	instruction := buildVersionSummaryInstruction(status, source, runtime.bookService, runtime.versionService)
	if instruction != "" {
		generator := a.versionSummaryGeneratorSnapshot()
		if summary, err := generator(ctx, runtime.cfg, instruction); err == nil && strings.TrimSpace(summary) != "" {
			persistAgentCallWithStore(runtime.sessionStore, config.AgentKindVersionSummary, instruction, summary)
			return strings.TrimSpace(summary), nil
		} else if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			slog.ErrorContext(ctx, fmt.Sprintf("[versions] LLM 生成版本说明失败 source=%s workspace=%s err=%v", source, runtime.workspace, err))
			persistAgentCallWithStore(runtime.sessionStore, config.AgentKindVersionSummary, instruction, "执行失败："+err.Error())
		}
	}
	return fallbackVersionMessage(source, status.Changes), nil
}

func buildVersionSummaryInstruction(status book.VersionStatus, source string, bookService *book.Service, versionService *book.VersionService) string {
	changes := append([]book.VersionChange(nil), status.Changes...)
	if len(changes) == 0 {
		return ""
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })

	var sb strings.Builder
	sb.WriteString("请根据以下 Denova 小说工程变更，推理这次版本保存说明。\n")
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
		book.VersionSourceTimer:          "自动版本",
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
		return "自动版本"
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
