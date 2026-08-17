package permission

import (
	"context"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type unidentifiedRules struct{}

func (unidentifiedRules) Identity() agent.CapabilityIdentity          { return agent.CapabilityIdentity{} }
func (unidentifiedRules) Allowed(context.Context, Rule) (bool, error) { return false, nil }
func (unidentifiedRules) Remember(context.Context, Rule) error        { return nil }

func TestRuleBackedPolicyRequiresOneIdentifiedStore(t *testing.T) {
	if _, err := CodingWithRules(nil); err == nil {
		t.Fatal("nil RuleStore was accepted")
	}
	if _, err := CodingWithRules(unidentifiedRules{}); err == nil {
		t.Fatal("unidentified RuleStore was accepted")
	}
	policy, err := CodingWithRules(MemoryRules())
	if err != nil {
		t.Fatal(err)
	}
	if policy.Identity().ConfigHash == "" {
		t.Fatalf("rule-backed Policy identity=%#v", policy.Identity())
	}
}
