package antigravity

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckRefusesActiveAmbientHooksWithoutEditingThem(t *testing.T) {
	client := fixtureClient(t, "tool")
	path := filepath.Join(os.Getenv("HOME"), ".gemini", "config", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	active := `{"my-hook":{"SessionStart":[{"type":"command","command":"write-data"}]}}`
	if err := os.WriteFile(path, []byte(active), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := client.Check(t.Context())
	if !errors.Is(err, ErrUnsafeCustomization) || state.Status != "unavailable" || state.ReasonKey != "agentRuntime.antigravityIsolationUnsupported" {
		t.Fatalf("unsafe hook check: %#v %v", state, err)
	}
	if body, err := os.ReadFile(path); err != nil || string(body) != active {
		t.Fatalf("user hooks changed: %s %v", body, err)
	}
	if err := os.WriteFile(path, []byte(`{"my-hook":{"enabled":false,"SessionStart":[{"command":"disabled"}]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	state, err = client.Check(t.Context())
	if err != nil || state.Status != "ready" {
		t.Fatalf("disabled hook check: %#v %v", state, err)
	}
	settings := filepath.Join(os.Getenv("HOME"), ".gemini", "antigravity-cli", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"hooks":{"inline":{"PreInvocation":[{"command":"unsafe"}]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkAmbientHooks(); !errors.Is(err, ErrUnsafeCustomization) {
		t.Fatalf("inline hooks were not inspected: %v", err)
	}
	if err := os.WriteFile(settings, []byte(`{"colorScheme":"dark","hooks":{"inline":{"enabled":false,"PreInvocation":[{"command":"disabled"}]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkAmbientHooks(); err != nil {
		t.Fatalf("disabled inline hooks changed readiness: %v", err)
	}
}
