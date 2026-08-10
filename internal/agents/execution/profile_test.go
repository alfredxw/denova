package execution

import (
	"context"
	"errors"
	"testing"

	"denova/internal/agents/run"
)

type identityOnlyProfile struct{ id ProfileID }

func (profile identityOnlyProfile) ID() ProfileID { return profile.id }

type inputOnlyProfile struct{ identityOnlyProfile }

func (inputOnlyProfile) PlanInput(context.Context, InputMaterializationRequest) (agentrun.InputMaterializationPlan, error) {
	return agentrun.InputMaterializationPlan{}, nil
}

func (inputOnlyProfile) MaterializeInput(context.Context, InputMaterializationRequest, agentrun.InputMaterializationPlan) (agentrun.InputMaterializationReceipt, error) {
	return agentrun.InputMaterializationReceipt{}, nil
}

func TestProfileRegistryRejectsIdentityWithoutExecutionCapability(t *testing.T) {
	_, err := newProfileRegistry([]Profile{identityOnlyProfile{id: ProfileWriting}})
	if !errors.Is(err, ErrProfileInvalid) {
		t.Fatalf("identity-only profile error = %v", err)
	}
}

func TestProfileRegistryRejectsInputWithoutDomainReconciliation(t *testing.T) {
	_, err := newProfileRegistry([]Profile{inputOnlyProfile{identityOnlyProfile{id: ProfileWriting}}})
	if !errors.Is(err, ErrProfileInvalid) {
		t.Fatalf("input-only profile error = %v", err)
	}
}
