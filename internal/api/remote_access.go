package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"

	"denova/config"
	"github.com/cloudwego/hertz/pkg/app"
	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// remoteAccessGate owns browser authentication for one server. Static application
// assets and explicit auth endpoints bootstrap the UI; all user resources stay behind the gate.
type remoteAccessGate struct {
	config       func() config.RemoteAccessConfig
	sessions     *browserSessions
	cookieName   string
	port         int
	listeningLAN bool
}

func newRemoteAccessGate(readConfig func() config.RemoteAccessConfig, port string) *remoteAccessGate {
	access := readConfig()
	number, _ := strconv.Atoi(port)
	return &remoteAccessGate{config: readConfig, sessions: newBrowserSessions(access.DataDir), cookieName: "denova_session_" + port, port: number, listeningLAN: access.AllowLANAccess}
}

func (g *remoteAccessGate) middleware(ctx context.Context, c *app.RequestContext) {
	// Cookies also accompany browser-initiated writes and WebSocket upgrades.
	if needsOriginCheck(c) && !sameOriginRequest(c) {
		abortWithLocalizedError(c, consts.StatusForbidden, "api.access.originRejected")
		return
	}
	if isLocalClientIP(requestClientIP(c)) {
		c.Next(ctx)
		return
	}
	access := g.config()
	if !access.AllowLANAccess {
		abortWithLocalizedError(c, consts.StatusForbidden, "api.access.lanDisabled")
		return
	}
	path := string(c.Request.Path())
	if !strings.HasPrefix(path, "/api/") && path != "/api" {
		c.Next(ctx)
		return
	}
	switch path {
	case "/api/auth/status", "/api/auth/login", "/api/auth/pair", "/api/auth/logout":
		c.Next(ctx)
		return
	}
	if g.sessions.authorized(string(c.Cookie(g.cookieName)), access) {
		c.Next(ctx)
		return
	}
	c.Response.Header.Set("Cache-Control", "no-store")
	// A machine-readable challenge avoids browser-native Basic Auth dialogs.
	c.Response.Header.Set("X-Denova-Auth", "required")
	abortWithLocalizedError(c, consts.StatusUnauthorized, "api.access.authRequired")
}

func needsOriginCheck(c *app.RequestContext) bool {
	method := string(c.Method())
	return (method != "GET" && method != "HEAD" && method != "OPTIONS") || strings.EqualFold(string(c.GetHeader("Upgrade")), "websocket")
}

func sameOriginRequest(c *app.RequestContext) bool {
	origin := string(c.GetHeader("Origin"))
	if origin == "" {
		origin = string(c.GetHeader("Referer"))
	}
	// Non-browser clients may omit both; browser form/fetch writes carry Origin.
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	host := string(c.Host())
	scheme := string(c.Request.URI().Scheme())
	if isLocalClientIP(directClientIP(c)) {
		if forwarded := string(c.GetHeader("X-Forwarded-Host")); forwarded != "" {
			host = forwarded
		}
		if forwarded := string(c.GetHeader("X-Forwarded-Proto")); forwarded != "" {
			scheme = forwarded
		}
	}
	return strings.EqualFold(parsed.Host, host) && strings.EqualFold(parsed.Scheme, scheme)
}

func (g *remoteAccessGate) registerRoutes(h *hertzserver.Hertz) {
	h.GET("/api/auth/status", g.status)
	h.POST("/api/auth/login", g.login)
	h.POST("/api/auth/logout", g.logout)
	h.POST("/api/auth/pair", g.pair)
	h.POST("/api/auth/link", localHostEffectMiddleware, g.link)
}

// A loopback proxy may serve the browser on a different port (for example Vite).
// Keep its HTTP endpoint while replacing only the host with the LAN address;
// forwarded headers from direct remote peers are never trusted.
func (g *remoteAccessGate) lanHTTPURL(c *app.RequestContext) string {
	fallback := config.LANHTTPURL(g.port)
	if !isLocalClientIP(directClientIP(c)) {
		return fallback
	}
	host := string(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		return fallback
	}
	scheme := string(c.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = "http"
	}
	if scheme != "http" && scheme != "https" {
		return fallback
	}
	endpoint, err := url.Parse(scheme + "://" + host)
	if err != nil || endpoint.Host != host || endpoint.Hostname() == "" {
		return fallback
	}
	address := config.LANAddress()
	if port := endpoint.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return fallback
		}
		address = net.JoinHostPort(address, port)
	}
	return scheme + "://" + address
}

func (g *remoteAccessGate) status(_ context.Context, c *app.RequestContext) {
	local := isLocalClientIP(requestClientIP(c))
	access := g.config()
	result := map[string]any{"local": local, "authenticated": local || g.sessions.authorized(string(c.Cookie(g.cookieName)), access)}
	if local && access.AllowLANAccess && g.listeningLAN {
		result["lan_url"] = g.lanHTTPURL(c) + "/"
	}
	c.Response.Header.Set("Cache-Control", "no-store")
	c.JSON(consts.StatusOK, result)
}

