package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agentstructural "denova/internal/agents/context/structural"
	agentrun "denova/internal/agents/run"
)

// ProfileID is the stable product execution profile persisted in a durable
// binding. These values are part of the existing runtime identity and must stay
// aligned with the binding codec.
type ProfileID string

const (
	ProfileWriting       ProfileID = "writing"
	ProfileAgentChat     ProfileID = "agent_chat"
	ProfileGame          ProfileID = "game"
	ProfileAutomation    ProfileID = "automation"
	ProfileConfigManager ProfileID = "config_manager"
	ProfileImage         ProfileID = "image"
	ProfileDirector      ProfileID = "director"
)

var (
	ErrProfileInvalid                  = errors.New("agent execution profile is invalid")
	ErrProfileDuplicate                = errors.New("agent execution profile is already registered")
	ErrProfileNotFound                 = errors.New("agent execution profile is not registered")
	ErrInputMaterializationUnavailable = errors.New("agent execution input materialization capability is unavailable")
	ErrDomainCommitUnavailable         = errors.New("agent execution domain commit capability is unavailable")
)

// Profile is the stable identity for one product execution adapter. Optional
// runtime behavior is expressed only through the capability interfaces below;
// profiles never publish nil callbacks or methods that are designed to fail.
type Profile interface {
	ID() ProfileID
}

// QueuedCycleProfile rebuilds process-local dependencies for an accepted
// queued or cold-recovered cycle. Start-only profiles do not implement it.
type QueuedCycleProfile interface {
	Profile
	PrepareCycle(context.Context, CycleRestoreRequest) (Cycle, error)
}

// InputProfile owns canonical user-input planning and idempotent append.
// Profiles without canonical user input do not implement it.
type InputProfile interface {
	Profile
	PlanInput(context.Context, InputMaterializationRequest) (agentrun.InputMaterializationPlan, error)
	MaterializeInput(context.Context, InputMaterializationRequest, agentrun.InputMaterializationPlan) (agentrun.InputMaterializationReceipt, error)
}

// DomainCommitProfile reconciles an exact durable commit identity against the
// owning canonical store. It is required by InputProfile and StructuralProfile.
type DomainCommitProfile interface {
	Profile
	ReconcileDomainCommit(context.Context, agentrun.DomainCommitReconcileRequest) (agentrun.DomainCommitReconcileResult, error)
}

// StructuralProfile is implemented only by profiles that support durable
// context compaction or removal. The capability is explicit at registration;
// recovery never guesses support from Agent kind.
type StructuralProfile interface {
	Profile
	RestoreStructural(context.Context, StructuralRestoreRequest) (agentstructural.Spec, error)
}

type profileRegistry struct {
	profiles map[ProfileID]Profile
}

func newProfileRegistry(profiles []Profile) (*profileRegistry, error) {
	registry := &profileRegistry{profiles: make(map[ProfileID]Profile, len(profiles))}
	for index, profile := range profiles {
		if profile == nil {
			return nil, fmt.Errorf("%w: profile at index %d is nil", ErrProfileInvalid, index)
		}
		id := ProfileID(strings.TrimSpace(string(profile.ID())))
		if !validProfileID(id) {
			return nil, fmt.Errorf("%w: unsupported profile id %q", ErrProfileInvalid, id)
		}
		if _, exists := registry.profiles[id]; exists {
			return nil, fmt.Errorf("%w: %q", ErrProfileDuplicate, id)
		}
		if err := validateProfileCapabilities(profile); err != nil {
			return nil, fmt.Errorf("%w: profile %q: %v", ErrProfileInvalid, id, err)
		}
		registry.profiles[id] = profile
	}
	return registry, nil
}

func validateProfileCapabilities(profile Profile) error {
	_, queued := profile.(QueuedCycleProfile)
	_, input := profile.(InputProfile)
	_, domain := profile.(DomainCommitProfile)
	_, structural := profile.(StructuralProfile)
	if !queued && !input && !domain && !structural {
		return errors.New("no execution capability is registered")
	}
	if input && !domain {
		return errors.New("input materialization requires domain commit reconciliation")
	}
	if structural && !domain {
		return errors.New("structural recovery requires domain commit reconciliation")
	}
	return nil
}

func validProfileID(id ProfileID) bool {
	switch id {
	case ProfileWriting, ProfileAgentChat, ProfileGame, ProfileAutomation, ProfileConfigManager, ProfileImage, ProfileDirector:
		return true
	default:
		return false
	}
}

func (registry *profileRegistry) profile(id string) (Profile, error) {
	resolved := ProfileID(strings.TrimSpace(id))
	if registry == nil {
		return nil, fmt.Errorf("%w: %q", ErrProfileNotFound, resolved)
	}
	profile, ok := registry.profiles[resolved]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrProfileNotFound, resolved)
	}
	return profile, nil
}

func (registry *profileRegistry) empty() bool {
	return registry == nil || len(registry.profiles) == 0
}
