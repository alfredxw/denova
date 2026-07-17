package agent

import (
	"path/filepath"
	"strings"
	"testing"

	"denova/internal/book"
)

func TestVerifyPostRunMutationsAcceptsIllustrationMetaWrite(t *testing.T) {
	workspace := t.TempDir()
	bookService := book.NewService(workspace)
	if err := bookService.WriteFile("assets/illustrations/ch01/run/meta.json", "{}"); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	result := VerifyPostRunMutations(bookService, []ToolMutation{{
		ToolName:          generateImageToolName,
		Target:            "assets/illustrations/ch01/run/meta.json",
		Source:            ToolSourceImage,
		RequiresPostCheck: true,
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
		ToolName:          "write_file",
		Target:            absoluteTarget,
		Source:            ToolSourceWrite,
		RequiresPostCheck: true,
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
		ToolName:          "write_file",
		Target:            outsideTarget,
		Source:            ToolSourceWrite,
		RequiresPostCheck: true,
	}})

	if result.Status != "warning" || len(result.Checks) != 1 || result.Checks[0].Type != "path" {
		t.Fatalf("outside absolute path must fail containment verification: %#v", result)
	}
	if !strings.Contains(result.Checks[0].Message, "workspace") {
		t.Fatalf("outside-path diagnostic should explain the boundary: %#v", result.Checks[0])
	}
}
