package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"

	agent "github.com/alfredxw/denova/agent"
)

// Combine preserves Toolset order. Duplicate tool names are rejected by Agent
// when a cycle is prepared.
func Combine(toolsets ...agent.Toolset) agent.Toolset { return agent.CombineToolsets(toolsets...) }

// Single selects one independently constructed common tool.
func Single(identity string, definition agent.ToolDefinition) agent.Toolset {
	return agent.StaticToolsIdentified(agent.CapabilityIdentity{Kind: identity, Version: 1}, definition)
}

type WorkspaceAccess string

const (
	WorkspaceReadOnly  WorkspaceAccess = "read_only"
	WorkspaceReadWrite WorkspaceAccess = "read_write"
)

// WorkspaceConfig binds the local read/search adapters and an optional
// host-owned mutation adapter into one cohesive Toolset. Read-write mode never
// bypasses the MutationAdapter's review, receipt, or concurrency semantics.
type WorkspaceConfig struct {
	Root              string
	Access            WorkspaceAccess
	Mutation          MutationAdapter
	RipgrepExecutable string
	Limits            WorkspaceLimits
}

// Workspace constructs read, glob, grep and, in read-write mode, write/edit.
func Workspace(config WorkspaceConfig) (agent.Toolset, error) {
	if config.Access == "" {
		config.Access = WorkspaceReadOnly
	}
	if config.Access != WorkspaceReadOnly && config.Access != WorkspaceReadWrite {
		return nil, fmt.Errorf("unsupported workspace access %q", config.Access)
	}
	workspace, err := OpenWorkspaceWithOptions(WorkspaceOptions{
		Root: config.Root, RipgrepExecutable: config.RipgrepExecutable, Limits: config.Limits,
	})
	if err != nil {
		return nil, err
	}
	localText, err := LocalTextAdapter(workspace)
	if err != nil {
		return nil, err
	}
	directory, err := DirectoryAdapter(workspace)
	if err != nil {
		return nil, err
	}
	read, err := Read([]ReadAdapter{localText, directory})
	if err != nil {
		return nil, err
	}
	glob, err := Glob(workspace)
	if err != nil {
		return nil, err
	}
	grep, err := Grep(workspace)
	if err != nil {
		return nil, err
	}
	definitions := []agent.ToolDefinition{read, glob, grep}
	if config.Access == WorkspaceReadWrite {
		if config.Mutation == nil {
			return nil, errors.New("read-write workspace requires a MutationAdapter")
		}
		identified, ok := config.Mutation.(IdentifiedMutationAdapter)
		if !ok || identified.Identity().Kind == "" || identified.Identity().Version == 0 {
			return nil, errors.New("read-write workspace MutationAdapter requires a stable Identity")
		}
		write, err := Write(config.Mutation)
		if err != nil {
			return nil, err
		}
		edit, err := Edit(config.Mutation)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, write, edit)
	}
	var mutation agent.CapabilityIdentity
	if identified, ok := config.Mutation.(IdentifiedMutationAdapter); ok {
		mutation = identified.Identity()
	}
	identity := toolsetIdentity("tools.workspace", struct {
		Root     string
		Access   WorkspaceAccess
		Limits   WorkspaceLimits
		Mutation agent.CapabilityIdentity
	}{Root: workspace.Root(), Access: config.Access, Limits: workspace.Limits(), Mutation: mutation})
	return agent.StaticToolsIdentified(identity, definitions...), nil
}

type ShellConfig struct {
	Runner CommandRunner
	Shells []ShellKind
}

// Shell constructs bash and/or pwsh over an injected process Adapter. When no
// shell is selected, the current platform's ordinary shell is used.
func Shell(config ShellConfig) (agent.Toolset, error) {
	if config.Runner == nil {
		return nil, errors.New("shell Toolset requires a CommandRunner")
	}
	identified, ok := config.Runner.(IdentifiedCommandRunner)
	if !ok || identified.Identity().Kind == "" || identified.Identity().Version == 0 {
		return nil, errors.New("shell CommandRunner requires a stable Identity")
	}
	shells := append([]ShellKind(nil), config.Shells...)
	if len(shells) == 0 {
		if runtime.GOOS == "windows" {
			shells = []ShellKind{ShellPwsh}
		} else {
			shells = []ShellKind{ShellBash}
		}
	}
	definitions := make([]agent.ToolDefinition, 0, len(shells))
	seen := make(map[ShellKind]bool)
	for _, shell := range shells {
		if seen[shell] {
			continue
		}
		seen[shell] = true
		var definition agent.ToolDefinition
		var err error
		switch shell {
		case ShellBash:
			definition, err = Bash(config.Runner)
		case ShellPwsh:
			definition, err = Pwsh(config.Runner)
		default:
			return nil, fmt.Errorf("unsupported shell %q", shell)
		}
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, definition)
	}
	return agent.StaticToolsIdentified(toolsetIdentity("tools.shell", struct {
		Runner agent.CapabilityIdentity
		Shells []ShellKind
	}{identified.Identity(), shells}), definitions...), nil
}

func toolsetIdentity(kind string, config any) agent.CapabilityIdentity {
	encoded, _ := json.Marshal(config)
	digest := sha256.Sum256(encoded)
	return agent.CapabilityIdentity{Kind: kind, Version: 1, ConfigHash: hex.EncodeToString(digest[:])}
}
