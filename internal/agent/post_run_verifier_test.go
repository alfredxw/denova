package agent

import (
	"testing"

	"nova/internal/book"
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
