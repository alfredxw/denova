package handlers

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agentprofiles"
	"github.com/cloudwego/hertz/pkg/app"
)

func TestSettingsMutationErrorPreservesFileOutcomesAndLocalizesMessage(t *testing.T) {
	files := []agentprofiles.FileResult{
		{Path: "agents/main/game.toml", Status: "failed", Code: "invalid_profile"},
		{Path: "agents/main/writing.toml", Status: "saved"},
	}
	for locale, phrase := range map[string]string{"zh-CN": "以下配置文件保存失败", "en-US": "These settings files could not be saved"} {
		t.Run(locale, func(t *testing.T) {
			request := app.NewContext(0)
			request.Request.Header.Set("X-Denova-Locale", locale)
			err := errors.Join(&agentprofiles.MutationError{Files: files}, config.ErrInvalidAgentProfile, errors.New("private storage failure"))
			if !writeSettingsMutationError(request, err) {
				t.Fatal("mutation error not handled")
			}
			var response struct {
				Error   string `json:"error"`
				Code    string `json:"code"`
				Details struct {
					Files []agentprofiles.FileResult `json:"files"`
				} `json:"details"`
			}
			if err := json.Unmarshal(request.Response.Body(), &response); err != nil {
				t.Fatal(err)
			}
			if request.Response.StatusCode() != 400 || response.Code != "settings_file_save_failed" || !reflect.DeepEqual(response.Details.Files, files) {
				t.Fatalf("partial result lost: status=%d response=%+v", request.Response.StatusCode(), response)
			}
			if !strings.Contains(response.Error, phrase) || !strings.Contains(response.Error, files[0].Path) || strings.Contains(response.Error, "private storage") || strings.Contains(response.Error, "{paths}") {
				t.Fatalf("unexpected localized error: %q", response.Error)
			}
		})
	}
}
