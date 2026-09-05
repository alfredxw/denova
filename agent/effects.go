package agent

import "context"

// EffectApplier accepts idempotent tool effects without owning the Agent's
// conversation journal. It returns one result per request, including failures.
// This lets delegated Agents keep their own transcripts while sharing product
// mutation handling with their parent.
type EffectApplier interface {
	Identity() CapabilityIdentity
	ApplyEffects(context.Context, []EffectRequest) ([]EffectResult, error)
}

type EffectApplierFuncs struct {
	CapabilityIdentity CapabilityIdentity
	ApplyEffectsFn     func(context.Context, []EffectRequest) ([]EffectResult, error)
}

func (applier EffectApplierFuncs) Identity() CapabilityIdentity {
	return applier.CapabilityIdentity
}

func (applier EffectApplierFuncs) ApplyEffects(ctx context.Context, requests []EffectRequest) ([]EffectResult, error) {
	if applier.ApplyEffectsFn == nil {
		return nil, ErrCapabilityUnsupported
	}
	return applier.ApplyEffectsFn(ctx, requests)
}
