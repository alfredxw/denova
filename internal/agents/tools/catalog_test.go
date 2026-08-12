package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	agenttools "github.com/alfredxw/denova/agent/tools"

	"denova/config"
)

func TestMain(m *testing.M) {
	if os.Getenv("DENOVA_TEST_CATALOG_RIPGREP_HELPER") == "1" {
		for _, arg := range os.Args[1:] {
			if arg == "--no-config" {
				if os.Getenv("DENOVA_TEST_CATALOG_RIPGREP_LARGE") == "1" {
					payload := strings.Repeat("x", 2048)
					for line := 1; line <= 200; line++ {
						fmt.Fprintf(os.Stdout, "large.txt:%d:%s\n", line, payload)
					}
				} else {
					fmt.Fprint(os.Stdout, "chapters/one.md\n")
				}
				os.Exit(0)
			}
		}
		fmt.Fprintln(os.Stderr, "missing --no-config")
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func TestCatalogReadCapabilityFollowsTheBroadestRegisteredAdapter(t *testing.T) {
	type referenceInput struct {
		Path string `json:"path"`
	}
	reference, err := agenttools.NewReadAdapter(
		agent.CapabilityIdentity{Kind: "test.read.reference", Version: 1},
		"reference",
		func(_ context.Context, path string) (bool, error) { return strings.HasPrefix(path, "skill://"), nil },
		func(_ context.Context, input referenceInput) (agenttools.ReadResult, error) {
			return agenttools.ReadResult{Path: input.Path, Content: "reference"}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	referenceBinding, err := newReadAdapterBinding(config.AgentToolSkills, reference)
	if err != nil {
		t.Fatal(err)
	}
	catalog := NewCatalog(&config.Config{Workspace: t.TempDir()}, nil, RuntimeExecutables{})

	combined, err := catalog.Workspace(config.ResolvedAgentToolSettings{
		config.AgentToolWorkspaceRead: true,
		config.AgentToolSkills:        true,
	}, referenceBinding)
	if err != nil {
		t.Fatal(err)
	}
	if capability := descriptorCapabilityForTool(t, combined, "read"); capability != config.AgentToolWorkspaceRead {
		t.Fatalf("combined read capability = %q, want %q", capability, config.AgentToolWorkspaceRead)
	}

	referenceOnly, err := catalog.Workspace(config.ResolvedAgentToolSettings{config.AgentToolSkills: true}, referenceBinding)
	if err != nil {
		t.Fatal(err)
	}
	if capability := descriptorCapabilityForTool(t, referenceOnly, "read"); capability != config.AgentToolSkills {
		t.Fatalf("reference-only read capability = %q, want %q", capability, config.AgentToolSkills)
	}

	disabled, err := catalog.Workspace(config.ResolvedAgentToolSettings{}, referenceBinding)
	if err != nil {
		t.Fatal(err)
	}
	if len(disabled) != 0 {
		t.Fatalf("disabled adapter capability exposed tools: %#v", disabled)
	}
}

func TestCatalogResolvesLoginEnvironmentOnlyForEnabledShell(t *testing.T) {
	resolutions := 0
	catalog := NewCatalog(
		&config.Config{Workspace: t.TempDir()},
		nil,
		RuntimeExecutables{ShellRuntime: func() (ShellRuntime, error) {
			resolutions++
			return ShellRuntime{}, fmt.Errorf("profile failed")
		}},
	)
	if _, err := catalog.Workspace(config.ResolvedAgentToolSettings{
		config.AgentToolWorkspaceRead: true,
	}); err != nil {
		t.Fatal(err)
	}
	if resolutions != 0 {
		t.Fatalf("disabled shell resolved the login environment %d times", resolutions)
	}
	if _, err := catalog.Workspace(config.ResolvedAgentToolSettings{
		config.AgentToolShell: true,
	}); err == nil || !strings.Contains(err.Error(), "profile failed") {
		t.Fatalf("enabled shell error = %v", err)
	}
	if resolutions != 1 {
		t.Fatalf("enabled shell resolved the login environment %d times, want 1", resolutions)
	}
}

func TestCatalogWorkspaceBindsProductResultLimitToDefinitionsAndAdapters(t *testing.T) {
	workspace := t.TempDir()
	content := strings.Repeat("a bounded line of workspace content\n", 200)
	if err := os.WriteFile(workspace+string(os.PathSeparator)+"large.txt", []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog := NewCatalog(
		&config.Config{Workspace: workspace, AgentToolResultLimitKB: 1},
		nil,
		RuntimeExecutables{},
	)
	definitions, err := catalog.Workspace(config.ResolvedAgentToolSettings{
		config.AgentToolWorkspaceRead:  true,
		config.AgentToolWorkspaceWrite: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantNames := map[string]bool{"read": true, "glob": true, "grep": true, "write": true, "edit": true}
	var readDefinition *agent.ToolDefinition
	for index := range definitions {
		definition := &definitions[index]
		info, infoErr := definition.Tool.Info(context.Background())
		if infoErr != nil {
			t.Fatal(infoErr)
		}
		if !wantNames[info.Name] {
			continue
		}
		delete(wantNames, info.Name)
		if definition.Descriptor.MaxResultBytes != 1024 {
			t.Fatalf("%s max result bytes = %d", info.Name, definition.Descriptor.MaxResultBytes)
		}
		if info.Name == "read" {
			readDefinition = definition
		}
	}
	if len(wantNames) != 0 || readDefinition == nil {
		t.Fatalf("missing bounded workspace tools: %#v", wantNames)
	}
	result, err := runToolForTest(context.Background(), readDefinition, `{"path":"large.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, `"status":"partial"`) || !strings.Contains(result, `"truncated":true`) ||
		!strings.Contains(result, `"next_offset"`) {
		t.Fatalf("read did not expose product-limit continuation: %q", result)
	}
}

func descriptorCapabilityForTool(t *testing.T, definitions []agent.ToolDefinition, name string) string {
	t.Helper()
	for _, definition := range definitions {
		info, err := definition.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name == name {
			return definition.Descriptor.Capability
		}
	}
	t.Fatalf("tool %q not found", name)
	return ""
}

func TestCatalogWorkspaceUsesHostRipgrepExecutable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("DENOVA_TEST_CATALOG_RIPGREP_HELPER", "1")
	catalog := NewCatalog(
		&config.Config{Workspace: t.TempDir()},
		nil,
		RuntimeExecutables{Ripgrep: os.Args[0]},
	)
	filesystemTools, err := catalog.Workspace(config.ResolvedAgentToolSettings{config.AgentToolWorkspaceRead: true})
	if err != nil {
		t.Fatal(err)
	}
	var grep *agent.ToolDefinition
	for _, candidate := range filesystemTools {
		info, infoErr := candidate.Tool.Info(context.Background())
		if infoErr != nil {
			t.Fatal(infoErr)
		}
		if info.Name == "grep" {
			selected := candidate
			grep = &selected
			break
		}
	}
	if grep == nil {
		t.Fatal("catalog did not expose invokable grep")
	}
	result, err := runToolForTest(context.Background(), grep, `{"command":"rg dragon"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, `"schema":"workspace.search.v1"`) || !strings.Contains(result, "chapters/one.md") {
		t.Fatalf("grep result = %q", result)
	}
}

func TestCatalogDefaultGrepBudgetPaginatesContextBlowingOutput(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("DENOVA_TEST_CATALOG_RIPGREP_HELPER", "1")
	t.Setenv("DENOVA_TEST_CATALOG_RIPGREP_LARGE", "1")
	catalog := NewCatalog(
		&config.Config{Workspace: t.TempDir()},
		nil,
		RuntimeExecutables{Ripgrep: os.Args[0]},
	)
	definitions, err := catalog.Workspace(config.ResolvedAgentToolSettings{config.AgentToolWorkspaceRead: true})
	if err != nil {
		t.Fatal(err)
	}
	var grep *agent.ToolDefinition
	for index := range definitions {
		info, infoErr := definitions[index].Tool.Info(context.Background())
		if infoErr != nil {
			t.Fatal(infoErr)
		}
		if info.Name == "grep" {
			grep = &definitions[index]
			break
		}
	}
	if grep == nil {
		t.Fatal("catalog did not expose invokable grep")
	}
	wantLimit := config.DefaultAgentToolResultLimitKB * 1024
	if grep.Descriptor.MaxResultBytes != wantLimit {
		t.Fatalf("grep model-result budget = %d, want %d", grep.Descriptor.MaxResultBytes, wantLimit)
	}
	result, err := runToolForTest(context.Background(), grep, `{"command":"rg dragon"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) > wantLimit {
		t.Fatalf("grep result = %d bytes, want at most %d", len(result), wantLimit)
	}
	if !strings.Contains(result, `"status":"partial"`) ||
		!strings.Contains(result, `"truncated":true`) ||
		!strings.Contains(result, `"next_cursor"`) {
		t.Fatalf("grep did not expose a bounded continuation: %q", result[:min(len(result), 1024)])
	}
}
