package i18n

import (
	"fmt"
	"strings"
)

const (
	LocaleAuto = "auto"
	LocaleZH   = "zh-CN"
	LocaleEN   = "en-US"
)

type Localizer struct {
	locale string
}

func New(locale string) Localizer {
	return Localizer{locale: Resolve(locale)}
}

func FromHeader(value string) Localizer {
	return New(value)
}

func Resolve(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" || strings.EqualFold(normalized, LocaleAuto) {
		return LocaleZH
	}
	lower := strings.ToLower(normalized)
	if strings.HasPrefix(lower, "zh") {
		return LocaleZH
	}
	if strings.HasPrefix(lower, "en") {
		return LocaleEN
	}
	return LocaleZH
}

func (l Localizer) Locale() string {
	return l.locale
}

func (l Localizer) T(key string, args ...any) string {
	catalog := catalogZH
	if l.locale == LocaleEN {
		catalog = catalogEN
	}
	template, ok := catalog[key]
	if !ok {
		template = catalogZH[key]
	}
	if template == "" {
		return key
	}
	return format(template, args...)
}

func format(template string, args ...any) string {
	out := template
	for i := 0; i+1 < len(args); i += 2 {
		name, ok := args[i].(string)
		if !ok || name == "" {
			continue
		}
		out = strings.ReplaceAll(out, "{{"+name+"}}", stringify(args[i+1]))
	}
	return out
}

