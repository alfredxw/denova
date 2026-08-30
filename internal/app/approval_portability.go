package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"denova/config"
	projectdomain "denova/internal/project"
)

const portableDataBackupDirectory = "portable-data-v4"

// portableApprovalRules projects user-owned approvals onto their final durable
// boundary. Managed Projects use ProjectID; external Projects deliberately keep
// a host path so moving or relinking them requires a new authorization.
func portableApprovalRules(records []projectdomain.Record, rules []config.AgentApprovalRule) ([]config.AgentApprovalRule, bool) {
	rules = config.NormalizeAgentApprovalRules(rules)
	if rules == nil {
		return nil, false
	}
	byID := make(map[string]projectdomain.Record, len(records))
	managed := make([]projectdomain.Record, 0, len(records))
	for _, record := range records {
		byID[strings.TrimSpace(record.ID)] = record
		if record.Location.Kind == projectdomain.LocationManaged {
			managed = append(managed, record)
		}
	}

	result := make([]config.AgentApprovalRule, 0, len(rules))
	changed := false
	for _, rule := range rules {
		original := rule
		if record, found := byID[rule.ProjectID]; found {
			switch record.Location.Kind {
			case projectdomain.LocationManaged:
				rule.Workspace = ""
			case projectdomain.LocationExternal:
				// A project-only rule cannot retain its security boundary after
				// the Project becomes external. Drop it rather than silently
				// authorizing the newly linked host directory.
				if rule.Workspace == "" {
					changed = true
					continue
				}
			}
		} else if rule.ProjectID == "" && rule.Workspace != "" {
			if projectID, found := managedProjectForApprovalPath(managed, rule.Workspace); found {
				rule.ProjectID = projectID
				rule.Workspace = ""
			}
		}
		if rule != original {
			changed = true
		}
		result = append(result, rule)
	}
	return result, changed
}

func managedProjectForApprovalPath(records []projectdomain.Record, workspace string) (string, bool) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", false
	}
	workspaceSlash := strings.TrimSuffix(strings.ReplaceAll(workspace, `\`, "/"), "/")
	match := ""
	for _, record := range records {
		currentSlash := strings.TrimSuffix(strings.ReplaceAll(record.WorkspacePath, `\`, "/"), "/")
		locationSlash := strings.Trim(strings.ReplaceAll(record.Location.Path, `\`, "/"), "/")
		exact := currentSlash != "" && strings.EqualFold(workspaceSlash, currentSlash)
		movedRoot := locationSlash != "" && strings.HasSuffix(strings.ToLower(workspaceSlash), "/"+strings.ToLower(locationSlash))
		if !exact && !movedRoot {
			continue
		}
		if match != "" && match != record.ID {
			return "", false
		}
		match = record.ID
	}
	return match, match != ""
}

func canonicalApprovalRuleForPersistence(registry *projectdomain.Registry, rule config.AgentApprovalRule) config.AgentApprovalRule {
	if registry == nil || strings.TrimSpace(rule.ProjectID) == "" {
		return rule
	}
	record, err := registry.Get(rule.ProjectID)
	if err == nil && record.Location.Kind == projectdomain.LocationManaged {
		rule.Workspace = ""
	}
	return rule
}

// migratePortableApprovalSettings is the one-time v0.3.3/final-schema bridge
// for approvals. It preserves the original user config before removing managed
// absolute paths and re-evaluates the current file inside the atomic mutation.
func migratePortableApprovalSettings(dataDir string, registry *projectdomain.Registry) error {
	if registry == nil {
		return nil
	}
	records, err := registry.List(true)
	if err != nil {
		return fmt.Errorf("list Projects for approval migration: %w", err)
	}
	settingsPath := config.UserConfigPath(dataDir)
	settings, err := config.ReadSettingsFile(settingsPath)
	if err != nil {
		return err
	}
	_, changed := portableApprovalRules(records, settings.AgentApprovalRules)
	if !changed {
		return nil
	}
	backupPath, err := preservePortableApprovalBackup(dataDir, settingsPath)
	if err != nil {
		return err
	}
	if _, err := config.MutateSettingsFile(settingsPath, "", func(current config.Settings) (config.Settings, error) {
		migrated, _ := portableApprovalRules(records, current.AgentApprovalRules)
		current.AgentApprovalRules = migrated
		return config.PrepareUserSettingsForWrite(current, current)
	}); err != nil {
		return fmt.Errorf("persist portable approval rules: %w", err)
	}
	slog.Info("[internal/app/approval_portability.go] migrated managed Project approval rules",
		"settings_path", settingsPath, "backup_path", backupPath)
	return nil
}

func preservePortableApprovalBackup(dataDir, sourcePath string) (string, error) {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", fmt.Errorf("read approval settings backup source %s: %w", sourcePath, err)
	}
	digest := sha256.Sum256(content)
	backupDir := filepath.Join(dataDir, "backups", portableDataBackupDirectory)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("create approval settings backup directory: %w", err)
	}
	backupPath := filepath.Join(backupDir, "config-"+hex.EncodeToString(digest[:8])+".toml")
	file, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		if _, writeErr := file.Write(content); writeErr != nil {
			_ = file.Close()
			return "", fmt.Errorf("write approval settings backup %s: %w", backupPath, writeErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			return "", fmt.Errorf("close approval settings backup %s: %w", backupPath, closeErr)
		}
		return backupPath, nil
	}
	if !os.IsExist(err) {
		return "", fmt.Errorf("create approval settings backup %s: %w", backupPath, err)
	}
	existing, readErr := os.ReadFile(backupPath)
	if readErr != nil {
		return "", fmt.Errorf("verify approval settings backup %s: %w", backupPath, readErr)
	}
	if !bytes.Equal(existing, content) {
		return "", fmt.Errorf("approval settings backup collision at %s", backupPath)
	}
	return backupPath, nil
}
