package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"denova/config"
	projectdomain "denova/internal/project"
)

func TestPortableApprovalRulesUseProjectIDForManagedProjects(t *testing.T) {
	t.Parallel()
	dataRoot := t.TempDir()
	managedRoot := filepath.Join(dataRoot, "projects", "Portable Book")
	externalRoot := t.TempDir()
	for _, root := range []string{managedRoot, externalRoot} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	registry := projectdomain.NewRegistry(dataRoot)
	managed, err := registry.Add(managedRoot, projectdomain.TypeBook, "Portable Book")
	if err != nil {
		t.Fatal(err)
	}
	external, err := registry.Add(externalRoot, projectdomain.TypeBook, "External Book")
	if err != nil {
		t.Fatal(err)
	}
	records, err := registry.List(true)
	if err != nil {
		t.Fatal(err)
	}

	managedRule := portableApprovalTestRule("managed", managed.ID, managedRoot)
	legacyRoot := `Z:\moved\.denova\` + strings.ReplaceAll(managed.Location.Path, "/", `\`)
	legacyRule := portableApprovalTestRule("legacy", "", legacyRoot)
	externalRule := portableApprovalTestRule("external", external.ID, externalRoot)
	rules, changed := portableApprovalRules(records, []config.AgentApprovalRule{managedRule, legacyRule, externalRule})
	if !changed || len(rules) != 3 {
		t.Fatalf("portable rules changed=%v rules=%#v", changed, rules)
	}
	for _, index := range []int{0, 1} {
		if rules[index].ProjectID != managed.ID || rules[index].Workspace != "" {
			t.Fatalf("managed rule %d retained a host path: %#v", index, rules[index])
		}
	}
	if rules[2].ProjectID != external.ID || rules[2].Workspace != externalRoot {
		t.Fatalf("external rule lost its host boundary: %#v", rules[2])
	}
}

func TestMigratePortableApprovalSettingsBacksUpAndRemovesManagedRoot(t *testing.T) {
	t.Parallel()
	dataRoot := t.TempDir()
	managedRoot := filepath.Join(dataRoot, "projects", "Portable Book")
	if err := os.MkdirAll(managedRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := projectdomain.NewRegistry(dataRoot)
	record, err := registry.Add(managedRoot, projectdomain.TypeBook, "Portable Book")
	if err != nil {
		t.Fatal(err)
	}
	settingsPath := config.UserConfigPath(dataRoot)
	if err := config.WriteSettingsFile(settingsPath, config.Settings{
		AgentApprovalRules: []config.AgentApprovalRule{portableApprovalTestRule("managed", record.ID, managedRoot)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := migratePortableApprovalSettings(dataRoot, registry); err != nil {
		t.Fatal(err)
	}
	settings, err := config.ReadSettingsFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.AgentApprovalRules) != 1 || settings.AgentApprovalRules[0].ProjectID != record.ID || settings.AgentApprovalRules[0].Workspace != "" {
		t.Fatalf("migrated approval rules = %#v", settings.AgentApprovalRules)
	}
	backups, err := filepath.Glob(filepath.Join(dataRoot, "backups", portableDataBackupDirectory, "config-*.toml"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("approval backups=%#v err=%v", backups, err)
	}
	backup, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(backup), managedRoot) {
		t.Fatalf("backup did not preserve the previous workspace path: %s", backup)
	}
}

func portableApprovalTestRule(id, projectID, workspace string) config.AgentApprovalRule {
	return config.AgentApprovalRule{
		ID: "approval-" + id, Scope: config.AgentApprovalRuleWorkspace,
		ProjectID: projectID, Workspace: workspace, ToolName: "bash",
		Matcher: config.AgentApprovalMatcherShell, MatcherVersion: config.AgentApprovalRuleMatcherVersion,
		MatchKey: `["go","test"]`, DisplayPattern: "go test ...",
		ApprovedArgsHash: strings.Repeat("a", 64), ApprovedInput: "go test ./...",
		CreatedAt: time.Unix(100, 0).UTC(),
	}
}
