package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/hertz/pkg/common/ut"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/api/agentui"
	runtimeapp "denova/internal/app"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/session"
)

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
	current := decodeAgentUIMessages(t, currentMessages.Body.Bytes())
	if len(current) != 1 || current[0].Role != "user" || testTextPartContent(t, current[0]) != "会话 B 消息" {
		t.Fatalf("当前会话消息应来自新会话: %#v", current)
	}

	switchResp := performJSONRequest(t, server, http.MethodPost, "/api/sessions/switch", map[string]string{"id": defaultID})
	if switchResp.Code != http.StatusOK {
		t.Fatalf("switch status = %d body=%s", switchResp.Code, switchResp.Body.String())
	}
	defaultMessages := performJSONRequest(t, server, http.MethodGet, "/api/session/messages?session_id="+defaultID, nil)
	defaultHistory := decodeAgentUIMessages(t, defaultMessages.Body.Bytes())
	if len(defaultHistory) != 1 || defaultHistory[0].Role != "user" || testTextPartContent(t, defaultHistory[0]) != "默认会话消息" {
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
	cleared := decodeAgentUIMessages(t, clearedResp.Body.Bytes())
	if len(cleared) != 2 || testPartType(t, cleared[1]) != agentui.DataTypeClear {
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

func TestAgentSessionAPIClearsBackgroundAgentContext(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	clearResp := performJSONRequest(t, server, http.MethodPost, "/api/agents/version_summary/session/clear", nil)
	if clearResp.Code != http.StatusOK {
		t.Fatalf("clear status = %d body=%s", clearResp.Code, clearResp.Body.String())
	}
	messagesResp := performJSONRequest(t, server, http.MethodGet, "/api/agents/version_summary/session/messages", nil)
	if messagesResp.Code != http.StatusOK {
		t.Fatalf("messages status = %d body=%s", messagesResp.Code, messagesResp.Body.String())
	}
	messages := decodeAgentUIMessages(t, messagesResp.Body.Bytes())
	if len(messages) != 1 || testPartType(t, messages[0]) != agentui.DataTypeClear {
		t.Fatalf("background agent session should expose clear marker: %#v", messages)
	}

	invalidResp := performJSONRequest(t, server, http.MethodPost, "/api/agents/unknown/session/clear", nil)
	if invalidResp.Code != http.StatusBadRequest {
		t.Fatalf("invalid agent clear status = %d body=%s", invalidResp.Code, invalidResp.Body.String())
	}
}

func TestSessionAPIReturnsSubAgentDisplayMetadata(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	if err := application.Session().AppendDisplayEvent(session.DisplayEvent{
		ID:                "run-1-subagent-01-researcher",
		Role:              "assistant",
		Content:           "SubAgent 调研结果",
		RunID:             "run-1",
		AgentName:         "researcher",
		RootAgentName:     "DenovaAgent",
		RunPath:           []string{"DenovaAgent", "researcher"},
		SubAgent:          true,
		SubAgentSessionID: "run-1-subagent-01-researcher",
		SubAgentType:      "researcher",
	}); err != nil {
		t.Fatal(err)
	}

	resp := performJSONRequest(t, server, http.MethodGet, "/api/session/messages", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("messages status = %d body=%s", resp.Code, resp.Body.String())
	}
	messages := decodeAgentUIMessages(t, resp.Body.Bytes())
	if len(messages) != 1 {
		t.Fatalf("expected one display message, got %#v", messages)
	}
	got := messages[0]
	if got.Role != "assistant" || testMetadataString(got, "display_role") != "assistant" || !testMetadataBool(got, "subagent") ||
		testMetadataString(got, "subagent_session_id") != "run-1-subagent-01-researcher" || testMetadataString(got, "subagent_type") != "researcher" {
		t.Fatalf("SubAgent metadata missing from API response: %#v", got)
	}
	if testTextPartContent(t, got) != "SubAgent 调研结果" || testMetadataString(got, "run_id") != "run-1" ||
		testMetadataString(got, "agent_name") != "researcher" || len(testMetadataStringSlice(got, "run_path")) != 2 {
		t.Fatalf("Agent path metadata missing from API response: %#v", got)
	}
}

func TestSessionAPIReturnsTokenUsageFields(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	if err := application.Session().AppendDisplayEvent(session.DisplayEvent{
		ID:                   "run-usage-1",
		Role:                 "token_usage",
		Content:              "cache_hit_rate=50.0%",
		RunID:                "run-usage-1",
		AgentKind:            config.AgentKindIDE,
		PromptTokens:         2000,
		CachedPromptTokens:   1000,
		UncachedPromptTokens: 1000,
		CacheHitRate:         0.5,
		CompletionTokens:     300,
		ReasoningTokens:      20,
		TotalTokens:          2300,
		ModelCalls:           1,
		GeneratedBytes:       128,
		UsageCalls: []session.TokenUsageCall{{
			Index:                1,
			PromptTokens:         2000,
			CachedPromptTokens:   1000,
			UncachedPromptTokens: 1000,
			CacheHitRate:         0.5,
			CompletionTokens:     300,
			ReasoningTokens:      20,
			TotalTokens:          2300,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	resp := performJSONRequest(t, server, http.MethodGet, "/api/session/messages", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("messages status = %d body=%s", resp.Code, resp.Body.String())
	}
	messages := decodeAgentUIMessages(t, resp.Body.Bytes())
	if len(messages) != 1 {
		t.Fatalf("expected one usage message, got %#v", messages)
	}
	got := messages[0]
	data := testDataPart(t, got, agentui.DataTypeTokenUsage)
	if got.Role != "assistant" || testMetadataString(got, "display_role") != "token_usage" ||
		testMetadataString(got, "agent_kind") != config.AgentKindIDE || testDataInt(data, "model_calls") != 1 {
		t.Fatalf("token usage metadata missing from API response: %#v", got)
	}
	if testDataInt(data, "prompt_tokens") != 2000 || testDataInt(data, "cached_prompt_tokens") != 1000 ||
		testDataInt(data, "uncached_prompt_tokens") != 1000 || testDataInt(data, "total_tokens") != 2300 {
		t.Fatalf("token usage counts missing from API response: %#v", got)
	}
	usageCalls, _ := data["usage_calls"].([]any)
	if testDataFloat(data, "cache_hit_rate") != 0.5 || testDataInt(data, "generated_bytes") != 128 ||
		len(usageCalls) != 1 || testDataInt(testMap(usageCalls[0]), "reasoning_tokens") != 20 {
		t.Fatalf("token usage details missing from API response: %#v", got)
	}
}

func decodeAgentUIMessages(t *testing.T, data []byte) []agentui.Message {
	t.Helper()
	var messages []agentui.Message
	decodeResponse(t, data, &messages)
	return messages
}

func testPartType(t *testing.T, message agentui.Message) string {
	t.Helper()
	if len(message.Parts) == 0 {
		t.Fatalf("message has no parts: %#v", message)
	}
	partType, _ := message.Parts[0]["type"].(string)
	return partType
}

func testTextPartContent(t *testing.T, message agentui.Message) string {
	t.Helper()
	if partType := testPartType(t, message); partType != "text" {
		t.Fatalf("expected text part, got %s in %#v", partType, message)
	}
	text, _ := message.Parts[0]["text"].(string)
	return text
}

func testDataPart(t *testing.T, message agentui.Message, expectedType string) map[string]any {
	t.Helper()
	if partType := testPartType(t, message); partType != expectedType {
		t.Fatalf("expected %s part, got %s in %#v", expectedType, partType, message)
	}
	return testMap(message.Parts[0]["data"])
}

func testMetadataString(message agentui.Message, key string) string {
	value, _ := message.Metadata[key].(string)
	return value
}

func testMetadataBool(message agentui.Message, key string) bool {
	value, _ := message.Metadata[key].(bool)
	return value
}

func testMetadataStringSlice(message agentui.Message, key string) []string {
	values, ok := message.Metadata[key].([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func testDataInt(data map[string]any, key string) int {
	switch value := data[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func testDataFloat(data map[string]any, key string) float64 {
	switch value := data[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	default:
		return 0
	}
}

func testMap(value any) map[string]any {
	data, _ := value.(map[string]any)
	return data
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
	t.Cleanup(application.Close)
	restoreDirector := application.SetInteractiveDirectorGeneratorForTest(func(callCtx context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, _ string) (string, error) {
		if toolContext.MaintenanceTask == "state_schema_initialization" {
			result, err := toolContext.SubmitStateSchemaBatch(callCtx, interactive.ActorStateSchemaBatch{
				Summary: "测试保持预设状态结构",
				Items: []interactive.ActorStateSchemaBatchItem{{
					ItemID: "keep-preset-state",
					Requirements: []interactive.ActorStateSchemaRequirementReview{{
						Source:       interactive.ActorStateSchemaRequirementSource{Kind: "opening", ID: toolContext.TurnID},
						Requirement:  "测试已审阅开局且不需要新增结构化状态",
						EvidenceKind: "confirmed",
						Decision:     "ignored",
						ValuePolicy:  interactive.ActorStateSchemaValuePolicySchemaOnly,
						Reason:       "测试开局没有建立长期状态规则",
					}},
				}},
				Finalize: true,
			})
			if err != nil || !result.Finalized {
				return "", fmt.Errorf("测试状态结构 Batch 未完成: result=%#v err=%v", result, err)
			}
			return "测试状态结构审查完成。", nil
		}
		if toolContext.MaintenanceTask == "director_plan_update" || toolContext.MaintenanceTask == "opening_plan" {
			_, err := toolContext.SubmitDirectorPlanUpdate(callCtx, interactive.DirectorPlanUpdateSubmission{
				Decision: interactive.PlanDecision{Mode: interactive.PlanDecisionKeep, Reason: "测试初始化导演规划完成。"},
				Finalize: true,
			})
			return "测试导演规划审查完成。", err
		}
		return "测试后台维护完成。", nil
	})
	t.Cleanup(restoreDirector)
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
