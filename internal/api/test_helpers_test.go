package api

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"

	"denova/config"
	agentinteractive "denova/internal/agents/interactive"
	runtimeapp "denova/internal/app"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/interactive/director"
)

func newTestApplication(t *testing.T) *runtimeapp.App {
	t.Helper()
	root := t.TempDir()
	application, err := runtimeapp.New(context.Background(), &config.Config{
		OpenAIModel: "test-model", NovaDir: root, Workspace: root, ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	restoreDirector := application.SetInteractiveDirectorGeneratorForTest(func(callCtx context.Context, _ *config.Config, _ *book.State, toolContext agentinteractive.InteractiveStoryToolContext, _ string) (string, error) {
		if toolContext.MaintenanceTask == "director_plan_update" || toolContext.MaintenanceTask == "opening_plan" {
			_, err := toolContext.SubmitDirectorPlanUpdate(callCtx, interactive.DirectorPlanUpdateSubmission{
				Decision: director.Decision{Mode: director.DecisionKeep, Reason: "测试初始化导演规划完成。"}, Finalize: true,
			})
			return "测试导演规划审查完成。", err
		}
		return "测试后台维护完成。", nil
	})
	t.Cleanup(restoreDirector)
	return application
}

func activeWritingSessionID(t *testing.T, application *runtimeapp.App) string {
	t.Helper()
	sessions, err := application.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range sessions {
		if session.Active {
			return session.ID
		}
	}
	t.Fatal("test application has no active Writing Session")
	return ""
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
	return ut.PerformRequest(server.engine.Engine, method, path, requestBody, ut.Header{Key: "Content-Type", Value: "application/json"})
}

func decodeResponse(t *testing.T, data []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode response: %v body=%s", err, string(data))
	}
}

func assertIdleRuntimeProjection(t *testing.T, body []byte) {
	t.Helper()
	var response struct {
		Active            bool   `json:"active"`
		Cursor            uint64 `json:"cursor"`
		Phase             string `json:"phase"`
		RecoveryPaused    bool   `json:"recovery_paused"`
		ActiveOperationID string `json:"active_operation_id"`
		ActiveCycle       int    `json:"active_cycle"`
		ActiveOutput      struct {
			Content  string `json:"content"`
			Thinking string `json:"thinking"`
		} `json:"active_output"`
		Queue              []any `json:"queue"`
		OpenTools          []any `json:"open_tools"`
		RuntimeRecoverable bool  `json:"runtime_recoverable"`
		StreamAttached     bool  `json:"stream_attached"`
		RecoveryActions    []any `json:"recovery_actions"`
	}
	decodeResponse(t, body, &response)
	if response.Active || response.Cursor != 0 || response.Phase != "idle" || response.ActiveOperationID != "" || response.ActiveCycle != 0 {
		t.Fatalf("runtime projection identity mismatch: %#v body=%s", response, body)
	}
	if response.Queue == nil || response.OpenTools == nil || response.RecoveryActions == nil {
		t.Fatalf("runtime projection collections must be explicit empty arrays: %s", body)
	}
	if response.RuntimeRecoverable || response.StreamAttached || len(response.RecoveryActions) != 0 {
		t.Fatalf("idle runtime must not advertise recovery or an attached stream: %#v body=%s", response, body)
	}
}
