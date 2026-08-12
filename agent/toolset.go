package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type staticToolset struct {
	definitions []ToolDefinition
	identity    CapabilityIdentity
	// durable reports whether behavior beyond the provider-visible schemas is
	// covered by either the Toolset identity or every definition's explicit
	// ImplementationIdentity. Schema-only StaticTools remain convenient for a
	// one-shot in-memory Agent but must not claim safe cold recovery.
	durable bool
}

// StaticTools constructs a schema-identified Toolset from fixed definitions.
// Use StaticToolsIdentified when implementations depend on host behavior or
// configuration not fully represented by their immutable tool contracts.
func StaticTools(definitions ...ToolDefinition) (Toolset, error) {
	return newStaticTools(CapabilityIdentity{Kind: "tools.static", Version: 1}, false, definitions...)
}

// StaticToolsIdentified adds an implementation/configuration identity when a
// stable schema alone cannot describe tool semantics.
func StaticToolsIdentified(identity CapabilityIdentity, definitions ...ToolDefinition) (Toolset, error) {
	return newStaticTools(identity, true, definitions...)
}

func newStaticTools(identity CapabilityIdentity, explicitlyIdentified bool, definitions ...ToolDefinition) (Toolset, error) {
	cloned := append([]ToolDefinition(nil), definitions...)
	if err := identity.validate("static Toolset"); err != nil {
		return nil, err
	}
	registry, err := NewRegistry(context.Background(), cloned...)
	if err != nil {
		return nil, fmt.Errorf("construct static Toolset: %w", err)
	}
	encoded, err := json.Marshal(struct {
		Identity        CapabilityIdentity
		Implementations []CapabilityIdentity
		Tools           []ToolDefinitionSnapshot
	}{identity, toolImplementationIdentities(cloned), registry.Snapshots()})
	if err != nil {
		return nil, fmt.Errorf("encode static Toolset identity: %w", err)
	}
	hash := sha256.Sum256(encoded)
	return staticToolset{
		definitions: cloned,
		durable:     explicitlyIdentified || staticImplementationsIdentified(cloned),
		identity: CapabilityIdentity{
			Kind: identity.Kind, Version: identity.Version, ConfigHash: hex.EncodeToString(hash[:]),
		},
	}, nil
}

func staticImplementationsIdentified(definitions []ToolDefinition) bool {
	for index := range definitions {
		if err := definitions[index].ImplementationIdentity.validate("tool implementation"); err != nil {
			return false
		}
	}
	return true
}

func (toolset staticToolset) validatePersistentToolset() error {
	if toolset.durable {
		return nil
	}
	return fmt.Errorf("schema-only StaticTools cannot be used by a durable Agent; set every ToolDefinition.ImplementationIdentity or use StaticToolsIdentified")
}

func toolImplementationIdentities(definitions []ToolDefinition) []CapabilityIdentity {
	identities := make([]CapabilityIdentity, len(definitions))
	for index := range definitions {
		identities[index] = definitions[index].ImplementationIdentity
	}
	return identities
}

func (toolset staticToolset) Identity() CapabilityIdentity { return toolset.identity }

func (toolset staticToolset) PrepareTools(context.Context, ToolRequest) ([]ToolDefinition, error) {
	return append([]ToolDefinition(nil), toolset.definitions...), nil
}

type combinedToolset struct {
	sets     []Toolset
	identity CapabilityIdentity
}

// CombineToolsets preserves source order and validates duplicates when a cycle
// is prepared. Nil entries are ignored.
func CombineToolsets(sets ...Toolset) (Toolset, error) {
	filtered := make([]Toolset, 0, len(sets))
	identities := make([]CapabilityIdentity, 0, len(sets))
	for index, set := range sets {
		if set == nil {
			continue
		}
		identity := set.Identity()
		if err := identity.validate(fmt.Sprintf("combined Toolset %d", index)); err != nil {
			return nil, err
		}
		filtered = append(filtered, set)
		identities = append(identities, identity)
	}
	encoded, err := json.Marshal(identities)
	if err != nil {
		return nil, fmt.Errorf("encode combined Toolset identity: %w", err)
	}
	hash := sha256.Sum256(encoded)
	return combinedToolset{
		sets: filtered,
		identity: CapabilityIdentity{
			Kind: "tools.combined", Version: 1, ConfigHash: hex.EncodeToString(hash[:]),
		},
	}, nil
}

func (toolset combinedToolset) Identity() CapabilityIdentity { return toolset.identity }

func (toolset combinedToolset) PrepareTools(ctx context.Context, request ToolRequest) ([]ToolDefinition, error) {
	var definitions []ToolDefinition
	for index, set := range toolset.sets {
		prepared, err := set.PrepareTools(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("prepare Toolset %d: %w", index, err)
		}
		definitions = append(definitions, prepared...)
	}
	return definitions, nil
}
