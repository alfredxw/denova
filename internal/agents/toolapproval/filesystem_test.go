package toolapproval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

func TestFilesystemReadPermissionSeparatesProjectExternalAndFullAccess(t *testing.T) {
	parent := t.TempDir()
	project := filepath.Join(parent, "project")
	external := filepath.Join(parent, "shared")
	for _, directory := range []string{project, external} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	projectFile := filepath.Join(project, "inside.md")
	externalFile := filepath.Join(external, "reference.md")
	for path, content := range map[string]string{projectFile: "inside", externalFile: "outside"} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	descriptor := agent.ToolDescriptor{
		Source: agent.ToolSourceRead, MutationScope: agent.ToolMutationNone,
		Capability: config.AgentToolFilesystemRead,
	}
	request := func(mode config.AgentApprovalMode, toolName, arguments string, rules []config.AgentApprovalRule) Decision {
		return Evaluate(Request{
			Mode: mode, ProjectID: "project-filesystem", Workspace: project,
			ToolName: toolName, Arguments: arguments, Descriptor: descriptor, Rules: rules,
		})
	}

	inside := request(config.AgentApprovalAsk, "read", marshalFilesystemTestArguments(t, map[string]any{"path": "inside.md"}), nil)
	if inside.Action != ActionAllow || inside.RuleID != "project_filesystem_read" {
		t.Fatalf("Project read = %#v", inside)
	}
	externalArguments := marshalFilesystemTestArguments(t, map[string]any{"path": externalFile})
	for _, mode := range []config.AgentApprovalMode{config.AgentApprovalAsk, config.AgentApprovalWrite} {
		decision := request(mode, "read", externalArguments, nil)
		if decision.Action != ActionPrompt || decision.RuleID != "external_filesystem_read" || decision.Remember == nil ||
			decision.Remember.Matcher != config.AgentApprovalMatcherFilesystem ||
			decision.Remember.ToolName != config.AgentApprovalFilesystemReadTool ||
			!strings.Contains(decision.Details, filepath.ToSlash(externalFile)) {
			t.Fatalf("external read in %s = %#v", mode, decision)
		}
	}
	full := request(config.AgentApprovalFullAccess, "read", externalArguments, nil)
	if full.Action != ActionAllow || full.RuleID != "external_filesystem_read_full_access" {
		t.Fatalf("Full Access external read = %#v", full)
	}
}

func TestFilesystemReadRuleIsSharedByReadGlobAndGrepWithinExactRoot(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(parent, "project")
	external := filepath.Join(parent, "shared")
	other := filepath.Join(parent, "private")
	for _, directory := range []string{project, external, other} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{filepath.Join(external, "one.md"), filepath.Join(other, "secret.md")} {
		if err := os.WriteFile(path, []byte("dragon\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	descriptor := agent.ToolDescriptor{
		Source: agent.ToolSourceRead, MutationScope: agent.ToolMutationNone,
		Capability: config.AgentToolFilesystemRead,
	}
	request := func(toolName, arguments string, rules []config.AgentApprovalRule) Decision {
		return Evaluate(Request{
			Mode: config.AgentApprovalAsk, ProjectID: "project-filesystem", Workspace: project,
			ToolName: toolName, Arguments: arguments, Descriptor: descriptor, Rules: rules,
		})
	}
	pattern := filepath.ToSlash(filepath.Join(external, "**", "*.md"))
	globArguments := marshalFilesystemTestArguments(t, map[string]any{"paths": []string{pattern}})
	first := request("glob", globArguments, nil)
	if first.Action != ActionPrompt || first.Remember == nil || first.Remember.DisplayPattern != filepath.ToSlash(external)+"/**" {
		t.Fatalf("external glob proposal = %#v", first)
	}
	rule, err := NewWorkspaceRule(
		"project-filesystem", project, *first.Remember, ArgumentsHash(globArguments),
		first.Details, "", first.RuleID, time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []struct {
		name      string
		arguments string
	}{
		{name: "read", arguments: marshalFilesystemTestArguments(t, map[string]any{"path": filepath.Join(external, "one.md")})},
		{name: "glob", arguments: marshalFilesystemTestArguments(t, map[string]any{"paths": []string{filepath.Join(external, "one.md")}})},
		{name: "grep", arguments: marshalFilesystemTestArguments(t, map[string]any{"command": "rg dragon -- " + filesystemTestShellWord(external)})},
	} {
		decision := request(call.name, call.arguments, []config.AgentApprovalRule{rule})
		if decision.Action != ActionAllow || decision.RuleID != "remembered_filesystem_read" {
			t.Fatalf("remembered %s = %#v", call.name, decision)
		}
	}
	outside := request("read", marshalFilesystemTestArguments(t, map[string]any{"path": filepath.Join(other, "secret.md")}), []config.AgentApprovalRule{rule})
	if outside.Action != ActionPrompt || outside.RuleID != "external_filesystem_read" {
		t.Fatalf("unrelated external root inherited rule: %#v", outside)
	}
}

func marshalFilesystemTestArguments(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func filesystemTestShellWord(value string) string {
	return `'` + strings.ReplaceAll(value, `'`, `'\''`) + `'`
}
