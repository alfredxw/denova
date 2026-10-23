package antigravity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"denova/internal/agents/runtime/external"
)

func TestConcurrentRunsIsolateTransientAgentsAndCleanUp(t *testing.T) {
	client := fixtureClient(t, "concurrent")
	root := filepath.Join(os.Getenv("HOME"), ".gemini", "config", "agents")
	userAgent := filepath.Join(root, "my-agent", "agent.md")
	if err := os.MkdirAll(filepath.Dir(userAgent), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userAgent, []byte("User-owned definition"), 0600); err != nil {
		t.Fatal(err)
	}
	started, release, done := make(chan struct{}, 2), make(chan struct{}), make(chan error, 2)
	inputs := []external.Input{fixtureInput(t), fixtureInput(t)}
	for _, input := range inputs {
		go func() {
			defer func() {
				if value := recover(); value != nil {
					done <- fmt.Errorf("concurrent run panicked: %v", value)
				}
			}()
			host := &testHost{onCall: func(ctx context.Context, _ external.ToolCall) (external.ToolResult, error) {
				started <- struct{}{}
				select {
				case <-release:
					return external.ToolResult{Text: "Committed product outcome", Success: true}, nil
				case <-ctx.Done():
					return external.ToolResult{}, ctx.Err()
				}
			}}
			_, err := client.Run(t.Context(), input, host)
			done <- err
		}()
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatal("concurrent host calls did not arrive")
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 3 {
		close(release)
		t.Fatalf("concurrent agent definitions: %#v %v", entries, err)
	}
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	entries, err = os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "my-agent" {
		t.Fatalf("transient agent cleanup: %#v %v", entries, err)
	}
	if body, err := os.ReadFile(userAgent); err != nil || string(body) != "User-owned definition" {
		t.Fatalf("user agent changed: %s %v", body, err)
	}
}

// This smoke test uses the native installed executable without model turns or
// sign-in prompts. It independently checks that the CLI discovers the generated
// frontmatter/server schema. Authenticated inference remains a separate check.
func TestInstalledAntigravityDiscovery(t *testing.T) {
	executable := os.Getenv("DENOVA_TEST_ANTIGRAVITY_EXE")
	if executable == "" {
		t.Skip("set DENOVA_TEST_ANTIGRAVITY_EXE to verify native CLI discovery")
	}
	// Keep the native CLI's own deeply nested data paths below Windows path
	// limits even when the test runner already uses a long sandbox temp root.
	home, err := os.MkdirTemp("", "denova-agy-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	ctx, cancel := context.WithTimeout(t.Context(), infrastructureTimeout)
	defer cancel()
	client, err := Connect(ctx, ProcessOptions{Executable: executable})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	bridge, err := startBridge(ctx, nil, &testHost{}, cancel)
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.close()
	name, cleanup, err := writeAgent(fixtureInput(t), bridge)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Fatal(err)
	}
	command := client.command(ctx, "agents")
	command.Dir = t.TempDir()
	body, err := command.Output()
	if err != nil || !slices.Contains(strings.Fields(string(body)), name) {
		t.Fatalf("native CLI did not discover transient agent: %q %v", body, err)
	}
	t.Logf("Native Antigravity %s accepted generated agent metadata and server schema", client.Version())
}

func TestInstalledAntigravityCatalog(t *testing.T) {
	executable := os.Getenv("DENOVA_TEST_ANTIGRAVITY_EXE")
	if executable == "" {
		t.Skip("set DENOVA_TEST_ANTIGRAVITY_EXE to verify native CLI catalog diagnostics")
	}
	home, err := os.MkdirTemp("", "denova-agy-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	client, err := Connect(t.Context(), ProcessOptions{Executable: executable})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Models(t.Context()); !errors.Is(err, errAuthenticationRequired) {
		t.Fatalf("native unauthenticated catalog check: %v", err)
	}
}
