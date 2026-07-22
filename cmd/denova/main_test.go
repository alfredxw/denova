package main

import (
	"net"
	"strconv"
	"strings"
	"testing"
)

func TestViteDevEnvOverridesPortValues(t *testing.T) {
	env := viteDevEnv([]string{
		"DENOVA_BACKEND_PORT=8080",
		"DENOVA_FRONTEND_PORT=5173",
		"KEEP=value",
	}, "15173", "18080")

	if got := envValue(env, "DENOVA_BACKEND_PORT"); got != "18080" {
		t.Fatalf("DENOVA_BACKEND_PORT should use actual backend port: %q", got)
	}
	if got := envValue(env, "DENOVA_FRONTEND_PORT"); got != "15173" {
		t.Fatalf("DENOVA_FRONTEND_PORT should use actual frontend port: %q", got)
	}
	if got := envValue(env, "KEEP"); got != "value" {
		t.Fatalf("unrelated env should be preserved: %q", got)
	}
	if countEnvKey(env, "DENOVA_BACKEND_PORT") != 1 {
		t.Fatalf("DENOVA_BACKEND_PORT should not be duplicated: %#v", env)
	}
}

func TestSelectFrontendPortAvoidsBackendPort(t *testing.T) {
	preferred := findPortWithAvailableWindow(t)

	got := selectFrontendPort(preferred, preferred)

	if got == preferred {
		t.Fatalf("frontend port should avoid reserved backend port %s", preferred)
	}
	if portReserved(got, preferred) {
		t.Fatalf("frontend port should not use reserved backend port: got=%s reserved=%s", got, preferred)
	}
}

func TestSelectFrontendPortAvoidsAutoPickedBackendPort(t *testing.T) {
	preferred := findPortWithAvailableWindow(t)
	ln, err := net.Listen("tcp", "0.0.0.0:"+preferred)
	if err != nil {
		t.Fatalf("reserve preferred backend port: %v", err)
	}
	defer ln.Close()

	backendListener, backendPort, err := reserveBackendListener("127.0.0.1", preferred, true)
	if err != nil {
		t.Fatalf("reserve fallback backend port: %v", err)
	}
	defer backendListener.Close()
	frontendPort := selectFrontendPort(backendPort, backendPort)

	if frontendPort == backendPort {
		t.Fatalf("frontend port should avoid auto-picked backend port %s", backendPort)
	}
}

func TestReserveBackendListenerAutoPicksAndKeepsPortReserved(t *testing.T) {
	preferred := findPortWithAvailableWindow(t)
	reserved, err := net.Listen("tcp", "127.0.0.1:"+preferred)
	if err != nil {
		t.Fatalf("reserve preferred backend port: %v", err)
	}
	defer reserved.Close()

	listener, selectedPort, err := reserveBackendListener("127.0.0.1", preferred, true)
	if err != nil {
		t.Fatalf("reserve fallback port: %v", err)
	}
	defer listener.Close()
	if selectedPort == preferred {
		t.Fatalf("selected port should differ from occupied port %s", preferred)
	}
	if listenerPort := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port); selectedPort != listenerPort {
		t.Fatalf("listener should reserve selected port: selected=%s addr=%s", selectedPort, listener.Addr())
	}
}

func TestReserveBackendListenerRejectsExplicitOccupiedPort(t *testing.T) {
	preferred := findPortWithAvailableWindow(t)
	reserved, err := net.Listen("tcp", "127.0.0.1:"+preferred)
	if err != nil {
		t.Fatalf("reserve preferred backend port: %v", err)
	}
	defer reserved.Close()

	listener, selectedPort, err := reserveBackendListener("127.0.0.1", preferred, false)
	if err == nil {
		if listener != nil {
			_ = listener.Close()
		}
		t.Fatal("explicit occupied port should return an error")
	}
	if listener != nil || selectedPort != preferred {
		t.Fatalf("unexpected explicit port result: listener=%v port=%s", listener, selectedPort)
	}
}

func TestPortWasExplicitlySet(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "not set", args: []string{"--dev"}, want: false},
		{name: "separate long flag", args: []string{"--port", "8080"}, want: true},
		{name: "assigned long flag", args: []string{"--port=8080"}, want: true},
		{name: "separate short flag", args: []string{"-port", "8080"}, want: true},
		{name: "after terminator", args: []string{"--", "--port", "8080"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := portWasExplicitlySet(tt.args); got != tt.want {
				t.Fatalf("portWasExplicitlySet(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix)
		}
	}
	return ""
}

func countEnvKey(env []string, key string) int {
	prefix := key + "="
	count := 0
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			count++
		}
	}
	return count
}

func findPortWithAvailableWindow(t *testing.T) string {
	t.Helper()
	for range 100 {
		ln, err := net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			t.Fatalf("find free port: %v", err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close()
		if port > 65515 {
			continue
		}
		preferred := strconv.Itoa(port)
		for candidate := port + 1; candidate <= port+20; candidate++ {
			if portAvailable(strconv.Itoa(candidate)) {
				return preferred
			}
		}
	}
	t.Fatal("could not find a free port with an available follow-up port")
	return ""
}
