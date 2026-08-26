package agentrun

import (
	"fmt"
	"strings"

	"denova/config"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

const agentSessionNamespacePrefix = "denova."

// AgentSessionKey maps Denova product identity directly onto the public Agent
// Session boundary. Product content paths remain mutable metadata for
// project-owned conversations; writing and game lanes stay workspace-scoped.
func (binding RuntimeBinding) AgentSessionKey() (agent.SessionKey, error) {
	identity, err := binding.identity()
	if err != nil {
		return agent.SessionKey{}, err
	}
	key, err := agentsession.NormalizeKey(agent.SessionKey{
		Namespace: identity.namespace(), ID: identity.id,
		Attributes: cloneBindingAttributes(identity.attributes),
	})
	if err != nil {
		return agent.SessionKey{}, fmt.Errorf("%w: %v", ErrInvalidBinding, err)
	}
	return key, nil
}

// RuntimeBindingFromAgentSessionKey decodes only Session keys created by
// AgentSessionKey. Unknown namespaces, attributes, or derived IDs fail closed.
func RuntimeBindingFromAgentSessionKey(key agent.SessionKey) (RuntimeBinding, error) {
	normalized, err := agentsession.NormalizeKey(key)
	if err != nil {
		return RuntimeBinding{}, fmt.Errorf("%w: %v", ErrInvalidBinding, err)
	}
	namespace := normalized.Namespace
	if !strings.HasPrefix(namespace, agentSessionNamespacePrefix) {
		return RuntimeBinding{}, fmt.Errorf("%w: invalid Denova Agent Session namespace %q", ErrInvalidBinding, namespace)
	}
	remainder := strings.TrimPrefix(namespace, agentSessionNamespacePrefix)
	separator := strings.LastIndexByte(remainder, '.')
	if separator <= 0 || separator == len(remainder)-1 {
		return RuntimeBinding{}, fmt.Errorf("%w: invalid Denova Agent Session namespace %q", ErrInvalidBinding, namespace)
	}
	kind, profile := remainder[:separator], remainder[separator+1:]
	attribute := func(name string) string { return normalized.Attributes[name] }
	binding := RuntimeBinding{
		ProjectID: attribute(bindingLabelProject), Workspace: attribute(bindingLabelWorkspace),
		SessionID: attribute(bindingLabelSession), StoryID: attribute(bindingLabelStory),
		BranchID: attribute(bindingLabelBranch), TaskID: attribute(bindingLabelTask),
	}
	switch {
	case kind == bindingKindWriting && profile == bindingProfileWriting:
		binding.AgentKind = AgentKindIDE
	case kind == bindingKindWriting && profile == bindingProfileAgentChat:
		binding.AgentKind, binding.Mode = AgentKindIDE, bindingProfileAgentChat
	case kind == bindingKindProject && profile == bindingProfileAgentChat:
		binding.AgentKind = attribute(bindingLabelAgentKind)
		if binding.AgentKind != AgentKindIDE && binding.AgentKind != AgentKindGeneral && binding.AgentKind != AgentKindHarness {
			return RuntimeBinding{}, fmt.Errorf("%w: unsupported project Agent kind %q", ErrInvalidBinding, binding.AgentKind)
		}
		binding.Mode = bindingProfileAgentChat
	case kind == bindingKindGame && profile == bindingProfileGame:
		binding.AgentKind = AgentKindInteractiveStory
	case kind == bindingKindWriting && profile == bindingProfileConfigManager:
		binding.AgentKind = AgentKindConfigManager
	case kind == bindingKindWriting && profile == bindingProfileImage:
		binding.AgentKind = AgentKindImage
	case kind == bindingKindAutomation && profile == bindingProfileAutomation:
		binding.AgentKind = AgentKindAutomation
	case kind == bindingKindGame && profile == bindingProfileDirector:
		binding.AgentKind = config.AgentKindInteractiveDirector
	default:
		return RuntimeBinding{}, fmt.Errorf("%w: unsupported Denova Session namespace %q", ErrInvalidBinding, namespace)
	}
	encoded, err := binding.AgentSessionKey()
	if err != nil {
		return RuntimeBinding{}, err
	}
	want, wantErr := agentsession.CanonicalKey(normalized)
	got, gotErr := agentsession.CanonicalKey(encoded)
	if wantErr != nil || gotErr != nil || want != got {
		return RuntimeBinding{}, fmt.Errorf("%w: Agent Session ID or attributes do not match Denova identity", ErrInvalidBinding)
	}
	return binding, nil
}

// AgentSessionKeyForOptions is the single app-facing identity adapter.
func AgentSessionKeyForOptions(options Options) (agent.SessionKey, error) {
	binding, err := RuntimeBindingForOptions(options)
	if err != nil {
		return agent.SessionKey{}, err
	}
	return binding.AgentSessionKey()
}

func cloneBindingAttributes(attributes map[string]string) map[string]string {
	if len(attributes) == 0 {
		return nil
	}
	result := make(map[string]string, len(attributes))
	for name, value := range attributes {
		result[name] = value
	}
	return result
}
