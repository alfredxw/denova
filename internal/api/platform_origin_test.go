package api

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestIsolatedRuntimeCannotReadLegacyLocalAPIs(t *testing.T) {
	_, server, _ := testRemoteAccess(t)
	for _, header := range []ut.Header{{Key: "Origin", Value: "http://127.0.0.1:49000"}, {Key: "Referer", Value: "http://127.0.0.1:49000/views/stage/"}} {
		response := ut.PerformRequest(server.Engine, "GET", "http://localhost:8080/api/private", nil, header)
		if response.Code != 403 {
			t.Fatalf("isolated view read legacy API: %d", response.Code)
		}
	}
	response := ut.PerformRequest(server.Engine, "GET", "http://localhost:8080/api/private", nil, ut.Header{Key: "Origin", Value: "http://localhost:8080"})
	if response.Code != 200 {
		t.Fatalf("trusted host cannot read API: %d", response.Code)
	}
}

func TestTrustedProxyWebSocketOrigin(t *testing.T) {
	_, server, _ := testRemoteAccess(t)
	for _, test := range []struct {
		origin, protocol string
		status           int
	}{
		{"http://127.0.0.1:15173", "ws", 200},
		{"https://127.0.0.1:15173", "wss", 200},
		{"http://127.0.0.1:49000", "ws", 403},
	} {
		response := ut.PerformRequest(server.Engine, "GET", "http://localhost:8080/api/terminal/sessions/one/attach", nil,
			ut.Header{Key: "Origin", Value: test.origin},
			ut.Header{Key: "Upgrade", Value: "websocket"},
			ut.Header{Key: "X-Forwarded-Host", Value: "127.0.0.1:15173"},
			ut.Header{Key: "X-Forwarded-Proto", Value: test.protocol})
		if response.Code != test.status {
			t.Fatalf("origin %s via %s: %d, want %d", test.origin, test.protocol, response.Code, test.status)
		}
	}
}
