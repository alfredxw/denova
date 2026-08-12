package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type testSkillSource struct{}

func (testSkillSource) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "skills.test", Version: 1}
}

func (testSkillSource) List(_ context.Context, query SkillQuery) ([]Skill, error) {
	return []Skill{{
		Ref:  SkillRef{Source: "workspace", ID: "outline"},
		Name: "outline", Description: "Plan a durable outline for " + query.Query,
	}}, nil
}

func (testSkillSource) Read(_ context.Context, ref SkillRef) (SkillContent, error) {
	if ref.ID == "missing" {
		return SkillContent{}, errors.New("skill not found")
	}
	return SkillContent{
		Skill: Skill{Ref: ref, Name: ref.ID}, Revision: "sha256:one", Instructions: "Keep scene causality explicit.",
	}, nil
}

func TestSkillsListsAndBatchReadsWithPartialOutcomes(t *testing.T) {
	toolset, err := Skills(testSkillSource{})
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := toolset.PrepareTools(context.Background(), agent.ToolRequest{})
	if err != nil || len(definitions) != 1 {
		t.Fatalf("Skills definitions=%d error=%v", len(definitions), err)
	}
	if definitions[0].Descriptor.Recovery != agent.ToolRecoveryReadOnly ||
		definitions[0].Descriptor.MutationScope != agent.ToolMutationNone {
		t.Fatalf("Skills descriptor=%#v", definitions[0].Descriptor)
	}

	listed, err := definitions[0].Tool.Run(context.Background(), `{"action":"list","query":"chapter","limit":10}`)
	if err != nil || !strings.Contains(listed.ModelContent, `"name":"outline"`) ||
		!strings.Contains(listed.ModelContent, `"source":"workspace"`) {
		t.Fatalf("Skills list=%#v error=%v", listed, err)
	}
	read, err := definitions[0].Tool.Run(context.Background(), `{"action":"read","refs":[{"source":"workspace","id":"outline"},{"source":"workspace","id":"missing"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"instructions":"Keep scene causality explicit."`, `"id":"missing"`, `"error":"skill not found"`} {
		if !strings.Contains(read.ModelContent, want) {
			t.Fatalf("Skills batch result %q does not contain %q", read.ModelContent, want)
		}
	}
}
