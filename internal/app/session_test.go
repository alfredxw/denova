package app

import (
	"testing"

	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/session"
)

func TestAppSwitchSessionUsesCurrentSessionHistoryOnly(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := &App{sessionStore: store}

	first, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Append(schema.UserMessage("会话 A 消息")); err != nil {
		t.Fatal(err)
	}
	app.session = first

	second, err := app.CreateSession("会话 B")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatal("新会话 ID 不应复用 default")
	}
	if err := second.Append(schema.UserMessage("会话 B 消息")); err != nil {
		t.Fatal(err)
	}

	history, err := app.SessionMessages("")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Content != "会话 B 消息" {
		t.Fatalf("当前历史应来自新会话: %#v", history)
	}

	if _, err := app.SwitchSession(first.ID); err != nil {
		t.Fatal(err)
	}
	history, err = app.SessionMessages("")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Content != "会话 A 消息" {
		t.Fatalf("切换后历史应来自目标会话: %#v", history)
	}
}

func TestAppDeleteActiveSessionSwitchesToRemainingSession(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	app := &App{sessionStore: store, session: first}
	second, err := app.CreateSession("会话 B")
	if err != nil {
		t.Fatal(err)
	}

	active, err := app.DeleteSession(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if active.ID != first.ID {
		t.Fatalf("删除当前会话后应切换到剩余会话: want=%s got=%s", first.ID, active.ID)
	}
	metas, err := app.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || !metas[0].Active || metas[0].ID != first.ID {
		t.Fatalf("剩余会话列表不符合预期: %#v", metas)
	}
}

func TestAppUserSessionsIgnoreFixedAgentSessions(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Append(schema.UserMessage("创作会话")); err != nil {
		t.Fatal(err)
	}
	app := &App{sessionStore: store, session: first}
	if err := persistAgentCallInStore(store, config.AgentKindConfigManager, "配置输入", "配置输出"); err != nil {
		t.Fatal(err)
	}
	scopedID, err := configManagerSessionID(ConfigManagerRequest{Origin: "automation", ResourceID: "daily-review"})
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := store.GetOrCreate(scopedID)
	if err != nil {
		t.Fatal(err)
	}
	if err := scoped.Append(schema.UserMessage("自动化配置会话")); err != nil {
		t.Fatal(err)
	}

	metas, err := app.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].ID != first.ID {
		t.Fatalf("创作会话列表不应包含固定 Agent 会话: %#v", metas)
	}
	if _, err := app.SwitchSession("config-manager-agent"); err == nil {
		t.Fatal("创作 Agent 不应允许切换到配置管理 Agent 固定会话")
	}
	if _, err := app.SessionMessages("config-manager-agent"); err == nil {
		t.Fatal("创作会话 API 不应读取配置管理 Agent 固定会话")
	}
	if err := app.RenameSession("config-manager-agent", "误改名"); err == nil {
		t.Fatal("创作会话 API 不应重命名配置管理 Agent 固定会话")
	}
	if _, err := app.DeleteSession("config-manager-agent"); err == nil {
		t.Fatal("创作会话 API 不应删除配置管理 Agent 固定会话")
	}
	if _, err := app.SwitchSession(scopedID); err == nil {
		t.Fatal("创作 Agent 不应允许切换到配置管理 Agent scoped 会话")
	}
	if _, err := app.SessionMessages(scopedID); err == nil {
		t.Fatal("创作会话 API 不应读取配置管理 Agent scoped 会话")
	}

	history, err := app.AgentSessionMessages(config.AgentKindConfigManager)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].Content != "配置输入" || history[1].Content != "配置输出" {
		t.Fatalf("配置管理 Agent 自己的会话应保持可读: %#v", history)
	}
}

func TestActiveUserSessionOrCreateIgnoresFixedAgentActiveSession(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	if err := persistAgentCallInStore(store, config.AgentKindConfigManager, "配置输入", "配置输出"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetActiveID("config-manager-agent"); err != nil {
		t.Fatal(err)
	}

	active, err := activeUserSessionOrCreate(store)
	if err != nil {
		t.Fatal(err)
	}
	if active.ID != first.ID {
		t.Fatalf("固定 Agent 会话不应恢复为创作 Agent 当前会话: got=%s want=%s", active.ID, first.ID)
	}
	activeID, err := store.ActiveID()
	if err != nil {
		t.Fatal(err)
	}
	if activeID != first.ID {
		t.Fatalf("active_id 应被修正回创作会话: got=%s want=%s", activeID, first.ID)
	}
}
