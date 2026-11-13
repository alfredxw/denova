package api

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"denova/config"
	runtimeapp "denova/internal/app"
	"github.com/cloudwego/hertz/pkg/app"
	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestNewServerUsesLocalHostByDefault(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	if server.host != config.LocalHTTPHost {
		t.Fatalf("server host = %q, want %q", server.host, config.LocalHTTPHost)
	}
}

func TestNewServerUsesLANHostWhenEnabled(t *testing.T) {
	root := t.TempDir()
	hash, err := config.HashRemoteAccessPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	application, err := runtimeapp.New(context.Background(), &config.Config{
		OpenAIModel:              "test-model",
		NovaDir:                  root,
		Workspace:                root,
		ResumeLastWorkspace:      false,
		AllowLANAccess:           true,
		RemoteAccessUsername:     "reader",
		RemoteAccessPasswordHash: hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(application, "0")
	if server.host != config.LANHTTPHost {
		t.Fatalf("server host = %q, want %q", server.host, config.LANHTTPHost)
	}
}

func TestIsLocalClientIP(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "::1"} {
		if !isLocalClientIP(value) {
			t.Fatalf("%s should be local", value)
		}
	}
	for _, value := range []string{"192.168.1.8", "10.0.0.2", ""} {
		if isLocalClientIP(value) {
			t.Fatalf("%s should be remote", value)
		}
	}
}

func TestForwardedClientIPUsesFirstValidAddress(t *testing.T) {
	got := forwardedClientIP(" 192.168.1.8, 127.0.0.1")
	if got != "192.168.1.8" {
		t.Fatalf("forwardedClientIP = %q", got)
	}
	if got := forwardedClientIP("unknown, "); got != "" {
		t.Fatalf("invalid forwarded header should be ignored: %q", got)
	}
}

func TestLocalHostEffectMiddlewareRejectsForwardedRemoteClient(t *testing.T) {
	called := false
	server := hertzserver.Default()
	server.POST("/native", localHostEffectMiddleware, func(_ context.Context, c *app.RequestContext) {
		called = true
		c.Status(http.StatusNoContent)
	})
	response := ut.PerformRequest(
		server.Engine,
		http.MethodPost,
		"/native",
		nil,
		ut.Header{Key: "X-Forwarded-For", Value: "192.168.1.8"},
		ut.Header{Key: "X-Denova-Locale", Value: "en-US"},
	)
	if response.Code != http.StatusForbidden || called {
		t.Fatalf("remote host effect response = %d called=%v body=%s", response.Code, called, response.Body.String())
	}
}

func TestPlatformManagementHostOnlyClassification(t *testing.T) {
	for _, test := range []struct {
		path, method string
		hostOnly     bool
	}{
		{"/api/platform/manage/runtimes", http.MethodGet, false},
		{"/api/platform/manage/catalog", http.MethodGet, false},
		{"/api/platform/manage/packages/plugin", http.MethodGet, false},
		{"/api/platform/manage/packages/plugin/example/impact", http.MethodGet, false},
		{"/api/platform/manage/development", http.MethodGet, false},
		{"/api/platform/manage/projects/p1/extensions", http.MethodGet, false},
		{"/api/platform/manage/instances/one", http.MethodPatch, false},
		{"/api/platform/manage/instances/one", http.MethodDelete, false},
		{"/api/platform/manage/runtimes/one/consumers/two", http.MethodDelete, false},
		{"/api/platform/manage/projects/p1/plugins/example/content", http.MethodPost, false},
		{"/api/platform/manage", http.MethodPost, true},
		{"/api/platform/manage/new-action", http.MethodPost, true},
		{"/api/platform/manage/runtimes/plugin/extra", http.MethodPost, true},
		{"/api/platform/manage/projects/p1/extensions/extra", http.MethodPut, true},
		{"/api/platform/manage/instances/one/stop", http.MethodPost, false},
		{"/api/platform/manage/runtimes/one/stop", http.MethodPost, false},
		{"/api/platform/manage/game-preferences", http.MethodPatch, false},
		{"/api/platform/manage/projects/p1/extensions", http.MethodPut, false},
		{"/api/platform/manage/instances", http.MethodPost, true},
		{"/api/platform/manage/instances/one/open", http.MethodPost, true},
		{"/api/platform/manage/instances/one/upgrade", http.MethodPost, true},
		{"/api/platform/manage/plugin-actions/open", http.MethodPost, true},
		{"/api/platform/manage/runtimes/plugin", http.MethodPost, true},
		{"/api/platform/manage/packages/install", http.MethodPost, true},
		{"/api/platform/manage/packages/preview", http.MethodPost, true},
		{"/api/platform/manage/packages/plugin/example", http.MethodPatch, true},
		{"/api/platform/manage/development/link", http.MethodPost, true},
		{"/api/platform/manage/candidates/one", http.MethodDelete, true},
	} {
		if got := platformManagementHostOnly(test.path, test.method); got != test.hostOnly {
			t.Fatalf("%s %s host-only = %v, want %v", test.method, test.path, got, test.hostOnly)
		}
	}
}

func TestPlatformManagementReadsServeAuthenticatedLANClients(t *testing.T) {
	root := t.TempDir()
	hash, err := config.HashRemoteAccessPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	application, err := runtimeapp.New(context.Background(), &config.Config{
		OpenAIModel:              "test-model",
		NovaDir:                  root,
		Workspace:                root,
		ResumeLastWorkspace:      false,
		AllowLANAccess:           true,
		RemoteAccessUsername:     "reader",
		RemoteAccessPasswordHash: hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	server := NewServer(application, "7789")

	login := ut.PerformRequest(server.engine.Engine, http.MethodPost, "/api/auth/login", &ut.Body{Body: bytes.NewBufferString(`{"username":"reader","password":"secret"}`), Len: len(`{"username":"reader","password":"secret"}`)}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if login.Code != http.StatusOK {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	cookie := strings.Split(login.Header().Get("Set-Cookie"), ";")[0]
	if cookie == "" {
		t.Fatal("login returned no session cookie")
	}
	unauthenticated := ut.PerformRequest(server.engine.Engine, http.MethodGet, "/api/platform/manage/runtimes", nil, ut.Header{Key: "X-Forwarded-For", Value: "192.168.1.8"})
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated management read: %d %s", unauthenticated.Code, unauthenticated.Body.String())
	}
	remote := []ut.Header{{Key: "X-Forwarded-For", Value: "192.168.1.8"}, {Key: "Cookie", Value: cookie}}

	if response := ut.PerformRequest(server.engine.Engine, http.MethodGet, "/api/platform/manage/runtimes", nil, remote...); response.Code != http.StatusOK {
		t.Fatalf("remote runtime read: %d %s", response.Code, response.Body.String())
	}
	open := ut.PerformRequest(server.engine.Engine, http.MethodPost, "/api/platform/manage/instances/one/open", &ut.Body{Body: bytes.NewBufferString(`{}`), Len: 2}, append(remote, ut.Header{Key: "Content-Type", Value: "application/json"})...)
	if open.Code != http.StatusForbidden || !strings.Contains(open.Body.String(), "LOCAL_HOST_ONLY") {
		t.Fatalf("remote host action: %d %s", open.Code, open.Body.String())
	}
	local := ut.PerformRequest(server.engine.Engine, http.MethodPost, "/api/platform/manage/instances/one/open", &ut.Body{Body: bytes.NewBufferString(`{}`), Len: 2}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if local.Code == http.StatusForbidden {
		t.Fatalf("local host action rejected: %d %s", local.Code, local.Body.String())
	}
}
