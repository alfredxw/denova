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
	lang := ""
	if runtime.cfg != nil {
		lang = runtime.cfg.Language
	}
	status, err := runtime.versionService.Status(ctx, runtime.settings)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		slog.ErrorContext(ctx, fmt.Sprintf("[versions] Failed to read changes for version summary source=%s err=%v", source, err))
		return fallbackVersionMessage(source, nil, lang), nil
	}

	instruction := buildVersionSummaryInstruction(status, source, runtime.bookService, runtime.versionService, lang)
	if instruction != "" {
		generator := a.versionSummaryGeneratorSnapshot()
		if summary, err := generator(ctx, runtime.cfg, instruction); err == nil && strings.TrimSpace(summary) != "" {
			persistAgentCallWithStore(runtime.sessionStore, config.AgentKindVersionSummary, instruction, summary)
			return strings.TrimSpace(summary), nil
		} else if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			slog.ErrorContext(ctx, fmt.Sprintf("[versions] LLM version summary generation failed source=%s workspace=%s err=%v", source, runtime.workspace, err))
			persistAgentCallWithStore(runtime.sessionStore, config.AgentKindVersionSummary, instruction, "LLM failed: "+err.Error())
		}
	}
	return fallbackVersionMessage(source, status.Changes, lang), nil
}

func buildVersionSummaryInstruction(status book.VersionStatus, source string, bookService *book.Service, versionService *book.VersionService, lang string) string {
	changes := append([]book.VersionChange(nil), status.Changes...)
	if len(changes) == 0 {
		return ""
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })

	var sb strings.Builder
	switch lang {
	case "ru-RU":
		sb.WriteString("Сделайте вывод о сути сохранённой версии романа Denova на основе следующих изменений.\n")
		sb.WriteString("Требования: обобщите только ключевые изменения в сюжете, персонажах, лоре или файлах проекта; не перечисляйте файлы по отдельности; не упоминайте Git, diff или снимки состояния.\n")
		sb.WriteString(fmt.Sprintf("Источник сохранения: %s\n", versionSourceLabel(source, lang)))
		sb.WriteString(fmt.Sprintf("Количество изменений: %d\n", len(changes)))
		sb.WriteString("Изменённые файлы:\n")
		for i, change := range changes {
			if i >= versionSummaryMaxChanges {
				sb.WriteString(fmt.Sprintf("- ещё %d изменённых файлов\n", len(changes)-i))
				break
			}
			sb.WriteString(fmt.Sprintf("- %s %s\n", versionStatusLabel(change.Status, lang), change.Path))
		}
		if bookService == nil || versionService == nil {
			return limitRunes(sb.String(), versionSummaryMaxPromptRunes)
		}
		sb.WriteString("\nКраткое содержание изменений:\n")
		for i, change := range changes {
			if i >= versionSummaryMaxChanges {
				break
			}
			sb.WriteString(versionChangeContext(bookService, versionService, status.Latest, change, lang))
		}
	case "en-US":
		sb.WriteString("Infer the version summary from the following Denova novel project changes.\n")
		sb.WriteString("Requirements: summarize only the most critical creative or project file changes; do not list files individually; do not mention Git, diff, or snapshots.\n")
		sb.WriteString(fmt.Sprintf("Save source: %s\n", versionSourceLabel(source, lang)))
		sb.WriteString(fmt.Sprintf("Change count: %d\n", len(changes)))
		sb.WriteString("Changed files:\n")
		for i, change := range changes {
			if i >= versionSummaryMaxChanges {
				sb.WriteString(fmt.Sprintf("- and %d more changed files\n", len(changes)-i))
				break
			}
			sb.WriteString(fmt.Sprintf("- %s %s\n", versionStatusLabel(change.Status, lang), change.Path))
		}
		if bookService == nil || versionService == nil {
			return limitRunes(sb.String(), versionSummaryMaxPromptRunes)
		}
		sb.WriteString("\nChange content summary:\n")
		for i, change := range changes {
			if i >= versionSummaryMaxChanges {
				break
			}
			sb.WriteString(versionChangeContext(bookService, versionService, status.Latest, change, lang))
		}
	default:
		sb.WriteString("请根据以下 Denova 小说工程变更，推理这次版本保存说明。\n")
		sb.WriteString("要求：只概括对创作内容或工程文件最关键的变化；不要逐文件罗列；不要提到 Git、diff、快照。\n")
		sb.WriteString(fmt.Sprintf("保存来源：%s\n", versionSourceLabel(source, lang)))
		sb.WriteString(fmt.Sprintf("变更数量：%d\n", len(changes)))
		sb.WriteString("变更文件：\n")
		for i, change := range changes {
			if i >= versionSummaryMaxChanges {
				sb.WriteString(fmt.Sprintf("- 另有 %d 个文件变更\n", len(changes)-i))
				break
			}
			sb.WriteString(fmt.Sprintf("- %s %s\n", versionStatusLabel(change.Status, lang), change.Path))
		}
		if bookService == nil || versionService == nil {
			return limitRunes(sb.String(), versionSummaryMaxPromptRunes)
		}
		sb.WriteString("\n变更内容摘要：\n")
		for i, change := range changes {
			if i >= versionSummaryMaxChanges {
				break
			}
			sb.WriteString(versionChangeContext(bookService, versionService, status.Latest, change, lang))
		}
	}
	return limitRunes(sb.String(), versionSummaryMaxPromptRunes)
}

