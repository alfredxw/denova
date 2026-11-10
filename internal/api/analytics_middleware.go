package api

import (
	"context"
	"encoding/json"
	"strings"

	"denova/internal/analytics"
	application "denova/internal/app"
	"github.com/cloudwego/hertz/pkg/app"
)

// Only authenticated, registered API routes are measured. A route is reduced
// locally to fixed feature/operation categories; paths, IDs, bodies and diagnostics never leave
// the host. Analytics endpoints are excluded to prevent feedback loops.
func analyticsMiddleware(application *application.App) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		defer func() {
			if recovered := recover(); recovered != nil {
				application.Analytics().RecoveredPanic()
				panic(recovered) // Preserve Hertz's existing recovery and response.
			}
		}()
		c.Next(ctx)
		route := c.FullPath()
		if route == "" || !strings.HasPrefix(route, "/api/") || strings.HasPrefix(route, "/api/analytics") {
			return
		}
		if status := c.Response.StatusCode(); status >= 400 {
			application.Analytics().APIError(analyticsRouteFeature(route), analyticsOperation(c), analyticsAPIError(c), status)
		}
	}
}

func analyticsRouteFeature(route string) analytics.Feature {
	switch {
	case strings.Contains(route, "/interactive"):
		return analytics.Game
	case strings.Contains(route, "/agent-chat"):
		return analytics.Chat
	case strings.Contains(route, "/images"):
		return analytics.Image
	case strings.Contains(route, "/lore"):
		return analytics.Library
	case strings.Contains(route, "/versions"):
		return analytics.Versions
	case strings.Contains(route, "/automation"):
		return analytics.Automation
	case strings.Contains(route, "/resource-market"), strings.Contains(route, "/resource-exchange"), strings.Contains(route, "/platform"):
		return analytics.Resources
	case strings.Contains(route, "/book"), strings.Contains(route, "/chat"):
		return analytics.Writing
	case strings.Contains(route, "/projects"):
		return analytics.Projects
	}
	return ""
}

// Wildcard platform paths are reduced by exact match. Unknown routes retain only
// the HTTP action; no URL, query, Project ID or file path can become a property.
func analyticsOperation(c *app.RequestContext) string {
	if c.FullPath() == "/api/books/cover" && string(c.Method()) == "GET" {
		return "book_cover"
	}
	if c.FullPath() == "/api/platform/manage/*path" && string(c.Method()) == "GET" {
		switch c.Param("path") {
		case "/plugin-pages":
			return "plugin_pages"
		case "/catalog":
			return "extension_catalog"
		case "/development":
			return "extension_development"
		case "/runtimes":
			return "extension_runtimes"
		case "/instances":
			return "game_instances"
		case "/game-preferences":
			return "game_preferences"
		}
	}
	switch string(c.Method()) {
	case "GET", "HEAD":
		return "read"
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	}
	return "other"
}

func analyticsAPIError(c *app.RequestContext) analytics.ErrorCode {
	var response struct {
		Code string `json:"code"`
	}
	// Parse only small error envelopes, then translate an allowlist locally.
	if body := c.Response.Body(); len(body) <= 64*1024 {
		_ = json.Unmarshal(body, &response)
	}
	switch response.Code {
	case "api.access.localHostEffect":
		return analytics.ErrorLocalOnly
	case "NOT_CONFIGURED", "api.workspace.noWorkspace":
		return analytics.ErrorNotConfigured
	}
	switch c.Response.StatusCode() {
	case 400, 422:
		return analytics.ErrorInvalidRequest
	case 401:
		return analytics.ErrorAuthentication
	case 403:
		return analytics.ErrorPermission
	case 404:
		return analytics.ErrorNotFound
	case 408, 504:
		return analytics.ErrorTimeout
	case 409:
		return analytics.ErrorConflict
	case 429:
		return analytics.ErrorRateLimit
	}
	return analytics.ErrorHTTP
}
