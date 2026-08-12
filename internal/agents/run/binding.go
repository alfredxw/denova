package agentrun

import (
	"fmt"
	"strings"

	"denova/config"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

const (
	runtimeBindingKindWriting    = "writing"
	runtimeBindingKindProject    = "project"
	runtimeBindingKindGame       = "game"
	runtimeBindingKindAutomation = "automation"
	runtimeBindingKindUser       = "user"

	runtimeBindingProfileWriting          = "writing"
	runtimeBindingProfileAgentChat        = "agent_chat"
	runtimeBindingProfileGame             = "game"
	runtimeBindingProfileAutomation       = "automation"
	runtimeBindingProfileConfigManager    = "config_manager"
	runtimeBindingProfileHarnessOptimizer = "harness_optimizer"
	runtimeBindingProfileImage            = "image"
	runtimeBindingProfileDirector         = "director"

	runtimeBindingLabelWorkspace = "workspace"
	runtimeBindingLabelProject   = "project_id"
	runtimeBindingLabelAgentKind = "agent_kind"
	runtimeBindingLabelSession   = "session_id"
	runtimeBindingLabelStory     = "story_id"
	runtimeBindingLabelBranch    = "branch_id"
	runtimeBindingLabelTask      = "task_id"
)

// ModeAgentChat identifies user-level project conversations that reuse an IDE
// or General Agent implementation without inheriting foreground workspace
// lifecycle.
const ModeAgentChat = runtimeBindingProfileAgentChat

// RuntimeBinding is Denova's product identity adapter for the reusable
// durable runtime. Product modes and workspace/session semantics stay here;
// agent/runtime only sees an open, bounded BindingRef.
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

// Ref validates and encodes a Denova binding as a provider-neutral runtime
// identity. Its wire kind/profile values intentionally remain stable.
func (binding RuntimeBinding) Ref() (runstate.BindingRef, error) {
	labels := make(map[string]string, 7)
	appendLabel := func(name, value string) {
		if value = strings.TrimSpace(value); value != "" {
			labels[name] = value
		}
	}
	appendLabel(runtimeBindingLabelWorkspace, binding.Workspace)
	appendLabel(runtimeBindingLabelProject, binding.ProjectID)
	appendLabel(runtimeBindingLabelSession, binding.SessionID)
	appendLabel(runtimeBindingLabelStory, binding.StoryID)
	appendLabel(runtimeBindingLabelBranch, binding.BranchID)
	appendLabel(runtimeBindingLabelTask, binding.TaskID)

	var ref runstate.BindingRef
	switch strings.TrimSpace(binding.AgentKind) {
	case AgentKindIDE:
		if labels[runtimeBindingLabelSession] == "" || binding.StoryID != "" || binding.BranchID != "" || binding.TaskID != "" {
			return runstate.BindingRef{}, runstate.ErrInvalidBinding
		}
		profile := runtimeBindingProfileWriting
		if strings.TrimSpace(binding.Mode) == runtimeBindingProfileAgentChat {
			if labels[runtimeBindingLabelProject] != "" {
				// A Project is the durable owner. Its content directory is mutable
				// metadata resolved by the app at execution time, so it must not
				// fork the journal when the user relinks the Project.
				delete(labels, runtimeBindingLabelWorkspace)
				labels[runtimeBindingLabelAgentKind] = AgentKindIDE
				ref = runstate.BindingRef{Kind: runtimeBindingKindProject, Profile: runtimeBindingProfileAgentChat, Key: labels[runtimeBindingLabelProject] + ":" + labels[runtimeBindingLabelSession], Labels: labels}
				break
			}
			profile = runtimeBindingProfileAgentChat
		}
		if labels[runtimeBindingLabelWorkspace] == "" {
			return runstate.BindingRef{}, runstate.ErrInvalidBinding
		}
		ref = runstate.BindingRef{Kind: runtimeBindingKindWriting, Profile: profile, Key: labels[runtimeBindingLabelSession], Labels: labels}
	case AgentKindGeneral:
		if strings.TrimSpace(binding.Mode) != runtimeBindingProfileAgentChat || labels[runtimeBindingLabelProject] == "" || labels[runtimeBindingLabelSession] == "" || binding.StoryID != "" || binding.BranchID != "" || binding.TaskID != "" {
			return runstate.BindingRef{}, runstate.ErrInvalidBinding
		}
		delete(labels, runtimeBindingLabelWorkspace)
		labels[runtimeBindingLabelAgentKind] = AgentKindGeneral
		ref = runstate.BindingRef{Kind: runtimeBindingKindProject, Profile: runtimeBindingProfileAgentChat, Key: labels[runtimeBindingLabelProject] + ":" + labels[runtimeBindingLabelSession], Labels: labels}
	case AgentKindInteractiveStory:
		if labels[runtimeBindingLabelWorkspace] == "" || labels[runtimeBindingLabelStory] == "" || labels[runtimeBindingLabelBranch] == "" || binding.TaskID != "" {
			return runstate.BindingRef{}, runstate.ErrInvalidBinding
		}
		ref = runstate.BindingRef{Kind: runtimeBindingKindGame, Profile: runtimeBindingProfileGame, Key: labels[runtimeBindingLabelStory] + ":" + labels[runtimeBindingLabelBranch], Labels: labels}
	case AgentKindConfigManager:
		if labels[runtimeBindingLabelWorkspace] == "" || labels[runtimeBindingLabelSession] == "" || binding.StoryID != "" || binding.BranchID != "" || binding.TaskID != "" {
			return runstate.BindingRef{}, runstate.ErrInvalidBinding
		}
		ref = runstate.BindingRef{Kind: runtimeBindingKindWriting, Profile: runtimeBindingProfileConfigManager, Key: labels[runtimeBindingLabelSession], Labels: labels}
	case AgentKindHarnessOptimizer:
		if labels[runtimeBindingLabelSession] == "" || binding.ProjectID != "" || binding.Workspace != "" || binding.StoryID != "" || binding.BranchID != "" || binding.TaskID != "" {
			return runstate.BindingRef{}, runstate.ErrInvalidBinding
		}
		ref = runstate.BindingRef{Kind: runtimeBindingKindUser, Profile: runtimeBindingProfileHarnessOptimizer, Key: labels[runtimeBindingLabelSession], Labels: labels}
	case AgentKindImage:
		if labels[runtimeBindingLabelWorkspace] == "" || labels[runtimeBindingLabelSession] == "" || binding.StoryID != "" || binding.BranchID != "" || binding.TaskID != "" {
			return runstate.BindingRef{}, runstate.ErrInvalidBinding
		}
		ref = runstate.BindingRef{Kind: runtimeBindingKindWriting, Profile: runtimeBindingProfileImage, Key: labels[runtimeBindingLabelSession], Labels: labels}
	case AgentKindAutomation:
		if labels[runtimeBindingLabelSession] == "" || labels[runtimeBindingLabelTask] == "" || binding.StoryID != "" || binding.BranchID != "" {
			return runstate.BindingRef{}, runstate.ErrInvalidBinding
		}
		// Automation journals predate Project IDs and already have durable
		// workspace/session/task identity. Keep that wire identity readable while
		// ProjectID owns task/session persistence outside the runtime journal.
		delete(labels, runtimeBindingLabelProject)
		ref = runstate.BindingRef{Kind: runtimeBindingKindAutomation, Profile: runtimeBindingProfileAutomation, Key: labels[runtimeBindingLabelSession], Labels: labels}
	case config.AgentKindInteractiveDirector:
		if labels[runtimeBindingLabelWorkspace] == "" || labels[runtimeBindingLabelStory] == "" || labels[runtimeBindingLabelBranch] == "" || binding.TaskID != "" {
			return runstate.BindingRef{}, runstate.ErrInvalidBinding
		}
		ref = runstate.BindingRef{Kind: runtimeBindingKindGame, Profile: runtimeBindingProfileDirector, Key: labels[runtimeBindingLabelStory] + ":" + labels[runtimeBindingLabelBranch], Labels: labels}
	default:
		return runstate.BindingRef{}, fmt.Errorf("%w: unsupported agent profile %q", runstate.ErrInvalidBinding, binding.AgentKind)
	}
	if err := runstate.ValidateBindingRef(ref); err != nil {
		return runstate.BindingRef{}, err
	}
	return ref, nil
}

// ParseRuntimeBinding decodes and validates a generic runtime identity at the
// product boundary. It rejects labels or keys that Denova did not create.
func ParseRuntimeBinding(ref runstate.BindingRef) (RuntimeBinding, error) {
	if err := runstate.ValidateBindingRef(ref); err != nil {
		return RuntimeBinding{}, err
	}
	binding := RuntimeBinding{
		ProjectID: ref.Label(runtimeBindingLabelProject),
		Workspace: ref.Label(runtimeBindingLabelWorkspace),
		SessionID: ref.Label(runtimeBindingLabelSession),
		StoryID:   ref.Label(runtimeBindingLabelStory),
		BranchID:  ref.Label(runtimeBindingLabelBranch),
		TaskID:    ref.Label(runtimeBindingLabelTask),
	}
	switch {
	case ref.Kind == runtimeBindingKindWriting && ref.Profile == runtimeBindingProfileWriting:
		binding.AgentKind = AgentKindIDE
	case ref.Kind == runtimeBindingKindWriting && ref.Profile == runtimeBindingProfileAgentChat:
		binding.AgentKind = AgentKindIDE
		binding.Mode = runtimeBindingProfileAgentChat
	case ref.Kind == runtimeBindingKindProject && ref.Profile == runtimeBindingProfileAgentChat:
		binding.AgentKind = ref.Label(runtimeBindingLabelAgentKind)
		if binding.AgentKind != AgentKindIDE && binding.AgentKind != AgentKindGeneral {
			return RuntimeBinding{}, fmt.Errorf("%w: unsupported project Agent kind %q", runstate.ErrInvalidBinding, binding.AgentKind)
		}
		binding.Mode = runtimeBindingProfileAgentChat
	case ref.Kind == runtimeBindingKindGame && ref.Profile == runtimeBindingProfileGame:
		binding.AgentKind = AgentKindInteractiveStory
	case ref.Kind == runtimeBindingKindWriting && ref.Profile == runtimeBindingProfileConfigManager:
		binding.AgentKind = AgentKindConfigManager
	case ref.Kind == runtimeBindingKindUser && ref.Profile == runtimeBindingProfileHarnessOptimizer:
		binding.AgentKind = AgentKindHarnessOptimizer
	case ref.Kind == runtimeBindingKindWriting && ref.Profile == runtimeBindingProfileImage:
		binding.AgentKind = AgentKindImage
	case ref.Kind == runtimeBindingKindAutomation && ref.Profile == runtimeBindingProfileAutomation:
		binding.AgentKind = AgentKindAutomation
	case ref.Kind == runtimeBindingKindGame && ref.Profile == runtimeBindingProfileDirector:
		binding.AgentKind = config.AgentKindInteractiveDirector
	default:
		return RuntimeBinding{}, fmt.Errorf("%w: unsupported Denova runtime kind=%q profile=%q", runstate.ErrInvalidBinding, ref.Kind, ref.Profile)
	}
	encoded, err := binding.Ref()
	if err != nil || !encoded.Equal(ref) {
		return RuntimeBinding{}, fmt.Errorf("%w: runtime binding key or labels do not match Denova identity", runstate.ErrInvalidBinding)
	}
	return binding, nil
}

// RuntimeBindingSelector returns a bounded selector for one Denova agent kind.
// Empty fields remain unconstrained; at least one constraint is required.
func BindingSelector(agentKind, workspace string) (runstate.BindingSelector, error) {
	selector := runstate.BindingSelector{}
	if workspace = strings.TrimSpace(workspace); workspace != "" {
		selector.Labels = map[string]string{runtimeBindingLabelWorkspace: workspace}
	}
	switch strings.TrimSpace(agentKind) {
	case "":
	case AgentKindGeneral:
		selector.Kind, selector.Profile = runtimeBindingKindProject, runtimeBindingProfileAgentChat
	case AgentKindIDE:
		selector.Kind, selector.Profile = runtimeBindingKindWriting, runtimeBindingProfileWriting
	case AgentKindInteractiveStory:
		selector.Kind, selector.Profile = runtimeBindingKindGame, runtimeBindingProfileGame
	case AgentKindConfigManager:
		selector.Kind, selector.Profile = runtimeBindingKindWriting, runtimeBindingProfileConfigManager
	case AgentKindHarnessOptimizer:
		selector.Kind, selector.Profile = runtimeBindingKindUser, runtimeBindingProfileHarnessOptimizer
	case AgentKindImage:
		selector.Kind, selector.Profile = runtimeBindingKindWriting, runtimeBindingProfileImage
	case AgentKindAutomation:
		selector.Kind, selector.Profile = runtimeBindingKindAutomation, runtimeBindingProfileAutomation
	case config.AgentKindInteractiveDirector:
		selector.Kind, selector.Profile = runtimeBindingKindGame, runtimeBindingProfileDirector
	default:
		return runstate.BindingSelector{}, fmt.Errorf("%w: unsupported agent profile %q", runstate.ErrInvalidBinding, agentKind)
	}
	if selector.Kind == "" && selector.Profile == "" && selector.Key == "" && len(selector.Labels) == 0 {
		return runstate.BindingSelector{}, runstate.ErrInvalidBinding
	}
	return selector, nil
}

// RuntimeWorkspaceBindingSelector selects every Denova binding rooted in one
// workspace, regardless of product mode or profile.
func WorkspaceBindingSelector(workspace string) (runstate.BindingSelector, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return runstate.BindingSelector{}, runstate.ErrInvalidBinding
	}
	return runstate.BindingSelector{Labels: map[string]string{runtimeBindingLabelWorkspace: workspace}}, nil
}

