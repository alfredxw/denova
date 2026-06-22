package api

import (
	"context"
	"encoding/base64"
	"testing"

	"nova/config"
	runtimeapp "nova/internal/app"
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

func TestRemoteAccessAuthorized(t *testing.T) {
	hash, err := config.HashRemoteAccessPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	header := "Basic " + base64.StdEncoding.EncodeToString([]byte("reader:secret"))
	if !remoteAccessAuthorized(config.RemoteAccessConfig{
		AllowLANAccess: true,
		Username:       "reader",
		PasswordHash:   hash,
	}, header) {
		t.Fatalf("expected valid basic auth")
	}
	if remoteAccessAuthorized(config.RemoteAccessConfig{
		AllowLANAccess: true,
		Username:       "reader",
		PasswordHash:   hash,
	}, "Basic "+base64.StdEncoding.EncodeToString([]byte("reader:wrong"))) {
		t.Fatalf("wrong password should be rejected")
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