func stringify(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

var catalogZH = map[string]string{
	"api.common.invalidRequest":               "请求参数无效",
	"api.common.invalidRequestWithDetail":     "请求参数无效: {{detail}}",
	"api.common.invalidBody":                  "无效请求体",
	"api.common.messageRequired":              "消息不能为空",
	"api.common.pathRequired":                 "请提供 path 参数",
	"api.access.authRequired":                 "请先输入 Nova 远程访问用户名和密码",
	"api.access.lanDisabled":                  "当前未开启局域网访问",
	"api.books.titleRequired":                 "title 不能为空",
	"api.books.novaDirMissing":                "Nova 数据目录未配置",
	"api.books.removed":                       "已移除书籍记录",
	"api.books.reordered":                     "已保存书籍排序",
	"api.books.pathQueryRequired":             "path 参数不能为空",
	"api.books.pathRequired":                  "path 不能为空",
	"api.chat.noActiveTask":                   "没有活跃任务",
	"api.command.empty":                       "命令不能为空",
	"api.command.clearFailed":                 "清空失败: {{detail}}",
	"api.command.cleared":                     "上下文已清理，历史消息已保留",
	"api.command.compactFailed":               "上下文压缩失败: {{detail}}",
	"api.command.compacted":                   "上下文压缩完成，epoch {{epoch}}，估算 {{before}} → {{after}} tokens",
	"api.command.noStatus":                    "当前无作品状态数据，请先创建大纲",
	"api.command.unknown":                     "未知命令: {{command}}",
	"api.command.help":                        "可用命令:\n\n  plan    — 先规划再执行（/plan <需求描述>）\n  clear   — 清理当前 Agent 上下文并保留历史消息\n  compact — 主动压缩当前 Agent 上下文\n  status  — 显示当前作品状态\n  help    — 显示此帮助信息\n  /<skill-name> — 在支持 Skills 的 Agent 中加载指定 Skill，例如 /skills-creator\n\n在聊天中直接输入创作想法即可开始与 Nova 对话。",
	"api.skills.scopeNameRequired":            "请提供 scope 和 name 参数",
	"api.characterCard.parseFailed":           "解析酒馆角色卡失败: {{detail}}",
	"api.characterCard.uploadRequired":        "请上传 PNG 或 JSON 格式的酒馆角色卡文件",
	"api.characterCard.tooLarge":              "角色卡文件不能超过 32MB",
	"api.characterCard.readFailed":            "读取上传文件失败: {{detail}}",
	"api.characterCard.invalidTarget":         "导入目标无效",
	"api.characterCard.importFailed":          "导入酒馆角色卡失败: {{detail}}",
	"api.novelImport.parseFailed":             "解析小说文件失败: {{detail}}",
	"api.novelImport.uploadRequired":          "请上传 txt 或 md 格式的小说文件",
	"api.novelImport.tooLarge":                "小说文件不能超过 64MB",
	"api.novelImport.readFailed":              "读取上传文件失败: {{detail}}",
	"api.novelImport.importFailed":            "导入小说失败: {{detail}}",
	"api.novelImport.imported":                "小说导入完成",
	"api.novelImport.singleChapterWarning":    "未识别到明确章节标题，已作为单章导入",
	"api.novelImport.agentFallbackWarning":    "智能识别章节正则失败，已回退内置规则",
	"api.novelImport.regexFewChaptersWarning": "智能识别出的章节正则少于 2 章，已回退内置规则",
	"api.novelImport.regexFallbackWarning":    "智能识别出的章节正则不可用，已回退内置规则: {{detail}}",
	"api.interactive.storyIDRequired":         "故事 ID 不能为空",
	"api.interactive.storyModeOnly":           "当前仅支持 story 子模式",
	"api.interactive.tellerInstructionEmpty":  "叙事方案编辑指令不能为空",
	"api.lore.instructionEmpty":               "资料库编辑指令不能为空",
	"api.settings.revisionConflict":           "配置已被 Agent 或其他操作更新，请重新加载后再保存",
	"api.resource.revisionConflict":           "内容已被 Agent 或其他操作更新，请重新加载后再保存",
	"api.versions.invalidCreateRequest":       "版本保存请求格式不正确",
	"api.versions.idRequired":                 "请提供版本 ID",
	"api.workspace.scanFailed":                "扫描目录失败: {{detail}}",
	"api.workspace.summaryFailed":             "统计作品进度失败: {{detail}}",
	"api.workspace.chapterStatusPathRequired": "请提供章节 path",
	"api.workspace.chapterStatusFailed":       "更新章节状态失败: {{detail}}",
	"api.workspace.chapterStatusSaved":        "章节状态已更新",
	"api.workspace.pathMissing":               "缺少 path 参数",
	"api.workspace.limitInvalid":              "limit 必须是非负整数",
	"api.workspace.searchFailed":              "搜索失败: {{detail}}",
	"api.workspace.pathContentRequired":       "请提供 path 和 content 参数",
	"api.workspace.writeFailed":               "写入文件失败: {{detail}}",
	"api.workspace.fileRevisionConflict":      "文件已被 Agent 或其他操作更新，请重新加载后再保存",
	"api.workspace.fileSaved":                 "文件已保存",
	"api.workspace.pathTypeRequired":          "请提供 path 和 type 参数",
	"api.workspace.targetExists":              "目标已存在",
	"api.workspace.created":                   "创建成功",
	"api.workspace.deleted":                   "删除成功",
	"api.workspace.deleteFailed":              "删除失败: {{detail}}",
	"api.workspace.pathNewNameRequired":       "请提供 path 和 new_name 参数",
	"api.workspace.renamed":                   "重命名成功",
	"api.workspace.fromToRequired":            "请提供 from 和 to 参数",
	"api.workspace.copyFailed":                "复制失败: {{detail}}",
	"api.workspace.copied":                    "复制成功",
	"api.workspace.moveFailed":                "移动失败: {{detail}}",
	"api.workspace.moved":                     "移动成功",
	"api.workspace.switched":                  "已切换到: {{workspace}}",
	"api.workspace.noWorkspace":               "尚未选择书籍工作区，请先在书籍管理页选择或创建书籍",
	"api.settings.workspaceMissing":           "当前没有打开的工作区",
	"api.settings.lanUsernameRequired":        "开启局域网访问时必须设置用户名",
	"api.settings.lanPasswordRequired":        "开启局域网访问时必须设置密码",
}

var catalogEN = map[string]string{
	"api.common.invalidRequest":               "Invalid request.",
	"api.common.invalidRequestWithDetail":     "Invalid request: {{detail}}",
	"api.common.invalidBody":                  "Invalid request body.",
	"api.common.messageRequired":              "Message is required.",
	"api.common.pathRequired":                 "Provide the path parameter.",
	"api.access.authRequired":                 "Enter the Nova remote access username and password first.",
	"api.access.lanDisabled":                  "LAN access is not enabled.",
	"api.books.titleRequired":                 "Title is required.",
	"api.books.novaDirMissing":                "Nova data directory is not configured.",
	"api.books.removed":                       "Book record removed.",
	"api.books.reordered":                     "Book order saved.",
	"api.books.pathQueryRequired":             "Path query parameter is required.",
	"api.books.pathRequired":                  "Path is required.",
	"api.chat.noActiveTask":                   "No active task.",
	"api.command.empty":                       "Command is required.",
	"api.command.clearFailed":                 "Clear failed: {{detail}}",
	"api.command.cleared":                     "Context cleared. History messages are preserved.",
	"api.command.compactFailed":               "Context compaction failed: {{detail}}",
	"api.command.compacted":                   "Context compacted. epoch {{epoch}}, estimated {{before}} -> {{after}} tokens.",
	"api.command.noStatus":                    "No story state data yet. Create an outline first.",
	"api.command.unknown":                     "Unknown command: {{command}}",
	"api.command.help":                        "Available commands:\n\n  plan    - Plan before execution (/plan <request>)\n  clear   - Clear the current Agent context while keeping history\n  compact - Manually compact the current Agent context\n  status  - Show the current story state\n  help    - Show this help\n  /<skill-name> - Load a Skill in Agents that support Skills, for example /skills-creator\n\nType your writing idea in chat to start working with Nova.",
	"api.skills.scopeNameRequired":            "Provide scope and name.",
	"api.characterCard.parseFailed":           "Failed to parse Tavern character card: {{detail}}",
	"api.characterCard.uploadRequired":        "Upload a PNG or JSON Tavern character card file.",
	"api.characterCard.tooLarge":              "Character card file must be 32MB or smaller.",
	"api.characterCard.readFailed":            "Failed to read uploaded file: {{detail}}",
	"api.characterCard.invalidTarget":         "Invalid import target.",
	"api.characterCard.importFailed":          "Failed to import Tavern character card: {{detail}}",
	"api.novelImport.parseFailed":             "Failed to parse novel file: {{detail}}",
	"api.novelImport.uploadRequired":          "Upload a txt or md novel file.",
	"api.novelImport.tooLarge":                "Novel file must be 64MB or smaller.",
	"api.novelImport.readFailed":              "Failed to read uploaded file: {{detail}}",
	"api.novelImport.importFailed":            "Failed to import novel: {{detail}}",
	"api.novelImport.imported":                "Novel import complete.",
	"api.novelImport.singleChapterWarning":    "No clear chapter title was detected. The file will be imported as one chapter.",
	"api.novelImport.agentFallbackWarning":    "Smart chapter regex detection failed. Built-in rules were used instead.",
	"api.novelImport.regexFewChaptersWarning": "The smart chapter regex found fewer than 2 chapters. Built-in rules were used instead.",
	"api.novelImport.regexFallbackWarning":    "The smart chapter regex could not be used. Built-in rules were used instead: {{detail}}",
	"api.interactive.storyIDRequired":         "Story ID is required.",
	"api.interactive.storyModeOnly":           "Only the story submode is supported now.",
	"api.interactive.tellerInstructionEmpty":  "Narrative direction edit instruction is required.",
	"api.lore.instructionEmpty":               "Lore edit instruction is required.",
	"api.settings.revisionConflict":           "Settings were updated by the Agent or another operation. Reload before saving.",
	"api.resource.revisionConflict":           "This content was updated by the Agent or another operation. Reload before saving.",
	"api.versions.invalidCreateRequest":       "Invalid version save request.",
	"api.versions.idRequired":                 "Version ID is required.",
	"api.workspace.scanFailed":                "Failed to scan the directory: {{detail}}",
	"api.workspace.summaryFailed":             "Failed to calculate writing progress: {{detail}}",
	"api.workspace.chapterStatusPathRequired": "Provide a chapter path.",
	"api.workspace.chapterStatusFailed":       "Failed to update chapter status: {{detail}}",
	"api.workspace.chapterStatusSaved":        "Chapter status updated.",
	"api.workspace.pathMissing":               "Missing path parameter.",
	"api.workspace.limitInvalid":              "limit must be a non-negative integer.",
	"api.workspace.searchFailed":              "Search failed: {{detail}}",
	"api.workspace.pathContentRequired":       "Provide path and content.",
	"api.workspace.writeFailed":               "Failed to write file: {{detail}}",
	"api.workspace.fileRevisionConflict":      "The file was updated by the Agent or another operation. Reload it before saving.",
	"api.workspace.fileSaved":                 "File saved.",
	"api.workspace.pathTypeRequired":          "Provide path and type.",
	"api.workspace.targetExists":              "Target already exists.",
	"api.workspace.created":                   "Created.",
	"api.workspace.deleted":                   "Deleted.",
	"api.workspace.deleteFailed":              "Delete failed: {{detail}}",
	"api.workspace.pathNewNameRequired":       "Provide path and new_name.",
	"api.workspace.renamed":                   "Renamed.",
	"api.workspace.fromToRequired":            "Provide from and to.",
	"api.workspace.copyFailed":                "Copy failed: {{detail}}",
	"api.workspace.copied":                    "Copied.",
	"api.workspace.moveFailed":                "Move failed: {{detail}}",
	"api.workspace.moved":                     "Moved.",
	"api.workspace.switched":                  "Switched to: {{workspace}}",
	"api.workspace.noWorkspace":               "No book workspace is selected. Choose or create a book in Book Management first.",
	"api.settings.workspaceMissing":           "No workspace is open.",
	"api.settings.lanUsernameRequired":        "Set a username before enabling LAN access.",
	"api.settings.lanPasswordRequired":        "Set a password before enabling LAN access.",
}