// RuntimeSessionBindingSelector selects one session-backed Denova actor.
func SessionBindingSelector(agentKind, workspace, sessionID string) (runstate.BindingSelector, error) {
	selector, err := BindingSelector(agentKind, workspace)
	if err != nil {
		return runstate.BindingSelector{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return runstate.BindingSelector{}, runstate.ErrInvalidBinding
	}
	selector.Key = sessionID
	if selector.Labels == nil {
		selector.Labels = make(map[string]string)
	}
	selector.Labels[runtimeBindingLabelSession] = sessionID
	return selector, nil
}

// RuntimeStoryBindingSelector selects all story actors (including the
// interactive Director profile) for an exact story or branch scope.
func StoryBindingSelector(workspace, storyID, branchID string) (runstate.BindingSelector, error) {
	workspace, storyID, branchID = strings.TrimSpace(workspace), strings.TrimSpace(storyID), strings.TrimSpace(branchID)
	if workspace == "" || storyID == "" {
		return runstate.BindingSelector{}, runstate.ErrInvalidBinding
	}
	labels := map[string]string{
		runtimeBindingLabelWorkspace: workspace,
		runtimeBindingLabelStory:     storyID,
	}
	if branchID != "" {
		labels[runtimeBindingLabelBranch] = branchID
	}
	return runstate.BindingSelector{Kind: runtimeBindingKindGame, Labels: labels}, nil
}

// ForegroundWorkspaceBindingSelectors returns the exact product profiles that
// are owned by a foreground workspace. Project-scoped AgentChat bindings are
// intentionally excluded.
func ForegroundWorkspaceBindingSelectors(workspace string) ([]runstate.BindingSelector, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, runstate.ErrInvalidBinding
	}
	profiles := []struct{ kind, profile string }{
		{runtimeBindingKindWriting, runtimeBindingProfileWriting},
		{runtimeBindingKindWriting, runtimeBindingProfileConfigManager},
		{runtimeBindingKindWriting, runtimeBindingProfileImage},
		{runtimeBindingKindGame, runtimeBindingProfileGame},
		{runtimeBindingKindGame, runtimeBindingProfileDirector},
		{runtimeBindingKindAutomation, runtimeBindingProfileAutomation},
	}
	selectors := make([]runstate.BindingSelector, 0, len(profiles))
	for _, candidate := range profiles {
		selectors = append(selectors, runstate.BindingSelector{
			Kind: candidate.kind, Profile: candidate.profile,
			Labels: map[string]string{runtimeBindingLabelWorkspace: workspace},
		})
	}
	return selectors, nil
}

