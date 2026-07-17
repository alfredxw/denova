package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"denova/internal/book"
)

type PostRunVerification struct {
	Status    string                     `json:"status"`
	Checks    []PostRunVerificationCheck `json:"checks"`
	Warnings  []string                   `json:"warnings,omitempty"`
	Mutations int                        `json:"mutations"`
}

type PostRunVerificationCheck struct {
	Type    string `json:"type"`
	Target  string `json:"target,omitempty"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func VerifyPostRunMutations(bookService *book.Service, mutations []ToolMutation) PostRunVerification {
	result := PostRunVerification{
		Status:    "skipped",
		Mutations: len(mutations),
	}
	if len(mutations) == 0 {
		result.Checks = append(result.Checks, PostRunVerificationCheck{Type: "mutation_scan", Status: "skipped", Message: "no workspace mutations observed"})
		return result
	}
	if bookService == nil || strings.TrimSpace(bookService.Workspace()) == "" {
		result.Status = "warning"
		result.Warnings = append(result.Warnings, "workspace unavailable")
		result.Checks = append(result.Checks, PostRunVerificationCheck{Type: "workspace", Status: "warning", Message: "workspace unavailable"})
		return result
	}
	for _, mutation := range mutations {
		result.Checks = append(result.Checks, verifyMutation(bookService, mutation)...)
	}
	result.Status = "ok"
	for _, check := range result.Checks {
		if check.Status == "warning" || check.Status == "error" {
			result.Status = "warning"
			if check.Message != "" {
				result.Warnings = append(result.Warnings, check.Message)
			}
		}
	}
	return result
}

func verifyMutation(bookService *book.Service, mutation ToolMutation) []PostRunVerificationCheck {
	checks := []PostRunVerificationCheck{}
	if mutation.Source == ToolSourceLore || mutation.ToolName == "write_lore_items" {
		return append(checks, verifyLoreMutation(bookService.Workspace(), mutation)...)
	}
	target := strings.TrimSpace(filepath.ToSlash(mutation.Target))
	if target == "" {
		return []PostRunVerificationCheck{{
			Type:    "target",
			Status:  "warning",
			Message: fmt.Sprintf("%s did not expose a target path", mutation.ToolName),
		}}
	}
	abs, relativeTarget, err := resolveVerifiedMutationTarget(bookService.Workspace(), target)
	if err != nil {
		return []PostRunVerificationCheck{{
			Type:    "path",
			Target:  target,
			Status:  "warning",
			Message: err.Error(),
		}}
	}
	info, statErr := os.Stat(abs)
	deletion := isDeletionMutation(mutation.ToolName)
	switch {
	case deletion && errors.Is(statErr, os.ErrNotExist):
		checks = append(checks, PostRunVerificationCheck{Type: "path_exists", Target: target, Status: "ok", Message: "deleted target is absent"})
	case deletion && statErr == nil:
		checks = append(checks, PostRunVerificationCheck{Type: "path_exists", Target: target, Status: "warning", Message: "delete-like tool returned but target still exists"})
	case statErr != nil:
		checks = append(checks, PostRunVerificationCheck{Type: "path_exists", Target: target, Status: "warning", Message: statErr.Error()})
	default:
		kind := "file"
		if info.IsDir() {
			kind = "directory"
		}
		checks = append(checks, PostRunVerificationCheck{Type: "path_exists", Target: target, Status: "ok", Message: kind + " exists"})
	}
	if strings.HasPrefix(relativeTarget, "chapters/") && !isChapterContentPath(relativeTarget) {
		checks = append(checks, PostRunVerificationCheck{Type: "chapter_path", Target: target, Status: "warning", Message: "chapter writes should use .md or .txt files under chapters/"})
	}
	if relativeTarget == "setting/character-states.md" || relativeTarget == "progress.md" {
		checks = append(checks, PostRunVerificationCheck{Type: "state_sync", Target: target, Status: "ok", Message: "tracked writing-state file"})
	}
	return checks
}

func resolveVerifiedMutationTarget(workspace, target string) (absolutePath, relativeTarget string, err error) {
	workspace = strings.TrimSpace(workspace)
	target = strings.TrimSpace(target)
	if !filepath.IsAbs(target) {
		absolutePath, err = book.SafePath(workspace, target)
		return absolutePath, filepath.ToSlash(filepath.Clean(target)), err
	}

	absoluteWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return "", "", fmt.Errorf("解析 workspace 路径失败: %w", err)
	}
	cleanTarget := filepath.Clean(target)
	relativeTarget, err = filepath.Rel(filepath.Clean(absoluteWorkspace), cleanTarget)
	if err != nil || relativeTarget == "." || relativeTarget == ".." || strings.HasPrefix(relativeTarget, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeTarget) {
		return "", "", errors.New("路径不在 workspace 范围内")
	}
	absolutePath, err = book.SafePath(absoluteWorkspace, filepath.ToSlash(relativeTarget))
	if err != nil {
		return "", "", err
	}
	return absolutePath, filepath.ToSlash(relativeTarget), nil
}

func verifyLoreMutation(workspace string, mutation ToolMutation) []PostRunVerificationCheck {
	store := book.NewLoreStore(workspace)
	items, err := store.List()
	if err != nil {
		return []PostRunVerificationCheck{{Type: "lore_store", Status: "warning", Message: err.Error()}}
	}
	byID := make(map[string]book.LoreItem, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	checks := []PostRunVerificationCheck{}
	for _, id := range mutation.LoreItemIDs {
		item, ok := byID[id]
		if !ok {
			checks = append(checks, PostRunVerificationCheck{Type: "lore_item", Target: id, Status: "warning", Message: "changed lore item not found after write"})
			continue
		}
		if strings.TrimSpace(item.BriefDescription) == "" {
			checks = append(checks, PostRunVerificationCheck{Type: "lore_brief", Target: id, Status: "warning", Message: "lore item is missing brief_description"})
			continue
		}
		checks = append(checks, PostRunVerificationCheck{Type: "lore_brief", Target: id, Status: "ok", Message: "brief_description present"})
	}
	for _, id := range mutation.DeletedLoreItemIDs {
		if _, ok := byID[id]; ok {
			checks = append(checks, PostRunVerificationCheck{Type: "lore_delete", Target: id, Status: "warning", Message: "deleted lore item still exists"})
			continue
		}
		checks = append(checks, PostRunVerificationCheck{Type: "lore_delete", Target: id, Status: "ok", Message: "deleted lore item is absent"})
	}
	if len(checks) == 0 {
		checks = append(checks, PostRunVerificationCheck{Type: "lore_write", Status: "ok", Message: "lore write completed"})
	}
	return checks
}

func isDeletionMutation(toolName string) bool {
	name := normalizeToolName(toolName)
	return strings.Contains(name, "delete") || strings.Contains(name, "remove")
}

func isChapterContentPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".txt"
}
