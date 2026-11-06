package api

import (
	"context"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestDesktopTransportKeepsOriginsAndTerminalCredentialsSeparate(t *testing.T) {
	for _, origin := range []string{"mygo://localhost", "http://mygo.localhost"} {
		t.Run(origin, func(t *testing.T) {
			gate, h, _ := testRemoteAccess(t)
			gate.desktopToken = "private-host-token"
			gate.desktopOrigin = origin
			h.GET("/api/terminal/sessions/protected/attach", func(_ context.Context, c *app.RequestContext) {
				if string(c.GetHeader("Origin")) != "http://localhost" {
					t.Errorf("socket origin was not normalized: %s", c.GetHeader("Origin"))
				}
				if c.Query("token") != "session-token" {
					c.Status(403)
					return
				}
				c.Status(200)
			})
			cases := []struct {
				name, path, origin, credential, upgrade string
				want                                    int
			}{
				{"private API", "/api/private", origin, "private-host-token", "", 200},
				{"origin alone is not authorization", "/api/private", origin, "", "", 403},
				{"iframe cannot use host token", "/api/private", "http://127.0.0.1:3456", "private-host-token", "", 403},
				{"other custom origin", "/api/private", "mygo://attacker", "private-host-token", "", 403},
				{"socket with session token", "/api/terminal/sessions/protected/attach?token=session-token", origin, "", "websocket", 200},
				{"socket missing token", "/api/terminal/sessions/protected/attach", origin, "", "websocket", 403},
				{"socket foreign origin", "/api/terminal/sessions/protected/attach?token=session-token", "http://127.0.0.1:3456", "", "websocket", 403},
				{"ordinary attach request", "/api/terminal/sessions/protected/attach?token=session-token", origin, "", "", 403},
				{"other websocket path", "/api/private", origin, "", "websocket", 403},
			}
			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					r := ut.PerformRequest(h.Engine, "GET", "http://localhost"+c.path, nil, ut.Header{Key: "Host", Value: "localhost"}, ut.Header{Key: "Origin", Value: c.origin}, ut.Header{Key: "X-Denova-Desktop", Value: c.credential}, ut.Header{Key: "Upgrade", Value: c.upgrade})
					if r.Code != c.want {
						t.Fatalf("status %d, want %d: %s", r.Code, c.want, r.Body.String())
					}
				})
			}
		})
	}
}