func AgentChatSessionBindingSelector(workspace, sessionID string) (runstate.BindingSelector, error) {
	workspace, sessionID = strings.TrimSpace(workspace), strings.TrimSpace(sessionID)
	if workspace == "" || sessionID == "" {
		return runstate.BindingSelector{}, runstate.ErrInvalidBinding
	}
	return runstate.BindingSelector{
		Kind: runtimeBindingKindWriting, Profile: runtimeBindingProfileAgentChat, Key: sessionID,
		Labels: map[string]string{runtimeBindingLabelWorkspace: workspace, runtimeBindingLabelSession: sessionID},
	}, nil
}

func ProjectSessionBindingSelector(projectID, sessionID string) (runstate.BindingSelector, error) {
	projectID, sessionID = strings.TrimSpace(projectID), strings.TrimSpace(sessionID)
	if projectID == "" || sessionID == "" {
		return runstate.BindingSelector{}, runstate.ErrInvalidBinding
	}
	return runstate.BindingSelector{
		Kind: runtimeBindingKindProject, Profile: runtimeBindingProfileAgentChat,
		Key:    projectID + ":" + sessionID,
		Labels: map[string]string{runtimeBindingLabelProject: projectID, runtimeBindingLabelSession: sessionID},
	}, nil
}

func ProjectBindingSelector(projectID string) (runstate.BindingSelector, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return runstate.BindingSelector{}, runstate.ErrInvalidBinding
	}
	return runstate.BindingSelector{
		Kind: runtimeBindingKindProject, Profile: runtimeBindingProfileAgentChat,
		Labels: map[string]string{runtimeBindingLabelProject: projectID},
	}, nil
}
