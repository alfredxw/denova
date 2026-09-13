// Package platform owns installed plugin capabilities and playable game releases.
// Package identities and frozen releases are shared mechanics; game instances,
// plugin activation scopes, and Product Session journals keep distinct lifetimes.
package platform

import (
	"encoding/json"
	"fmt"
	"time"
)

const APIMajor = 1

type Kind string

const (
	Plugin Kind = "plugin"
	Game   Kind = "game"
)

func (k Kind) valid() bool { return k == Plugin || k == Game }
func (k Kind) directory() string {
	if k == Plugin {
		return "plugins"
	}
	return "games"
}
func (k Kind) manifestFile() string { return "denova." + string(k) + ".json" }

type LocalizedText struct {
	Chinese string `json:"zh-CN"`
	English string `json:"en-US"`
}

type PackageRef struct {
	Kind Kind   `json:"kind"`
	ID   string `json:"id"`
}

type ReleaseRef struct {
	Package   PackageRef `json:"package"`
	ReleaseID string     `json:"releaseId"`
}

type Command struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type Backend struct {
	Protocol string `json:"protocol"`
	Launch   struct {
		Kind    string   `json:"kind"`
		Runtime string   `json:"runtime"`
		Entry   string   `json:"entry"`
		Args    []string `json:"args"`
	} `json:"launch"`
}

type View struct {
	ID     string `json:"id"`
	Source struct {
		Kind string `json:"kind"`
		Path string `json:"path"`
	} `json:"source"`
}

type Tool struct {
	ID         string `json:"id"`
	Definition string `json:"definition"`
	Endpoint   struct {
		Method string `json:"method"`
		Path   string `json:"path"`
	} `json:"endpoint"`
}

type Toolset struct {
	ID    string   `json:"id"`
	Tools []string `json:"tools"`
}

type DefinitionFile struct {
	ID         string `json:"id"`
	Definition string `json:"definition"`
}

// Contributions expose executable extension capabilities. Skills and user Agent
// profiles remain owned by their existing management surfaces.
type Contributions struct {
	Tools    []Tool    `json:"tools,omitempty"`
	Toolsets []Toolset `json:"toolsets,omitempty"`
}

// GameDefinitions are private game content, never installed as user Agent profiles.
type GameDefinitions struct {
	Contributions
	Agents []DefinitionFile `json:"agents,omitempty"`
}

type Dependency struct {
	PluginID      string   `json:"pluginId"`
	VersionRange  string   `json:"versionRange"`
	Contributions []string `json:"contributions"`
}

type DependencyPin struct {
	PluginID  string `json:"pluginId"`
	ReleaseID string `json:"releaseId"`
}

type Uses struct {
	Agents   []string `json:"agents,omitempty"`
	Toolsets []string `json:"toolsets,omitempty"`
}

type GameDeclaration struct {
	// Cover is an optional distributed raster image, relative to the package root.
	Cover   string                    `json:"cover,omitempty"`
	ViewID  string                    `json:"viewId"`
	Setup   *ConfigurationDeclaration `json:"setup,omitempty"`
	Uses    Uses                      `json:"uses,omitempty"`
	Storage struct {
		Kind string `json:"kind"`
		// A stable saveFormat is an author's explicit compatibility declaration.
		// An instance upgrade requires equality and never runs a migration script.
		SaveFormat string `json:"saveFormat,omitempty"`
	} `json:"storage"`
}

