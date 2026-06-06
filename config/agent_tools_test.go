package config

import "testing"

func TestResolveAgentToolsDefaults(t *testing.T) {
	ide := ResolveAgentTools(&Config{}, AgentKindIDE)
	if !ide.FileRead || !ide.FileWrite || !ide.ShellExecute || !ide.Skills || !ide.LoreRead || !ide.LoreWrite || !ide.Todo {
		t.Fatalf("IDE Agent 默认工具应全部开启: %+v", ide)
	}

	story := ResolveAgentTools(&Config{}, AgentKindInteractiveStory)
	if !story.FileRead || !story.FileWrite || !story.ShellExecute || !story.LoreRead {
		t.Fatalf("互动叙事 Agent 应保留当前文件/命令/资料读取能力: %+v", story)
	}
	if story.Skills || story.LoreWrite || story.Todo {
		t.Fatalf("互动叙事 Agent 默认不应启用 skills/资料写入/todo: %+v", story)
	}

	summary := ResolveAgentTools(&Config{}, AgentKindVersionSummary)
	if summary.FileRead || summary.FileWrite || summary.ShellExecute || summary.Skills || summary.LoreRead || summary.LoreWrite || summary.Todo {
		t.Fatalf("版本说明 Agent 默认不应注册工具: %+v", summary)
	}
}

func TestResolveAgentToolsPerAgentOverride(t *testing.T) {
	off := false
	on := true
	cfg := &Config{
		AgentTools: AgentToolSettings{
			Default: AgentToolOverride{ShellExecute: &off},
			IDE:     AgentToolOverride{ShellExecute: &on, LoreWrite: &off},
		},
	}

	ide := ResolveAgentTools(cfg, AgentKindIDE)
	if !ide.ShellExecute {
		t.Fatalf("IDE Agent 应覆盖 default 重新开启命令执行: %+v", ide)
	}
	if ide.LoreWrite {
		t.Fatalf("IDE Agent 应允许单独关闭资料库写入: %+v", ide)
	}

	story := ResolveAgentTools(cfg, AgentKindInteractiveStory)
	if story.ShellExecute {
		t.Fatalf("互动叙事 Agent 应继承 default 关闭命令执行: %+v", story)
	}
}
