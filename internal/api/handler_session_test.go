package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/hertz/pkg/common/ut"

	"nova/config"
	runtimeapp "nova/internal/app"
)

type testMessageDTO struct {
	Type      string `json:"type"`
	ID        string `json:"id,omitempty"`
	Role      string `json:"role,omitempty"`
	Content   string `json:"content,omitempty"`
	Name      string `json:"name,omitempty"`
	Status    string `json:"status,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type testSessionDTO struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	Active       bool   `json:"active"`
	MessageCount int    `json:"message_count"`
}

func TestSessionAPICRUDSwitchAndMessages(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	defaultID := application.Session().ID

	if err := application.Session().Append(schema.UserMessage("默认会话消息")); err != nil {
		t.Fatal(err)
	}

	listResp := performJSONRequest(t, server, http.MethodGet, "/api/sessions", nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", listResp.Code, listResp.Body.String())
	}
	var listBody struct {
		Sessions []testSessionDTO `json:"sessions"`
	}
	decodeResponse(t, listResp.Body.Bytes(), &listBody)
	if len(listBody.Sessions) != 1 || listBody.Sessions[0].ID != defaultID || !listBody.Sessions[0].Active {
		t.Fatalf("初始会话列表不符合预期: %#v", listBody.Sessions)
	}

	createResp := performJSONRequest(t, server, http.MethodPost, "/api/sessions", map[string]string{"title": "会话 B"})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	var created testSessionDTO
	decodeResponse(t, createResp.Body.Bytes(), &created)
	if created.ID == "" || created.ID == defaultID || !created.Active || created.Title != "会话 B" {
		t.Fatalf("创建会话返回不符合预期: %#v", created)
	}
	if err := application.Session().Append(schema.UserMessage("会话 B 消息")); err != nil {
		t.Fatal(err)
	}

	currentMessages := performJSONRequest(t, server, http.MethodGet, "/api/session/messages", nil)
	var current []testMessageDTO
	decodeResponse(t, currentMessages.Body.Bytes(), &current)
	if len(current) != 1 || current[0].Content != "会话 B 消息" {
		t.Fatalf("当前会话消息应来自新会话: %#v", current)
	}

	switchResp := performJSONRequest(t, server, http.MethodPost, "/api/sessions/switch", map[string]string{"id": defaultID})
	if switchResp.Code != http.StatusOK {
		t.Fatalf("switch status = %d body=%s", switchResp.Code, switchResp.Body.String())
	}
	defaultMessages := performJSONRequest(t, server, http.MethodGet, "/api/session/messages?session_id="+defaultID, nil)
	var defaultHistory []testMessageDTO
	decodeResponse(t, defaultMessages.Body.Bytes(), &defaultHistory)
	if len(defaultHistory) != 1 || defaultHistory[0].Content != "默认会话消息" {
		t.Fatalf("指定会话消息读取不符合预期: %#v", defaultHistory)
	}

	renameResp := performJSONRequest(t, server, http.MethodPost, "/api/sessions/rename", map[string]string{"id": created.ID, "title": "新标题"})
	if renameResp.Code != http.StatusOK {
		t.Fatalf("rename status = %d body=%s", renameResp.Code, renameResp.Body.String())
	}
	listResp = performJSONRequest(t, server, http.MethodGet, "/api/sessions", nil)
	decodeResponse(t, listResp.Body.Bytes(), &listBody)
	if !containsSessionTitle(listBody.Sessions, created.ID, "新标题") {
		t.Fatalf("重命名后的会话列表不符合预期: %#v", listBody.Sessions)
	}

	clearResp := performJSONRequest(t, server, http.MethodPost, "/api/command", map[string]string{"command": "clear"})
	if clearResp.Code != http.StatusOK {
		t.Fatalf("clear status = %d body=%s", clearResp.Code, clearResp.Body.String())
	}
	clearedResp := performJSONRequest(t, server, http.MethodGet, "/api/session/messages", nil)
	var cleared []testMessageDTO
	decodeResponse(t, clearedResp.Body.Bytes(), &cleared)
	if len(cleared) != 2 || cleared[1].Type != "clear" {
		t.Fatalf("/clear 后应保留历史并追加 clear 标记: %#v", cleared)
	}

	deleteResp := performJSONRequest(t, server, http.MethodPost, "/api/sessions/delete", map[string]string{"id": created.ID})
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", deleteResp.Code, deleteResp.Body.String())
	}
	var active testSessionDTO
	decodeResponse(t, deleteResp.Body.Bytes(), &active)
	if active.ID != defaultID || !active.Active {
		t.Fatalf("删除非唯一会话后应保留默认会话激活: %#v", active)
	}
}

func newTestApplication(t *testing.T) *runtimeapp.App {
	t.Helper()
	root := t.TempDir()
	application, err := runtimeapp.New(context.Background(), &config.Config{
		OpenAIModel:         "test-model",
		NovaDir:             root,
		Workspace:           root,
		ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	return application
}

func performJSONRequest(t *testing.T, server *Server, method, path string, body any) *ut.ResponseRecorder {
	t.Helper()
	var requestBody *ut.Body
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		requestBody = &ut.Body{Body: bytes.NewReader(data), Len: len(data)}
	}
	return ut.PerformRequest(
		server.engine.Engine,
		method,
		path,
		requestBody,
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
}

func decodeResponse(t *testing.T, data []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, string(data))
	}
}

func containsSessionTitle(sessions []testSessionDTO, id, title string) bool {
	for _, sess := range sessions {
		if sess.ID == id && sess.Title == title {
			return true
		}
	}
	return false
}
