package tools

import (
	"context"
	"testing"

	novaskills "denova/internal/agents/skills"
)

func TestSkillToolDescriptionIsCatalogIndependent(t *testing.T) {
	definition, err := NewSkill(context.Background(), novaskills.NewBackend(nil), 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	info, err := definition.Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Desc != skillToolDescription {
		t.Fatalf("Skill tool description = %q, want stable description %q", info.Desc, skillToolDescription)
	}
	if info.Name != "skill" {
		t.Fatalf("Skill tool name = %q, want skill", info.Name)
	}
}
