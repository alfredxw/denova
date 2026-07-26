package agents

import (
	"path/filepath"
	"strings"
	"testing"

	producttools "denova/internal/agents/tools"
	"denova/internal/book"
)

func TestVerifyPostRunMutationsAcceptsIllustrationMetaWrite(t *testing.T) {
	workspace := t.TempDir()
	bookService := book.NewService(workspace)
	if err := bookService.WriteFile("assets/illustrations/ch01/run/meta.json", "{}"); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	result := VerifyPostRunMutations(bookService, []ToolMutation{{
		ToolName:      producttools.GenerateImageToolName,
		Target:        "assets/illustrations/ch01/run/meta.json",
		Source:        ToolSourceImage,
		MutationScope: ToolMutationWorkspace,
		PostCheck:     ToolPostCheckWorkspaceChange,
	}})
	if result.Status != "ok" {
		t.Fatalf("verification status = %s checks=%#v warnings=%#v", result.Status, result.Checks, result.Warnings)
	}
}

func TestVerifyPostRunMutationsAcceptsAbsolutePathInsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	bookService := book.NewService(workspace)
	const relativeTarget = "interactive/stories/story-1/director/main/director.md"
	if err := bookService.WriteFile(relativeTarget, "# Director Plan"); err != nil {
		t.Fatalf("write director plan: %v", err)
	}

	absoluteTarget := filepath.Join(workspace, filepath.FromSlash(relativeTarget))
	result := VerifyPostRunMutations(bookService, []ToolMutation{{
		ToolName:      "write",
		Target:        absoluteTarget,
		Source:        ToolSourceWrite,
		MutationScope: ToolMutationWorkspace,
		PostCheck:     ToolPostCheckWorkspaceChange,
	}})

	if result.Status != "ok" {
		t.Fatalf("workspace-contained absolute path should verify: status=%s checks=%#v warnings=%#v", result.Status, result.Checks, result.Warnings)
	}
	if len(result.Checks) != 1 || result.Checks[0].Status != "ok" || result.Checks[0].Target != absoluteTarget {
		t.Fatalf("absolute target should remain visible in verification output: %#v", result.Checks)
	}
}

func TestVerifyPostRunMutationsRejectsAbsolutePathOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	bookService := book.NewService(workspace)
	outsideTarget := filepath.Join(workspace+"-outside", "director.md")

	result := VerifyPostRunMutations(bookService, []ToolMutation{{
		ToolName:      "write",
		Target:        outsideTarget,
		Source:        ToolSourceWrite,
		MutationScope: ToolMutationWorkspace,
		PostCheck:     ToolPostCheckWorkspaceChange,
	}})

	if result.Status != "warning" || len(result.Checks) != 1 || result.Checks[0].Type != "path" {
		t.Fatalf("outside absolute path must fail containment verification: %#v", result)
	}
	if !strings.Contains(result.Checks[0].Message, "workspace") {
		t.Fatalf("outside-path diagnostic should explain the boundary: %#v", result.Checks[0])
	}
}

func TestReadOnlyMutationReceiptAddsWarningWithoutRejectingMutation(t *testing.T) {
	verification := applyToolMutationWarnings(
		RunOptions{WriteMode: RunWriteModeReadOnly, WriteScope: "none"},
		PostRunVerification{Status: "ok", Mutations: 1},
		nil,
	)
	if verification.Status != "warning" || len(verification.Warnings) != 1 ||
		!strings.Contains(verification.Warnings[0], "changes were retained") {
		t.Fatalf("read-only verification = %#v", verification)
	}
}
