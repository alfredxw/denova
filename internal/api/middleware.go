package api

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"net"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"nova/config"
	novaApp "nova/internal/app"
	"nova/internal/i18n"
)

// corsMiddleware 处理 CORS 跨域请求。
func corsMiddleware(ctx context.Context, c *app.RequestContext) {
	origin := string(c.Request.Header.Peek("Origin"))
	allowedOrigins := []string{
		"http://localhost:5173",
		"http://localhost:3000",
		"http://127.0.0.1:5173",
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
	c.Response.Header.Set("Access-Control-Allow-Headers", "Content-Type, X-Nova-Locale, Authorization")

	if string(c.Request.Method()) == "OPTIONS" {
		c.AbortWithStatus(consts.StatusNoContent)
		return
	}

	c.Next(ctx)
}

func remoteAccessMiddleware(application *novaApp.App) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		clientIP := requestClientIP(c)
		if isLocalClientIP(clientIP) {
			c.Next(ctx)
			return
		}

		access := application.RemoteAccessConfig()
		if !access.AllowLANAccess {
			abortWithLocalizedError(c, consts.StatusForbidden, "api.access.lanDisabled")
			return
		}
		if remoteAccessAuthorized(access, string(c.Request.Header.Peek("Authorization"))) {
			c.Next(ctx)
			return
		}

		c.Response.Header.Set("WWW-Authenticate", `Basic realm="Nova"`)
		abortWithLocalizedError(c, consts.StatusUnauthorized, "api.access.authRequired")
	}
}

func abortWithLocalizedError(c *app.RequestContext, status int, key string) {
	message := i18n.FromHeader(string(c.Request.Header.Peek("X-Nova-Locale"))).T(key)
	c.AbortWithStatusJSON(status, map[string]string{"error": message})
}

func remoteAccessAuthorized(access config.RemoteAccessConfig, header string) bool {
	username, password, ok := parseBasicAuth(header)
	if !ok {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(username), []byte(access.Username)) != 1 {
		return false
	}
	return config.CheckRemoteAccessPassword(access.PasswordHash, password)
}

func parseBasicAuth(header string) (string, string, bool) {
	const prefix = "Basic "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", "", false
	}
	payload, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header[len(prefix):]))
	if err != nil {
		return "", "", false
	}
	username, password, ok := strings.Cut(string(payload), ":")
	if !ok {
		return "", "", false
	}
	return username, password, true
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
