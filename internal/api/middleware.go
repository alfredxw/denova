package api

import (
	"context"
	"net"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/i18n"
	"denova/internal/observability"
)

// corsMiddleware 处理 CORS 跨域请求。
func corsMiddleware(ctx context.Context, c *app.RequestContext) {
	origin := string(c.Request.Header.Peek("Origin"))
	allowedOrigins := []string{
		"http://localhost:7788",
		"http://localhost:3000",
		"http://127.0.0.1:7788",
		"http://127.0.0.1:3000",
	}

	allowed := false
	for _, o := range allowedOrigins {
		if strings.EqualFold(origin, o) {
			allowed = true
			break
		}
	}
	if allowed {
		c.Response.Header.Set("Access-Control-Allow-Origin", origin)
	}
	c.Response.Header.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, PUT, OPTIONS")
	c.Response.Header.Set("Access-Control-Allow-Headers", "Content-Type, X-Denova-Locale, X-Nova-Locale, Authorization")
	c.Response.Header.Set("Access-Control-Expose-Headers", observability.RequestIDHeader+", X-Denova-Auth")

	if string(c.Request.Method()) == "OPTIONS" {
		c.AbortWithStatus(consts.StatusNoContent)
		return
	}

	c.Next(ctx)
}

// localHostEffectMiddleware prevents authenticated LAN clients from opening
// windows on the machine that runs Denova. Remote browsers cannot usefully
// select a server-local absolute path in any case.
func localHostEffectMiddleware(ctx context.Context, c *app.RequestContext) {
	if !isLocalClientIP(requestClientIP(c)) {
		abortWithLocalizedError(c, consts.StatusForbidden, "api.access.localHostEffect")
		return
	}
	c.Next(ctx)
}

// platformManagementLocalHostMiddleware keeps only host-bound management
// actions on loopback. Extension runtimes listen on 127.0.0.1, package
// installation executes extension code here, and development previews resolve
// host paths, so authenticated LAN browsers cannot use them. Management reads
// and Project-scoped configuration stay available after login; a blanket gate
// would also reject the read-only polling that renders the workbench.
func platformManagementLocalHostMiddleware(ctx context.Context, c *app.RequestContext) {
	if platformManagementHostOnly(string(c.Path()), string(c.Method())) && !isLocalClientIP(requestClientIP(c)) {
		message := i18n.FromHeader(localeHeader(c)).T("api.access.localHostEffect")
		c.AbortWithStatusJSON(consts.StatusForbidden, map[string]string{
			"error":      message,
			"code":       "LOCAL_HOST_ONLY",
			"messageKey": "platform.errors.LOCAL_HOST_ONLY",
		})
		return
	}
	c.Next(ctx)
}

// platformManagementHostOnly classifies management actions that must run on the
// Denova host. Classify new actions here when they start host runtimes, install
// or preview host packages, or resolve host file paths.
func platformManagementHostOnly(path, method string) bool {
	if method == "GET" || method == "HEAD" || method == "OPTIONS" {
		return false
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(path, "/api/platform/manage"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return true
	}
	// Only known non-host writes are allowed from an authenticated LAN
	// browser. New mutating routes require an explicit policy decision.
	switch parts[0] {
	case "game-preferences":
		return !(len(parts) == 1 && method == "PATCH")
	case "instances":
		return !((len(parts) == 2 && (method == "PATCH" || method == "DELETE")) ||
			(len(parts) == 3 && parts[2] == "stop" && method == "POST"))
	case "runtimes":
		return !((len(parts) == 3 && parts[2] == "stop" && method == "POST") ||
			(len(parts) == 4 && parts[2] == "consumers" && method == "DELETE"))
	case "projects":
		return !((len(parts) == 3 && parts[2] == "extensions" && method == "PUT") ||
			(len(parts) == 5 && parts[2] == "plugins" && parts[4] == "content" && method == "POST"))
	}
	return true
}

func abortWithLocalizedError(c *app.RequestContext, status int, key string) {
	message := i18n.FromHeader(localeHeader(c)).T(key)
	c.AbortWithStatusJSON(status, map[string]string{"error": message})
}

func localeHeader(c *app.RequestContext) string {
	if header := strings.TrimSpace(string(c.Request.Header.Peek("X-Denova-Locale"))); header != "" {
		return header
	}
	return strings.TrimSpace(string(c.Request.Header.Peek("X-Nova-Locale")))
}

func requestClientIP(c *app.RequestContext) string {
	remote := directClientIP(c)
	if isLocalClientIP(remote) {
		if forwarded := forwardedClientIP(string(c.Request.Header.Peek("X-Forwarded-For"))); forwarded != "" {
			return forwarded
		}
	}
	return remote
}

func directClientIP(c *app.RequestContext) string {
	if addr := c.RemoteAddr(); addr != nil {
		host, _, err := net.SplitHostPort(strings.TrimSpace(addr.String()))
		if err == nil {
			return host
		}
		if addr.String() != "" {
			return addr.String()
		}
	}
	return c.ClientIP()
}

func forwardedClientIP(header string) string {
	for _, part := range strings.Split(header, ",") {
		value := strings.TrimSpace(part)
		if net.ParseIP(value) != nil {
			return value
		}
	}
	return ""
}

func isLocalClientIP(value string) bool {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsUnspecified()
}
