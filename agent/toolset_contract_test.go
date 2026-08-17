package agent

import (
	"context"
	"reflect"
	"testing"
)

func mustStaticTools(t testing.TB, definitions ...ToolDefinition) Toolset {
	t.Helper()
	set, err := StaticTools(definitions...)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestStaticToolsImplementationIdentityChangesRestoreWithoutChangingModelContract(t *testing.T) {
	tool, err := InferTool("identity_contract", "identity contract", func(context.Context, struct{}) (string, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	base := testToolDefinition(tool)
	firstDefinition := base
	firstDefinition.ImplementationIdentity = CapabilityIdentity{Kind: "test.tool-implementation", Version: 1, ConfigHash: "one"}
	secondDefinition := base
	secondDefinition.ImplementationIdentity = CapabilityIdentity{Kind: "test.tool-implementation", Version: 1, ConfigHash: "two"}
	baseIdentity := CapabilityIdentity{Kind: "test.static-implementation", Version: 1}
	first, err := StaticToolsIdentified(baseIdentity, firstDefinition)
	if err != nil {
		t.Fatal(err)
	}
	second, err := StaticToolsIdentified(baseIdentity, secondDefinition)
	if err != nil {
		t.Fatal(err)
	}
	if first.Identity() == second.Identity() {
		t.Fatal("tool implementation identity did not change Toolset behavior identity")
	}
	firstDefinitions, err := first.PrepareTools(context.Background(), ToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	secondDefinitions, err := second.PrepareTools(context.Background(), ToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	firstRegistry, err := NewRegistry(context.Background(), firstDefinitions...)
	if err != nil {
		t.Fatal(err)
	}
	secondRegistry, err := NewRegistry(context.Background(), secondDefinitions...)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstRegistry.Snapshots(), secondRegistry.Snapshots()) {
		t.Fatal("model-visible tool contract changed with implementation-only identity")
	}
}

func mustStaticToolsIdentified(t testing.TB, identity CapabilityIdentity, definitions ...ToolDefinition) Toolset {
	t.Helper()
	set, err := StaticToolsIdentified(identity, definitions...)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

type invalidIdentityToolset struct{}

func (invalidIdentityToolset) Identity() CapabilityIdentity { return CapabilityIdentity{} }

func (invalidIdentityToolset) PrepareTools(context.Context, ToolRequest) ([]ToolDefinition, error) {
	return nil, nil
}

func TestCombineToolsetsRejectsInvalidChildIdentity(t *testing.T) {
	valid, err := StaticToolsIdentified(CapabilityIdentity{Kind: "test.valid-tools", Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CombineToolsets(
		valid,
		invalidIdentityToolset{},
	); err == nil {
		t.Fatal("CombineToolsets wrapped an invalid child identity")
	}
}

func TestStaticToolsIdentifiedFailsClosedOnInvalidIdentityOrDefinition(t *testing.T) {
	tests := []struct {
		identity    CapabilityIdentity
		definitions []ToolDefinition
	}{
		{identity: CapabilityIdentity{}},
		{identity: CapabilityIdentity{Kind: "test.invalid-definition", Version: 1}, definitions: []ToolDefinition{{}}},
	}
	for _, test := range tests {
		if set, err := StaticToolsIdentified(test.identity, test.definitions...); err == nil || set != nil {
			t.Fatalf("invalid static Toolset constructed: set=%#v error=%v", set, err)
		}
	}
}
