package agentrun

import (
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

const (
	bindingKindWriting    = "writing"
	bindingKindProject    = "project"
	bindingKindGame       = "game"
	bindingKindAutomation = "automation"
	bindingKindUser       = "user"

	bindingProfileWriting    = "writing"
	bindingProfileAgentChat  = "agent_chat"
	bindingProfileGame       = "game"
	bindingProfileAutomation = "automation"
	bindingProfileImage      = "image"

	bindingLabelWorkspace = "workspace"
	bindingLabelProject   = "project_id"
	bindingLabelAgentKind = "agent_kind"
	bindingLabelSession   = "session_id"
	bindingLabelStory     = "story_id"
	bindingLabelBranch    = "branch_id"
	bindingLabelTask      = "task_id"
)

// ModeAgentChat identifies user-level project conversations that reuse a
// Writing or General Agent implementation without inheriting foreground
// workspace lifecycle.
const ModeAgentChat = bindingProfileAgentChat

// RuntimeBinding is Denova's product identity adapter for the reusable public
// Agent Session. ProjectID is the durable owner; Workspace is runtime-only
// environment metadata and never enters the public SessionKey.
type RuntimeBinding struct {
	AgentKind string
	ProjectID string
	// Mode separates products that reuse the Writing Agent implementation but
	// own different lifecycles. AgentChat conversations are user-level bindings
	// and must survive switches of the foreground Writing workspace.
	Mode      string
	Workspace string
	SessionID string
	StoryID   string
	BranchID  string
	TaskID    string
}

// SessionStorageScope is the stable Project and product-journal scope carried
// by a Denova Agent Session selector. It lets storage route constrained catalog
// reads without exposing Denova's private identity labels to other packages.
type SessionStorageScope struct {
	ProjectID string
	SessionID string
	StoryID   string
	Journal   SessionJournalKind
}

// SessionJournalKind identifies the canonical product journal family that can
// contain the Sessions selected by one storage scope.
type SessionJournalKind uint8

const (
	SessionJournalAny SessionJournalKind = iota
	SessionJournalProduct
	SessionJournalStory
)

// StorageScopeFromSessionSelector returns the exact Project owner encoded in a
// selector. SessionID and StoryID narrow the journal when the selector carries
// one of those immutable product identities.
func StorageScopeFromSessionSelector(selector agent.SessionSelector) (SessionStorageScope, bool) {
	projectID := strings.TrimSpace(selector.Attributes[bindingLabelProject])
	if projectID == "" {
		return SessionStorageScope{}, false
	}
	scope := SessionStorageScope{
		ProjectID: projectID,
		SessionID: strings.TrimSpace(selector.Attributes[bindingLabelSession]),
		StoryID:   strings.TrimSpace(selector.Attributes[bindingLabelStory]),
	}
	switch {
	case scope.SessionID != "" && scope.StoryID == "":
		scope.Journal = SessionJournalProduct
	case scope.StoryID != "" && scope.SessionID == "":
		scope.Journal = SessionJournalStory
	default:
		switch selector.Namespace {
		case agentSessionNamespacePrefix + bindingKindWriting + "." + bindingProfileWriting,
			agentSessionNamespacePrefix + bindingKindWriting + "." + bindingProfileImage,
			agentSessionNamespacePrefix + bindingKindProject + "." + bindingProfileAgentChat,
			agentSessionNamespacePrefix + bindingKindAutomation + "." + bindingProfileAutomation:
			scope.Journal = SessionJournalProduct
		case agentSessionNamespacePrefix + bindingKindGame + "." + bindingProfileGame:
			scope.Journal = SessionJournalStory
		}
	}
	return scope, true
}

// bindingIdentity is the private Denova mapping from product identity to one
// public Agent Session namespace. It is not a second runtime identity or a
// persistence contract.
type bindingIdentity struct {
	kind       string
	profile    string
	id         string
	attributes map[string]string
}

func (identity bindingIdentity) namespace() string {
	return agentSessionNamespacePrefix + identity.kind + "." + identity.profile
}

func (binding RuntimeBinding) identity() (bindingIdentity, error) {
	attributes := make(map[string]string, 7)
	appendAttribute := func(name, value string) {
		if value = strings.TrimSpace(value); value != "" {
			attributes[name] = value
		}
	}
	appendAttribute(bindingLabelWorkspace, binding.Workspace)
	appendAttribute(bindingLabelProject, binding.ProjectID)
	appendAttribute(bindingLabelSession, binding.SessionID)
	appendAttribute(bindingLabelStory, binding.StoryID)
	appendAttribute(bindingLabelBranch, binding.BranchID)
	appendAttribute(bindingLabelTask, binding.TaskID)
	projectID := attributes[bindingLabelProject]
	dropRuntimeWorkspace := func() { delete(attributes, bindingLabelWorkspace) }

	invalid := func() (bindingIdentity, error) { return bindingIdentity{}, ErrInvalidBinding }
	var identity bindingIdentity
	switch strings.TrimSpace(binding.AgentKind) {
	case AgentKindIDE:
		if projectID == "" || attributes[bindingLabelSession] == "" || binding.StoryID != "" || binding.BranchID != "" || binding.TaskID != "" {
			return invalid()
		}
		dropRuntimeWorkspace()
		profile := bindingProfileWriting
		if strings.TrimSpace(binding.Mode) == bindingProfileAgentChat {
			attributes[bindingLabelAgentKind] = AgentKindIDE
			identity = bindingIdentity{
				kind: bindingKindProject, profile: bindingProfileAgentChat,
				id: projectID + ":" + attributes[bindingLabelSession], attributes: attributes,
			}
			break
		}
		identity = bindingIdentity{kind: bindingKindWriting, profile: profile, id: projectID + ":" + attributes[bindingLabelSession], attributes: attributes}
	case AgentKindGeneral:
		if strings.TrimSpace(binding.Mode) != bindingProfileAgentChat || projectID == "" ||
			attributes[bindingLabelSession] == "" || binding.StoryID != "" || binding.BranchID != "" || binding.TaskID != "" {
			return invalid()
		}
		dropRuntimeWorkspace()
		attributes[bindingLabelAgentKind] = strings.TrimSpace(binding.AgentKind)
		identity = bindingIdentity{
			kind: bindingKindProject, profile: bindingProfileAgentChat,
			id: projectID + ":" + attributes[bindingLabelSession], attributes: attributes,
		}
	case AgentKindInteractiveStory:
		if projectID == "" || attributes[bindingLabelStory] == "" ||
			attributes[bindingLabelBranch] == "" || binding.TaskID != "" {
			return invalid()
		}
		dropRuntimeWorkspace()
		identity = bindingIdentity{
			kind: bindingKindGame, profile: bindingProfileGame,
			id: projectID + ":" + attributes[bindingLabelStory] + ":" + attributes[bindingLabelBranch], attributes: attributes,
		}
	case AgentKindImage:
		if projectID == "" || attributes[bindingLabelSession] == "" ||
			binding.StoryID != "" || binding.BranchID != "" || binding.TaskID != "" {
			return invalid()
		}
		dropRuntimeWorkspace()
		identity = bindingIdentity{kind: bindingKindWriting, profile: bindingProfileImage, id: projectID + ":" + attributes[bindingLabelSession], attributes: attributes}
	case AgentKindAutomation:
		if projectID == "" || attributes[bindingLabelSession] == "" || attributes[bindingLabelTask] == "" || binding.StoryID != "" || binding.BranchID != "" {
			return invalid()
		}
		dropRuntimeWorkspace()
		identity = bindingIdentity{kind: bindingKindAutomation, profile: bindingProfileAutomation, id: projectID + ":" + attributes[bindingLabelSession], attributes: attributes}
	default:
		return bindingIdentity{}, fmt.Errorf("%w: unsupported agent profile %q", ErrInvalidBinding, binding.AgentKind)
	}
	return identity, nil
}

// ProfileID returns the Denova execution profile selected by this binding.
// The value chooses a product Definition builder; it is not an Agent runtime
// type and never enters the public Session Store.
func (binding RuntimeBinding) ProfileID() (string, error) {
	identity, err := binding.identity()
	if err != nil {
		return "", err
	}
	return identity.profile, nil
}

// BindingSelector returns a bounded public Session selector for one Denova
// agent kind and optional stable Project owner.
func BindingSelector(agentKind, projectID string) (agent.SessionSelector, error) {
	selector := agent.SessionSelector{}
	if projectID = strings.TrimSpace(projectID); projectID != "" {
		selector.Attributes = map[string]string{bindingLabelProject: projectID}
	}
	var kind, profile string
	switch strings.TrimSpace(agentKind) {
	case "":
	case AgentKindGeneral:
		kind, profile = bindingKindProject, bindingProfileAgentChat
	case AgentKindIDE:
		kind, profile = bindingKindWriting, bindingProfileWriting
	case AgentKindInteractiveStory:
		kind, profile = bindingKindGame, bindingProfileGame
	case AgentKindImage:
		kind, profile = bindingKindWriting, bindingProfileImage
	case AgentKindAutomation:
		kind, profile = bindingKindAutomation, bindingProfileAutomation
	default:
		return agent.SessionSelector{}, fmt.Errorf("%w: unsupported agent profile %q", ErrInvalidBinding, agentKind)
	}
	if kind != "" {
		selector.Namespace = agentSessionNamespacePrefix + kind + "." + profile
	}
	return validatedBindingSelector(selector)
}

// SessionBindingSelector selects one session-backed Denova Agent Session.
func SessionBindingSelector(agentKind, projectID, sessionID string) (agent.SessionSelector, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return agent.SessionSelector{}, ErrInvalidBinding
	}
	selector, err := BindingSelector(agentKind, projectID)
	if err != nil {
		return agent.SessionSelector{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return agent.SessionSelector{}, ErrInvalidBinding
	}
	selector.ID = projectID + ":" + sessionID
	if selector.Attributes == nil {
		selector.Attributes = make(map[string]string)
	}
	selector.Attributes[bindingLabelSession] = sessionID
	return validatedBindingSelector(selector)
}

// StoryBindingSelector selects all story Sessions for an exact story or
// branch scope. Callers add the game namespace explicitly.
func StoryBindingSelector(projectID, storyID, branchID string) (agent.SessionSelector, error) {
	projectID, storyID, branchID = strings.TrimSpace(projectID), strings.TrimSpace(storyID), strings.TrimSpace(branchID)
	if projectID == "" || storyID == "" {
		return agent.SessionSelector{}, ErrInvalidBinding
	}
	attributes := map[string]string{bindingLabelProject: projectID, bindingLabelStory: storyID}
	if branchID != "" {
		attributes[bindingLabelBranch] = branchID
	}
	return validatedBindingSelector(agent.SessionSelector{Attributes: attributes})
}

// ForegroundProjectBindingSelectors returns the exact product profiles that
// are owned by a foreground Project. Project-scoped AgentChat bindings are
// intentionally excluded.
func ForegroundProjectBindingSelectors(projectID string) ([]agent.SessionSelector, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, ErrInvalidBinding
	}
	profiles := []struct{ kind, profile string }{
		{bindingKindWriting, bindingProfileWriting},
		{bindingKindWriting, bindingProfileImage},
		{bindingKindGame, bindingProfileGame},
		{bindingKindAutomation, bindingProfileAutomation},
	}
	selectors := make([]agent.SessionSelector, 0, len(profiles))
	for _, candidate := range profiles {
		selector, err := validatedBindingSelector(agent.SessionSelector{
			Namespace:  agentSessionNamespacePrefix + candidate.kind + "." + candidate.profile,
			Attributes: map[string]string{bindingLabelProject: projectID},
		})
		if err != nil {
			return nil, err
		}
		selectors = append(selectors, selector)
	}
	return selectors, nil
}

func ProjectSessionBindingSelector(projectID, sessionID string) (agent.SessionSelector, error) {
	projectID, sessionID = strings.TrimSpace(projectID), strings.TrimSpace(sessionID)
	if projectID == "" || sessionID == "" {
		return agent.SessionSelector{}, ErrInvalidBinding
	}
	return validatedBindingSelector(agent.SessionSelector{
		Namespace:  agentSessionNamespacePrefix + bindingKindProject + "." + bindingProfileAgentChat,
		ID:         projectID + ":" + sessionID,
		Attributes: map[string]string{bindingLabelProject: projectID, bindingLabelSession: sessionID},
	})
}

func ProjectBindingSelector(projectID string) (agent.SessionSelector, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return agent.SessionSelector{}, ErrInvalidBinding
	}
	return validatedBindingSelector(agent.SessionSelector{
		Attributes: map[string]string{bindingLabelProject: projectID},
	})
}

func validatedBindingSelector(selector agent.SessionSelector) (agent.SessionSelector, error) {
	if err := selector.Validate(); err != nil {
		return agent.SessionSelector{}, fmt.Errorf("%w: %v", ErrInvalidBinding, err)
	}
	return selector, nil
}
