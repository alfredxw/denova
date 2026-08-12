package agentrun

import (
	"fmt"
	"strings"

	"denova/config"

	agent "github.com/alfredxw/denova/agent"
)

const (
	bindingKindWriting    = "writing"
	bindingKindProject    = "project"
	bindingKindGame       = "game"
	bindingKindAutomation = "automation"
	bindingKindUser       = "user"

	bindingProfileWriting          = "writing"
	bindingProfileAgentChat        = "agent_chat"
	bindingProfileGame             = "game"
	bindingProfileAutomation       = "automation"
	bindingProfileConfigManager    = "config_manager"
	bindingProfileHarnessOptimizer = "harness_optimizer"
	bindingProfileImage            = "image"
	bindingProfileDirector         = "director"

	bindingLabelWorkspace = "workspace"
	bindingLabelProject   = "project_id"
	bindingLabelAgentKind = "agent_kind"
	bindingLabelSession   = "session_id"
	bindingLabelStory     = "story_id"
	bindingLabelBranch    = "branch_id"
	bindingLabelTask      = "task_id"
)

// ModeAgentChat identifies user-level project conversations that reuse an IDE
// or General Agent implementation without inheriting foreground workspace
// lifecycle.
const ModeAgentChat = bindingProfileAgentChat

// RuntimeBinding is Denova's product identity adapter for the reusable public
// Agent Session. Product modes and workspace/session semantics stay here; the
// Agent package receives only its provider-neutral SessionKey.
type RuntimeBinding struct {
	AgentKind string
	ProjectID string
	// Mode separates products that reuse the IDE Agent implementation but own
	// different lifecycles. AgentChat conversations are user-level bindings and
	// must survive switches of the foreground Writing workspace.
	Mode      string
	Workspace string
	SessionID string
	StoryID   string
	BranchID  string
	TaskID    string
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

	invalid := func() (bindingIdentity, error) { return bindingIdentity{}, ErrInvalidBinding }
	var identity bindingIdentity
	switch strings.TrimSpace(binding.AgentKind) {
	case AgentKindIDE:
		if attributes[bindingLabelSession] == "" || binding.StoryID != "" || binding.BranchID != "" || binding.TaskID != "" {
			return invalid()
		}
		profile := bindingProfileWriting
		if strings.TrimSpace(binding.Mode) == bindingProfileAgentChat {
			if attributes[bindingLabelProject] != "" {
				// Project is the durable owner. Its content directory is mutable
				// metadata and must not fork the Session when relinked.
				delete(attributes, bindingLabelWorkspace)
				attributes[bindingLabelAgentKind] = AgentKindIDE
				identity = bindingIdentity{
					kind: bindingKindProject, profile: bindingProfileAgentChat,
					id: attributes[bindingLabelProject] + ":" + attributes[bindingLabelSession], attributes: attributes,
				}
				break
			}
			profile = bindingProfileAgentChat
		}
		if attributes[bindingLabelWorkspace] == "" {
			return invalid()
		}
		identity = bindingIdentity{kind: bindingKindWriting, profile: profile, id: attributes[bindingLabelSession], attributes: attributes}
	case AgentKindGeneral:
		if strings.TrimSpace(binding.Mode) != bindingProfileAgentChat || attributes[bindingLabelProject] == "" ||
			attributes[bindingLabelSession] == "" || binding.StoryID != "" || binding.BranchID != "" || binding.TaskID != "" {
			return invalid()
		}
		delete(attributes, bindingLabelWorkspace)
		attributes[bindingLabelAgentKind] = AgentKindGeneral
		identity = bindingIdentity{
			kind: bindingKindProject, profile: bindingProfileAgentChat,
			id: attributes[bindingLabelProject] + ":" + attributes[bindingLabelSession], attributes: attributes,
		}
	case AgentKindInteractiveStory:
		if attributes[bindingLabelWorkspace] == "" || attributes[bindingLabelStory] == "" ||
			attributes[bindingLabelBranch] == "" || binding.TaskID != "" {
			return invalid()
		}
		identity = bindingIdentity{
			kind: bindingKindGame, profile: bindingProfileGame,
			id: attributes[bindingLabelStory] + ":" + attributes[bindingLabelBranch], attributes: attributes,
		}
	case AgentKindConfigManager:
		if attributes[bindingLabelWorkspace] == "" || attributes[bindingLabelSession] == "" ||
			binding.StoryID != "" || binding.BranchID != "" || binding.TaskID != "" {
			return invalid()
		}
		identity = bindingIdentity{kind: bindingKindWriting, profile: bindingProfileConfigManager, id: attributes[bindingLabelSession], attributes: attributes}
	case AgentKindHarnessOptimizer:
		if attributes[bindingLabelSession] == "" || binding.ProjectID != "" || binding.Workspace != "" ||
			binding.StoryID != "" || binding.BranchID != "" || binding.TaskID != "" {
			return invalid()
		}
		identity = bindingIdentity{kind: bindingKindUser, profile: bindingProfileHarnessOptimizer, id: attributes[bindingLabelSession], attributes: attributes}
	case AgentKindImage:
		if attributes[bindingLabelWorkspace] == "" || attributes[bindingLabelSession] == "" ||
			binding.StoryID != "" || binding.BranchID != "" || binding.TaskID != "" {
			return invalid()
		}
		identity = bindingIdentity{kind: bindingKindWriting, profile: bindingProfileImage, id: attributes[bindingLabelSession], attributes: attributes}
	case AgentKindAutomation:
		if attributes[bindingLabelSession] == "" || attributes[bindingLabelTask] == "" || binding.StoryID != "" || binding.BranchID != "" {
			return invalid()
		}
		// ProjectID owns task records outside Agent. Existing automation Session
		// identity remains workspace/session/task scoped.
		delete(attributes, bindingLabelProject)
		identity = bindingIdentity{kind: bindingKindAutomation, profile: bindingProfileAutomation, id: attributes[bindingLabelSession], attributes: attributes}
	case config.AgentKindInteractiveDirector:
		if attributes[bindingLabelWorkspace] == "" || attributes[bindingLabelStory] == "" ||
			attributes[bindingLabelBranch] == "" || binding.TaskID != "" {
			return invalid()
		}
		identity = bindingIdentity{
			kind: bindingKindGame, profile: bindingProfileDirector,
			id: attributes[bindingLabelStory] + ":" + attributes[bindingLabelBranch], attributes: attributes,
		}
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
// agent kind. Empty fields remain unconstrained; at least one constraint is
// required.
func BindingSelector(agentKind, workspace string) (agent.SessionSelector, error) {
	selector := agent.SessionSelector{}
	if workspace = strings.TrimSpace(workspace); workspace != "" {
		selector.Attributes = map[string]string{bindingLabelWorkspace: workspace}
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
	case AgentKindConfigManager:
		kind, profile = bindingKindWriting, bindingProfileConfigManager
	case AgentKindHarnessOptimizer:
		kind, profile = bindingKindUser, bindingProfileHarnessOptimizer
	case AgentKindImage:
		kind, profile = bindingKindWriting, bindingProfileImage
	case AgentKindAutomation:
		kind, profile = bindingKindAutomation, bindingProfileAutomation
	case config.AgentKindInteractiveDirector:
		kind, profile = bindingKindGame, bindingProfileDirector
	default:
		return agent.SessionSelector{}, fmt.Errorf("%w: unsupported agent profile %q", ErrInvalidBinding, agentKind)
	}
	if kind != "" {
		selector.Namespace = agentSessionNamespacePrefix + kind + "." + profile
	}
	return validatedBindingSelector(selector)
}

// WorkspaceBindingSelector selects every Denova Session rooted in one
// workspace, regardless of product mode or profile.
func WorkspaceBindingSelector(workspace string) (agent.SessionSelector, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return agent.SessionSelector{}, ErrInvalidBinding
	}
	return validatedBindingSelector(agent.SessionSelector{Attributes: map[string]string{bindingLabelWorkspace: workspace}})
}

// SessionBindingSelector selects one session-backed Denova Agent Session.
func SessionBindingSelector(agentKind, workspace, sessionID string) (agent.SessionSelector, error) {
	selector, err := BindingSelector(agentKind, workspace)
	if err != nil {
		return agent.SessionSelector{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return agent.SessionSelector{}, ErrInvalidBinding
	}
	selector.ID = sessionID
	if selector.Attributes == nil {
		selector.Attributes = make(map[string]string)
	}
	selector.Attributes[bindingLabelSession] = sessionID
	return validatedBindingSelector(selector)
}

// StoryBindingSelector selects all story Sessions for an exact story or
// branch scope. Callers add the game or Director namespace explicitly.
func StoryBindingSelector(workspace, storyID, branchID string) (agent.SessionSelector, error) {
	workspace, storyID, branchID = strings.TrimSpace(workspace), strings.TrimSpace(storyID), strings.TrimSpace(branchID)
	if workspace == "" || storyID == "" {
		return agent.SessionSelector{}, ErrInvalidBinding
	}
	attributes := map[string]string{bindingLabelWorkspace: workspace, bindingLabelStory: storyID}
	if branchID != "" {
		attributes[bindingLabelBranch] = branchID
	}
	return validatedBindingSelector(agent.SessionSelector{Attributes: attributes})
}

// ForegroundWorkspaceBindingSelectors returns the exact product profiles that
// are owned by a foreground workspace. Project-scoped AgentChat bindings are
// intentionally excluded.
func ForegroundWorkspaceBindingSelectors(workspace string) ([]agent.SessionSelector, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, ErrInvalidBinding
	}
	profiles := []struct{ kind, profile string }{
		{bindingKindWriting, bindingProfileWriting},
		{bindingKindWriting, bindingProfileConfigManager},
		{bindingKindWriting, bindingProfileImage},
		{bindingKindGame, bindingProfileGame},
		{bindingKindGame, bindingProfileDirector},
		{bindingKindAutomation, bindingProfileAutomation},
	}
	selectors := make([]agent.SessionSelector, 0, len(profiles))
	for _, candidate := range profiles {
		selector, err := validatedBindingSelector(agent.SessionSelector{
			Namespace:  agentSessionNamespacePrefix + candidate.kind + "." + candidate.profile,
			Attributes: map[string]string{bindingLabelWorkspace: workspace},
		})
		if err != nil {
			return nil, err
		}
		selectors = append(selectors, selector)
	}
	return selectors, nil
}

func AgentChatSessionBindingSelector(workspace, sessionID string) (agent.SessionSelector, error) {
	workspace, sessionID = strings.TrimSpace(workspace), strings.TrimSpace(sessionID)
	if workspace == "" || sessionID == "" {
		return agent.SessionSelector{}, ErrInvalidBinding
	}
	return validatedBindingSelector(agent.SessionSelector{
		Namespace: agentSessionNamespacePrefix + bindingKindWriting + "." + bindingProfileAgentChat,
		ID:        sessionID, Attributes: map[string]string{bindingLabelWorkspace: workspace, bindingLabelSession: sessionID},
	})
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
		Namespace:  agentSessionNamespacePrefix + bindingKindProject + "." + bindingProfileAgentChat,
		Attributes: map[string]string{bindingLabelProject: projectID},
	})
}

func validatedBindingSelector(selector agent.SessionSelector) (agent.SessionSelector, error) {
	if err := selector.Validate(); err != nil {
		return agent.SessionSelector{}, fmt.Errorf("%w: %v", ErrInvalidBinding, err)
	}
	return selector, nil
}
