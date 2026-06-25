package config

import "testing"

func TestResolveAgentToolsDefaults(t *testing.T) {
	ide := ResolveAgentTools(&Config{}, AgentKindIDE)
	if !ide.FileRead || !ide.FileWrite || !ide.ShellExecute || !ide.Skills || !ide.LoreRead || !ide.LoreWrite || !ide.Todo || !ide.WebSearch {
		t.Fatalf("IDE Agent 默认工具应全部开启: %+v", ide)
	}
	if ide.AgentConfigRead || ide.AgentConfigWrite {
		t.Fatalf("IDE Agent 默认不应启用 Agent 配置工具: %+v", ide)
	}

	story := ResolveAgentTools(&Config{}, AgentKindInteractiveStory)
	if !story.FileRead || !story.FileWrite || !story.ShellExecute || !story.LoreRead {
		t.Fatalf("互动叙事 Agent 应保留当前文件/命令/资料读取能力: %+v", story)
	}
	if !story.Skills {
		t.Fatalf("互动叙事 Agent 默认应启用 skills: %+v", story)
	}
	if story.LoreWrite || story.Todo || story.WebSearch {
		t.Fatalf("互动叙事 Agent 默认不应启用资料写入/todo/web search: %+v", story)
	}

	manager := ResolveAgentTools(&Config{}, AgentKindConfigManager)
	if !manager.FileRead || !manager.FileWrite || !manager.Skills || !manager.LoreRead || !manager.LoreWrite || !manager.Todo || !manager.WebSearch {
		t.Fatalf("配置管理 Agent 默认应启用常用资源管理工具: %+v", manager)
	}
	if manager.ShellExecute {
		t.Fatalf("配置管理 Agent 默认不应启用命令执行: %+v", manager)
	}
	if !manager.AgentConfigRead || !manager.AgentConfigWrite {
		t.Fatalf("配置管理 Agent 默认应启用 Agent 配置工具: %+v", manager)
	}

	summary := ResolveAgentTools(&Config{}, AgentKindVersionSummary)
	if summary.FileRead || summary.FileWrite || summary.ShellExecute || summary.Skills || summary.LoreRead || summary.LoreWrite || summary.Todo || summary.WebSearch || summary.AgentConfigRead || summary.AgentConfigWrite {
		t.Fatalf("版本说明 Agent 默认不应注册工具: %+v", summary)
	}
	toolAgent := ResolveAgentTools(&Config{}, AgentKindToolAgent)
	if toolAgent.FileRead || toolAgent.FileWrite || toolAgent.ShellExecute || toolAgent.Skills || toolAgent.LoreRead || toolAgent.LoreWrite || toolAgent.Todo || toolAgent.WebSearch || toolAgent.AgentConfigRead || toolAgent.AgentConfigWrite {
		t.Fatalf("工具 Agent 默认不应注册工具: %+v", toolAgent)
	}
	compaction := ResolveAgentTools(&Config{}, AgentKindContextCompaction)
	if compaction.FileRead || compaction.FileWrite || compaction.ShellExecute || compaction.Skills || compaction.LoreRead || compaction.LoreWrite || compaction.Todo || compaction.WebSearch || compaction.AgentConfigRead || compaction.AgentConfigWrite {
		t.Fatalf("上下文压缩 Agent 默认不应注册工具: %+v", compaction)
	}

	automation := ResolveAgentTools(&Config{}, AgentKindAutomation)
	if !automation.FileRead || !automation.FileWrite || !automation.Skills || !automation.LoreRead || !automation.LoreWrite || !automation.Todo || !automation.WebSearch {
		t.Fatalf("Automation Agent 默认应允许常用自动化工具: %+v", automation)
	}
	if automation.ShellExecute {
		t.Fatalf("Automation Agent 默认不应启用命令执行: %+v", automation)
	}
	if automation.AgentConfigRead || automation.AgentConfigWrite {
		t.Fatalf("Automation Agent 默认不应启用 Agent 配置工具: %+v", automation)
	}
}

func TestResolveAgentToolsPerAgentOverride(t *testing.T) {
	off := false
	on := true
	cfg := &Config{
		AgentTools: AgentToolSettings{
			Default: AgentToolOverride{ShellExecute: &off, WebSearch: &off},
			IDE:     AgentToolOverride{ShellExecute: &on, LoreWrite: &off, WebSearch: &on},
		},
	}

	ide := ResolveAgentTools(cfg, AgentKindIDE)
	if !ide.ShellExecute {
		t.Fatalf("IDE Agent 应覆盖 default 重新开启命令执行: %+v", ide)
	}
	if ide.LoreWrite {
		t.Fatalf("IDE Agent 应允许单独关闭资料库写入: %+v", ide)
	}
	if !ide.WebSearch {
		t.Fatalf("IDE Agent 应允许单独开启网页搜索: %+v", ide)
	}

	story := ResolveAgentTools(cfg, AgentKindInteractiveStory)
	if story.ShellExecute {
		t.Fatalf("互动叙事 Agent 应继承 default 关闭命令执行: %+v", story)
	}
	if story.WebSearch {
		t.Fatalf("互动叙事 Agent 应继承 default 关闭网页搜索: %+v", story)
	}
}

func TestResolveAgentToolsKeepsShellExecuteConfigurableOnWindows(t *testing.T) {
	on := true
	off := false
	cfg := &Config{
		AgentTools: AgentToolSettings{
			Default: AgentToolOverride{ShellExecute: &on},
			IDE:     AgentToolOverride{ShellExecute: &on},
		},
	}

	ide := resolveAgentToolsForGOOS(cfg, AgentKindIDE, "windows")
	if !ide.ShellExecute {
		t.Fatalf("Windows 支持 PowerShell execute 后应保留命令执行配置: %+v", ide)
	}
	if !ide.FileRead || !ide.FileWrite {
		t.Fatalf("Windows 平台不应影响其它工具: %+v", ide)
	}

	cfg.AgentTools.IDE.ShellExecute = &off
	ide = resolveAgentToolsForGOOS(cfg, AgentKindIDE, "windows")
	if ide.ShellExecute {
		t.Fatalf("Windows 下显式关闭命令执行仍应生效: %+v", ide)
	}
}
