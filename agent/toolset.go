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
}

// StaticTools constructs a Toolset from fixed definitions.
func StaticTools(definitions ...ToolDefinition) Toolset {
	return StaticToolsIdentified(CapabilityIdentity{Kind: "tools.static", Version: 1}, definitions...)
}

// StaticToolsIdentified adds an implementation/configuration identity when a
// stable schema alone cannot describe tool semantics.
func StaticToolsIdentified(identity CapabilityIdentity, definitions ...ToolDefinition) Toolset {
	cloned := append([]ToolDefinition(nil), definitions...)
	registry, _ := NewRegistry(context.Background(), cloned...)
	encoded, _ := json.Marshal(struct {
		Identity CapabilityIdentity
		Tools    []ToolDefinitionSnapshot
	}{identity, registry.Snapshots()})
	hash := sha256.Sum256(encoded)
	return staticToolset{
		definitions: cloned,
		identity: CapabilityIdentity{
			Kind: identity.Kind, Version: identity.Version, ConfigHash: hex.EncodeToString(hash[:]),
		},
	}
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
func CombineToolsets(sets ...Toolset) Toolset {
	filtered := make([]Toolset, 0, len(sets))
	identities := make([]CapabilityIdentity, 0, len(sets))
	for _, set := range sets {
		if set == nil {
			continue
		}
		filtered = append(filtered, set)
		identities = append(identities, set.Identity())
	}
	encoded, _ := json.Marshal(identities)
	hash := sha256.Sum256(encoded)
	return combinedToolset{
		sets: filtered,
		identity: CapabilityIdentity{
			Kind: "tools.combined", Version: 1, ConfigHash: hex.EncodeToString(hash[:]),
		},
	}
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