func (g *remoteAccessGate) login(ctx context.Context, c *app.RequestContext) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	access := g.config()
	if !access.AllowLANAccess {
		abortWithLocalizedError(c, consts.StatusForbidden, "api.access.lanDisabled")
		return
	}
	if len(c.Request.Body()) > 4096 || c.BindJSON(&input) != nil {
		abortWithLocalizedError(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(input.Username)), []byte(access.Username)) != 1 || !config.CheckRemoteAccessPassword(access.PasswordHash, input.Password) {
		slog.InfoContext(ctx, "remote_login_rejected", "client_ip", requestClientIP(c))
		abortWithLocalizedError(c, consts.StatusUnauthorized, "api.access.invalidCredentials")
		return
	}
	token, err := g.sessions.issue(access)
	if err != nil {
		g.storeFailure(ctx, c, err)
		return
	}
	g.setCookie(c, token, int(browserSessionLifetime.Seconds()))
	slog.InfoContext(ctx, "remote_login_succeeded", "client_ip", requestClientIP(c))
	c.JSON(consts.StatusOK, map[string]any{"local": isLocalClientIP(requestClientIP(c)), "authenticated": true})
}

func (g *remoteAccessGate) logout(ctx context.Context, c *app.RequestContext) {
	if err := g.sessions.revoke(string(c.Cookie(g.cookieName))); err != nil {
		g.storeFailure(ctx, c, err)
		return
	}
	g.setCookie(c, "", -1)
	c.Status(consts.StatusNoContent)
	slog.InfoContext(ctx, "remote_logout_succeeded", "client_ip", requestClientIP(c))
}

func (g *remoteAccessGate) link(ctx context.Context, c *app.RequestContext) {
	// Link creation grants access without a password; require a loopback host as well as a local peer.
	host, err := url.Parse("http://" + string(c.Host()))
	if err != nil || (host.Hostname() != "localhost" && !isLocalClientIP(host.Hostname())) {
		abortWithLocalizedError(c, consts.StatusForbidden, "api.access.localHostEffect")
		return
	}
	access := g.config()
	if !access.AllowLANAccess || !g.listeningLAN {
		abortWithLocalizedError(c, consts.StatusForbidden, "api.access.lanDisabled")
		return
	}
	token, err := g.sessions.createPairing(access)
	if err != nil {
		g.storeFailure(ctx, c, err)
		return
	}
	c.Response.Header.Set("Cache-Control", "no-store")
	// The fragment never reaches access logs or Referer headers.
	c.JSON(consts.StatusOK, map[string]any{"url": g.lanHTTPURL(c) + "/#pair=" + token, "expires_in": int(pairingLinkLifetime.Seconds())})
	slog.InfoContext(ctx, "remote_pairing_link_created")
}

func (g *remoteAccessGate) pair(ctx context.Context, c *app.RequestContext) {
	var input struct {
		Token string `json:"token"`
	}
	if len(c.Request.Body()) > 4096 || c.BindJSON(&input) != nil {
		abortWithLocalizedError(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	token, err := g.sessions.exchangePairing(input.Token, g.config())
	if errors.Is(err, errPairingInvalid) {
		abortWithLocalizedError(c, consts.StatusUnauthorized, "api.access.pairingInvalid")
		return
	}
	if err != nil {
		g.storeFailure(ctx, c, err)
		return
	}
	g.setCookie(c, token, int(browserSessionLifetime.Seconds()))
	slog.InfoContext(ctx, "remote_pairing_succeeded", "client_ip", requestClientIP(c))
	c.JSON(consts.StatusOK, map[string]any{"local": isLocalClientIP(requestClientIP(c)), "authenticated": true})
}

func (g *remoteAccessGate) setCookie(c *app.RequestContext, token string, maxAge int) {
	secure := string(c.Request.URI().Scheme()) == "https"
	if isLocalClientIP(directClientIP(c)) && strings.EqualFold(string(c.GetHeader("X-Forwarded-Proto")), "https") {
		secure = true
	}
	c.SetCookie(g.cookieName, token, maxAge, "/", "", protocol.CookieSameSiteLaxMode, secure, true)
	c.Response.Header.Set("Cache-Control", "no-store")
}

func (g *remoteAccessGate) storeFailure(ctx context.Context, c *app.RequestContext, err error) {
	slog.ErrorContext(ctx, "remote_access_store_failed", "error", err)
	abortWithLocalizedError(c, consts.StatusServiceUnavailable, "api.access.storeFailed")
}
