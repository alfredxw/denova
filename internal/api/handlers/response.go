package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/i18n"
)

// decodeStrictJSONRequest rejects unknown fields and trailing JSON values at
// mutation boundaries, where silently accepting a caller typo is unsafe.
func decodeStrictJSONRequest(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

// writeJSON 写入 JSON 响应。
func writeJSON(c *app.RequestContext, code int, obj interface{}) {
	c.JSON(code, obj)
}

// writeError 写入错误响应。
func writeError(c *app.RequestContext, code int, msg string) {
	c.JSON(code, map[string]string{"error": msg})
}

func requestLocalizer(c *app.RequestContext) i18n.Localizer {
	return i18n.FromHeader(requestLocaleHeader(c))
}

func requestLocale(c *app.RequestContext) string {
	header := requestLocaleHeader(c)
	if header == "" {
		return ""
	}
	return i18n.FromHeader(header).Locale()
}

func requestLocaleHeader(c *app.RequestContext) string {
	if header := strings.TrimSpace(string(c.Request.Header.Peek("X-Denova-Locale"))); header != "" {
		return header
	}
	return strings.TrimSpace(string(c.Request.Header.Peek("X-Nova-Locale")))
}

func writeErrorKey(c *app.RequestContext, code int, key string, args ...any) {
	writeError(c, code, requestLocalizer(c).T(key, args...))
}

func messageKey(c *app.RequestContext, key string, args ...any) string {
	return requestLocalizer(c).T(key, args...)
}

func requireProjectID(c *app.RequestContext) (string, bool) {
	projectID := strings.TrimSpace(c.Param("project_id"))
	if projectID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.projectFiles.projectIDRequired")
		return "", false
	}
	return projectID, true
}

// requireWorkspace 校验当前 App 是否已绑定 workspace；
// 未绑定时直接写入 409 错误并返回 false，由调用方 return 终止处理。
func (h *Handlers) requireWorkspace(c *app.RequestContext) bool {
	if h.app.HasWorkspace() {
		return true
	}
	writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
	return false
}