func versionChangeContext(bookService *book.Service, versionService *book.VersionService, latest *book.VersionEntry, change book.VersionChange, lang string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n### %s %s\n", versionStatusLabel(change.Status, lang), change.Path))
	wroteText := false
	if latest != nil && change.Status != "added" {
		content, err := versionService.ReadFileAtVersion(latest.ID, change.Path)
		text := string(content)
		if err == nil && utf8.Valid(content) && !strings.ContainsRune(text, '\x00') {
			switch lang {
			case "ru-RU":
				sb.WriteString("Фрагмент старого содержимого:\n")
			case "en-US":
				sb.WriteString("Old content snippet:\n")
			default:
				sb.WriteString("旧内容片段：\n")
			}
			sb.WriteString(limitRunes(text, versionSummaryMaxSnippetRunes))
			sb.WriteByte('\n')
		wroteText = true
		}
	}
	if change.Status == "deleted" {
		if !wroteText {
			switch lang {
			case "ru-RU":
				sb.WriteString("Файл удалён.\n")
			case "en-US":
				sb.WriteString("File deleted.\n")
			default:
				sb.WriteString("文件已删除。\n")
			}
		}
		return sb.String()
	}
	content, err := bookService.ReadFile(change.Path)
	if err != nil {
		switch lang {
		case "ru-RU":
			sb.WriteString(fmt.Sprintf("Не удалось прочитать файл: %v\n", err))
		case "en-US":
			sb.WriteString(fmt.Sprintf("Failed to read file: %v\n", err))
		default:
			sb.WriteString(fmt.Sprintf("读取文件失败：%v\n", err))
		}
		return sb.String()
	}
	if utf8.ValidString(content) && !strings.ContainsRune(content, '\x00') {
		switch lang {
		case "ru-RU":
			sb.WriteString("Фрагмент нового содержимого:\n")
		case "en-US":
			sb.WriteString("New content snippet:\n")
		default:
			sb.WriteString("新内容片段：\n")
		}
		sb.WriteString(limitRunes(content, versionSummaryMaxSnippetRunes))
		sb.WriteByte('\n')
	} else if !wroteText {
		switch lang {
		case "ru-RU":
			sb.WriteString("Файл не содержит текста для аннотации.\n")
		case "en-US":
			sb.WriteString("File is not summarizable text content.\n")
		default:
			sb.WriteString("文件不是可摘要的文本内容。\n")
		}
	}
	return sb.String()
}

func fallbackVersionMessage(source string, changes []book.VersionChange, lang string) string {
	switch lang {
	case "ru-RU":
		prefix := map[string]string{
			book.VersionSourceManual:         "Ручное сохранение",
			book.VersionSourceTimer:          "Автосохранение",
			book.VersionSourceAgent:          "Автосохранение ассистентом",
			book.VersionSourceRollbackBackup: "Резервная копия перед откатом",
		}[source]
		if prefix == "" {
			prefix = "Сохранение версии"
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
			parts = append(parts, fmt.Sprintf("добавлено %d", counts["added"]))
		}
		if counts["modified"] > 0 {
			parts = append(parts, fmt.Sprintf("изменено %d", counts["modified"]))
		}
		if counts["deleted"] > 0 {
			parts = append(parts, fmt.Sprintf("удалено %d", counts["deleted"]))
		}
		if len(parts) == 0 {
			parts = append(parts, fmt.Sprintf("обновлено %d", len(changes)))
		}
		return fmt.Sprintf("%s: %s (файлы: %s)", prefix, strings.Join(parts, ", "), strings.Join(paths, ", "))
	case "en-US":
		prefix := map[string]string{
			book.VersionSourceManual:         "Manual save",
			book.VersionSourceTimer:          "Auto save",
			book.VersionSourceAgent:          "Agent auto save",
			book.VersionSourceRollbackBackup: "Backup before rollback",
		}[source]
		if prefix == "" {
			prefix = "Save version"
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
			parts = append(parts, fmt.Sprintf("added %d", counts["added"]))
		}
		if counts["modified"] > 0 {
			parts = append(parts, fmt.Sprintf("modified %d", counts["modified"]))
		}
		if counts["deleted"] > 0 {
			parts = append(parts, fmt.Sprintf("deleted %d", counts["deleted"]))
		}
		if len(parts) == 0 {
			parts = append(parts, fmt.Sprintf("updated %d", len(changes)))
		}
		return fmt.Sprintf("%s: %s (files: %s)", prefix, strings.Join(parts, ", "), strings.Join(paths, ", "))
	default:
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
}

func versionSourceLabel(source string, lang string) string {
	switch lang {
	case "ru-RU":
		switch source {
		case book.VersionSourceManual:
			return "Ручное сохранение"
		case book.VersionSourceTimer:
			return "Автосохранение"
		case book.VersionSourceAgent:
			return "Автосохранение ассистентом"
		case book.VersionSourceRollbackBackup:
			return "Резервная копия перед откатом"
		default:
			return "Сохранение версии"
		}
	case "en-US":
		switch source {
		case book.VersionSourceManual:
			return "Manual save"
		case book.VersionSourceTimer:
			return "Auto save"
		case book.VersionSourceAgent:
			return "Agent auto save"
		case book.VersionSourceRollbackBackup:
			return "Backup before rollback"
		default:
			return "Save version"
		}
	default:
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
}

func versionStatusLabel(status string, lang string) string {
	switch lang {
	case "ru-RU":
		switch status {
		case "added":
			return "добавлено"
		case "modified":
			return "изменено"
		case "deleted":
			return "удалено"
		default:
			return status
		}
	case "en-US":
		switch status {
		case "added":
			return "added"
		case "modified":
			return "modified"
		case "deleted":
			return "deleted"
		default:
			return status
		}
	default:
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
}

func limitRunes(value string, max int) string {
	if max <= 0 || utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max]) + "\n..."
}
