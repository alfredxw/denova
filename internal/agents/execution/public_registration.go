package execution

import (
	"encoding/json"
	"strings"

	agentrun "denova/internal/agents/run"
	agent "github.com/alfredxw/denova/agent"
)

// registerInput publishes routing before Agent admission can start preparation.
// Retries keep the original command's route; rejected attempts may only release
// their own registration, never a previously accepted command's callbacks.
func (backend *publicBackend) registerInput(key agent.SessionKey, commandID string, registration *publicCycleRegistration) *publicCycleRegistration {
	identity := publicRegistrationKey(key, commandID)
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if existing := backend.registrations[identity]; existing != nil {
		return existing
	}
	backend.registrations[identity] = registration
	return registration
}

func (backend *publicBackend) forgetRegistration(key agent.SessionKey, commandID string, registration *publicCycleRegistration) {
	identity := publicRegistrationKey(key, commandID)
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.registrations[identity] == registration {
		delete(backend.registrations, identity)
	}
}

func (backend *publicBackend) registration(key agent.SessionKey, commandID string) *publicCycleRegistration {
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	return backend.registrations[publicRegistrationKey(key, commandID)]
}

func (backend *publicBackend) rememberRegistration(key agent.SessionKey, commandID string, registration *publicCycleRegistration) {
	backend.mu.Lock()
	backend.registrations[publicRegistrationKey(key, commandID)] = registration
	backend.mu.Unlock()
}

func (backend *publicBackend) bindRecoveryRoute(
	key agent.SessionKey,
	commandID string,
	options agentrun.Options,
	emit func(agentrun.Event),
) *publicCycleRegistration {
	registration := backend.registration(key, commandID)
	if registration == nil {
		registration = &publicCycleRegistration{options: options, emit: emit}
		backend.rememberRegistration(key, commandID, registration)
		return registration
	}
	registration.mu.Lock()
	registration.options = options
	registration.emit = emit
	projector := registration.projector
	registration.mu.Unlock()
	if projector != nil {
		projector.SetEmit(emit)
	}
	return registration
}

func publicRegistrationKey(key agent.SessionKey, commandID string) string {
	encoded, _ := json.Marshal(key)
	return string(encoded) + "\x00" + strings.TrimSpace(commandID)
}
