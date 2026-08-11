package agentrun

import (
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

const agentSessionNamespacePrefix = "denova."

// AgentSessionKey maps Denova product identity onto the public Agent Session
// boundary. It intentionally preserves the existing durable lane semantics so
// Project content paths remain metadata while writing/game workspace identity
// remains scoped exactly as before.
func (binding RuntimeBinding) AgentSessionKey() (agent.SessionKey, error) {
	ref, err := binding.Ref()
	if err != nil {
		return agent.SessionKey{}, err
	}
	return agent.SessionKey{
		Namespace:  agentSessionNamespacePrefix + ref.Kind + "." + ref.Profile,
		ID:         ref.Key,
		Attributes: cloneBindingLabels(ref.Labels),
	}, nil
}

// RuntimeBindingFromAgentSessionKey decodes only Session keys created by
// AgentSessionKey. Unknown namespaces, labels, or derived IDs fail closed.
func RuntimeBindingFromAgentSessionKey(key agent.SessionKey) (RuntimeBinding, error) {
	namespace := strings.TrimSpace(key.Namespace)
	if !strings.HasPrefix(namespace, agentSessionNamespacePrefix) {
		return RuntimeBinding{}, fmt.Errorf("invalid Denova Agent Session namespace %q", namespace)
	}
	remainder := strings.TrimPrefix(namespace, agentSessionNamespacePrefix)
	separator := strings.LastIndexByte(remainder, '.')
	if separator <= 0 || separator == len(remainder)-1 {
		return RuntimeBinding{}, fmt.Errorf("invalid Denova Agent Session namespace %q", namespace)
	}
	ref := runstate.BindingRef{
		Kind: remainder[:separator], Profile: remainder[separator+1:],
		Key: strings.TrimSpace(key.ID), Labels: cloneBindingLabels(key.Attributes),
	}
	return ParseRuntimeBinding(ref)
}

// AgentSessionKeyForOptions is the single app-facing identity adapter.
func AgentSessionKeyForOptions(options Options) (agent.SessionKey, error) {
	binding, err := BindingForOptions(options)
	if err != nil {
		return agent.SessionKey{}, err
	}
	product, err := ParseRuntimeBinding(binding)
	if err != nil {
		return agent.SessionKey{}, err
	}
	return product.AgentSessionKey()
}

func cloneBindingLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	result := make(map[string]string, len(labels))
	for name, value := range labels {
		result[name] = value
	}
	return result
}