type Manifest struct {
	ManifestVersion int               `json:"manifestVersion"`
	ID              string            `json:"id"`
	Version         string            `json:"version"`
	APIMajor        int               `json:"apiMajor"`
	Name            LocalizedText     `json:"name"`
	Description     *LocalizedText    `json:"description,omitempty"`
	Locales         map[string]string `json:"locales,omitempty"`
	Development     *struct {
		Build *Command `json:"build,omitempty"`
		Dev   *Command `json:"dev,omitempty"`
	} `json:"development,omitempty"`
	Distribution *struct {
		Files []string `json:"files"`
	} `json:"distribution,omitempty"`
	Runtime *struct {
		Backend *Backend `json:"backend,omitempty"`
	} `json:"runtime,omitempty"`
	Views       []View                    `json:"views,omitempty"`
	ModelSlots  []ModelSlot               `json:"modelSlots,omitempty"`
	Settings    *ConfigurationDeclaration `json:"settings,omitempty"`
	Permissions struct {
		Required []string `json:"required"`
		Optional []string `json:"optional"`
	} `json:"permissions"`
	Requires    []Dependency     `json:"requires,omitempty"`
	Contributes *Contributions   `json:"contributes,omitempty"`
	Definitions *GameDefinitions `json:"definitions,omitempty"`
	Game        *GameDeclaration `json:"game,omitempty"`
}

// ModelSlot requests a named model profile. Providers never receive credentials.
type ModelSlot struct {
	ID       string `json:"id"`
	TitleKey string `json:"titleKey"`
	Kind     string `json:"kind"`
	Required bool   `json:"required"`
}

func (m Manifest) backend() *Backend {
	if m.Runtime == nil {
		return nil
	}
	return m.Runtime.Backend
}

func (m Manifest) contributions() Contributions {
	if m.Contributes != nil {
		return *m.Contributes
	}
	if m.Definitions != nil {
		return m.Definitions.Contributions
	}
	return Contributions{}
}

func (m Manifest) privateAgents() []DefinitionFile {
	if m.Definitions == nil {
		return nil
	}
	return m.Definitions.Agents
}

type Release struct {
	Ref         ReleaseRef `json:"ref"`
	Manifest    Manifest   `json:"manifest"`
	Digest      string     `json:"digest"`
	InstalledAt time.Time  `json:"installedAt"`
	Grants      []string   `json:"grants"`
}

type Installed struct {
	// Source tracks upstream updates; frozen package identity remains the digest.
	Source         *GitHubSource `json:"source,omitempty"`
	ID             string        `json:"id"`
	Enabled        bool          `json:"enabled"`
	Removed        bool          `json:"removed,omitempty"`
	CurrentRelease string        `json:"currentRelease"`
	Grants         []string      `json:"grants"`
	Releases       []Release     `json:"releases"`
}

// Scope is supplied by trusted management, then bound to runtime credentials.
// Consumer payloads cannot choose or widen it. Preview always has a separate ID.
type Scope struct {
	Kind       string `json:"kind"`
	InstanceID string `json:"instanceId,omitempty"`
	ProjectID  string `json:"projectId,omitempty"`
	SessionID  string `json:"sessionId,omitempty"`
	StoryID    string `json:"storyId,omitempty"`
	BranchID   string `json:"branchId,omitempty"`
}

type Instance struct {
	ID           string            `json:"instanceId"`
	GameID       string            `json:"gameId"`
	ReleaseID    string            `json:"releaseId"`
	Title        string            `json:"title"`
	ProjectID    string            `json:"projectId,omitempty"`
	Dependencies []DependencyPin   `json:"dependencies"`
	Setup        map[string]any    `json:"setup"`
	Models       map[string]string `json:"models"`
	Preview      bool              `json:"preview"`
	CreatedAt    time.Time         `json:"createdAt"`
}

type Error struct {
	Fields     []ConfigurationIssue `json:"fields,omitempty"`
	PackageID  string               `json:"packageId,omitempty"`
	Version    string               `json:"version,omitempty"`
	Code       string               `json:"code"`
	MessageKey string               `json:"messageKey"`
	Diagnostic string               `json:"diagnostic"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Diagnostic }
func failure(code, format string, args ...any) error {
	return &Error{Code: code, MessageKey: "platform.errors." + code, Diagnostic: fmt.Sprintf(format, args...)}
}

type ToolDefinition struct {
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
	Effect       string          `json:"effect"`
	TitleKey     string          `json:"titleKey,omitempty"`
}

type AgentDefinition struct {
	Instructions string   `json:"instructions"`
	ModelSlot    string   `json:"modelSlot"`
	Tools        []string `json:"tools,omitempty"`
	Toolsets     []string `json:"toolsets,omitempty"`
}
