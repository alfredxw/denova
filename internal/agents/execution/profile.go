package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ProfileID is the stable product execution profile persisted in a durable
// binding. These values are part of the existing runtime identity and must stay
// aligned with the binding codec.
type ProfileID string

const (
	ProfileWriting          ProfileID = "writing"
	ProfileAgentChat        ProfileID = "agent_chat"
	ProfileGame             ProfileID = "game"
	ProfileConfigManager    ProfileID = "config_manager"
	ProfileHarnessOptimizer ProfileID = "harness_optimizer"
	ProfileImage            ProfileID = "image"
)

var (
	ErrProfileInvalid   = errors.New("agent execution profile is invalid")
	ErrProfileDuplicate = errors.New("agent execution profile is already registered")
	ErrProfileNotFound  = errors.New("agent execution profile is not registered")
)

// Profile identifies one Denova product adapter that can rebuild a public
// Agent Definition from durable host data.
type Profile interface {
	ID() ProfileID
}

// QueuedCycleProfile rebuilds process-local dependencies for any accepted or
// cold-recovered cycle. Every registered Denova profile must implement it.
type QueuedCycleProfile interface {
	Profile
	PrepareCycle(context.Context, CycleRestoreRequest) (Cycle, error)
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
		if _, ok := profile.(QueuedCycleProfile); !ok {
			return nil, fmt.Errorf("%w: profile %q cannot prepare a cycle", ErrProfileInvalid, id)
		}
		registry.profiles[id] = profile
	}
	return registry, nil
}

func validProfileID(id ProfileID) bool {
	switch id {
	case ProfileWriting, ProfileAgentChat, ProfileGame, ProfileConfigManager, ProfileHarnessOptimizer, ProfileImage:
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
