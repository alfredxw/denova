package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"

	"nova/internal/restart"
)

func TestHandleRestartSchedulesProcessReplacement(t *testing.T) {
	oldScheduleRestart := scheduleRestart
	defer func() { scheduleRestart = oldScheduleRestart }()

	called := make(chan time.Duration, 1)
	scheduleRestart = func(delay time.Duration) error {
		called <- delay
		return nil
	}

	resp := performRestartRequest(t)
	if resp.Code != http.StatusOK {
		t.Fatalf("restart status = %d body=%s", resp.Code, resp.Body.String())
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "restarting" {
		t.Fatalf("restart body = %#v", body)
	}

	select {
	case delay := <-called:
		if delay != restart.DefaultDelay {
			t.Fatalf("restart delay = %s, want %s", delay, restart.DefaultDelay)
		}
	case <-time.After(time.Second):
		t.Fatal("restart scheduler was not called")
	}
}

func TestHandleRestartReportsScheduleFailure(t *testing.T) {
	oldScheduleRestart := scheduleRestart
	defer func() { scheduleRestart = oldScheduleRestart }()

	scheduleRestart = func(time.Duration) error {
		return errors.New("restart unavailable")
	}

	resp := performRestartRequest(t)
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("restart status = %d body=%s", resp.Code, resp.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error != "restart unavailable" {
		t.Fatalf("restart error = %#v", body)
	}
}

func performRestartRequest(t *testing.T) *ut.ResponseRecorder {
	t.Helper()
	server := hertzserver.Default()
	server.POST("/api/restart", New(nil).HandleRestart)
	return ut.PerformRequest(server.Engine, http.MethodPost, "/api/restart", nil)
}
