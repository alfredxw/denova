package interactive

import (
	mathrand "math/rand"
	"strings"
	"time"
)

type OpeningRollRequest struct {
	TellerID         string   `json:"teller_id,omitempty"`
	StoryDirectorID  string   `json:"story_director_id,omitempty"`
	SelectedTraitIDs []string `json:"selected_trait_ids,omitempty"`
	LockedTraitIDs   []string `json:"locked_trait_ids,omitempty"`
	Seed             int64    `json:"seed,omitempty"`
}

type OpeningRollResult struct {
	TellerID        string               `json:"teller_id,omitempty"`
	StoryDirectorID string               `json:"story_director_id,omitempty"`
	Seed            int64                `json:"seed"`
	Traits          []OpeningRolledTrait `json:"traits"`
	StateOps        []StateOp            `json:"state_ops"`
}

type OpeningRolledTrait struct {
	PoolID  string `json:"pool_id"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
}

func RollOpening(teller Teller, req OpeningRollRequest) (OpeningRollResult, error) {
	teller = normalizeTeller(teller)
	seed := req.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	result := OpeningRollResult{
		TellerID: firstNonEmptyString(strings.TrimSpace(req.TellerID), teller.ID),
		Seed:     seed,
		Traits:   []OpeningRolledTrait{},
		StateOps: append([]StateOp(nil), teller.Orchestration.Opening.InitialStateOps...),
	}
	if teller.Orchestration == nil || !teller.Orchestration.Opening.Enabled {
		return result, nil
	}
	rng := mathrand.New(mathrand.NewSource(seed))
	selected := normalizeStringListLimit(req.SelectedTraitIDs, 64)
	locked := normalizeStringListLimit(req.LockedTraitIDs, 64)
	for _, pool := range teller.Orchestration.Opening.TraitPools {
		picked := pickOpeningTraits(pool, selected, locked, rng)
		for _, trait := range picked {
			result.Traits = append(result.Traits, OpeningRolledTrait{
				PoolID:  pool.ID,
				ID:      trait.ID,
				Name:    trait.Name,
				Summary: trait.Summary,
			})
			result.StateOps = append(result.StateOps, trait.Ops...)
			result.StateOps = append(result.StateOps, StateOp{
				Op:   "push",
				Path: "rules.opening_traits",
				Value: map[string]any{
					"pool_id": pool.ID,
					"id":      trait.ID,
					"name":    trait.Name,
					"summary": trait.Summary,
				},
			})
		}
	}
	result.StateOps = normalizeStateOps(result.StateOps)
	return result, nil
}

func RollOpeningWithStoryDirector(director StoryDirector, req OpeningRollRequest) (OpeningRollResult, error) {
	director = normalizeStoryDirector(director)
	seed := req.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	result := OpeningRollResult{
		StoryDirectorID: firstNonEmptyString(NormalizeStoryDirectorID(req.StoryDirectorID), director.ID),
		Seed:            seed,
		Traits:          []OpeningRolledTrait{},
		StateOps:        StoryDirectorInitialStateOps(director),
	}
	if !director.OpeningSelector.Enabled {
		return result, nil
	}
	rng := mathrand.New(mathrand.NewSource(seed))
	selected := normalizeStringListLimit(req.SelectedTraitIDs, 64)
	locked := normalizeStringListLimit(req.LockedTraitIDs, 64)
	for _, pool := range director.OpeningSelector.TraitPools {
		picked := pickOpeningTraits(pool, selected, locked, rng)
		for _, trait := range picked {
			result.Traits = append(result.Traits, OpeningRolledTrait{
				PoolID:  pool.ID,
				ID:      trait.ID,
				Name:    trait.Name,
				Summary: trait.Summary,
			})
			result.StateOps = append(result.StateOps, trait.Ops...)
			result.StateOps = append(result.StateOps, StateOp{
				Op:   "push",
				Path: "rules.opening_traits",
				Value: map[string]any{
					"pool_id": pool.ID,
					"id":      trait.ID,
					"name":    trait.Name,
					"summary": trait.Summary,
				},
			})
		}
	}
	result.StateOps = normalizeStateOps(result.StateOps)
	return result, nil
}

func pickOpeningTraits(pool OpeningTraitPool, selected, locked []string, rng *mathrand.Rand) []OpeningTrait {
	picked := make([]OpeningTrait, 0, pool.DrawCount)
	used := map[string]bool{}
	for _, id := range append(append([]string(nil), locked...), selected...) {
		for _, trait := range pool.Traits {
			if used[trait.ID] || trait.ID != id {
				continue
			}
			picked = append(picked, trait)
			used[trait.ID] = true
			if len(picked) >= pool.DrawCount {
				return picked
			}
		}
	}
	candidates := append([]OpeningTrait(nil), pool.Traits...)
	for len(picked) < pool.DrawCount && len(candidates) > 0 {
		index := weightedOpeningTraitIndex(candidates, rng)
		trait := candidates[index]
		candidates = append(candidates[:index], candidates[index+1:]...)
		if used[trait.ID] {
			continue
		}
		picked = append(picked, trait)
		used[trait.ID] = true
	}
	return picked
}

func weightedOpeningTraitIndex(traits []OpeningTrait, rng *mathrand.Rand) int {
	total := 0.0
	for _, trait := range traits {
		total += trait.Weight
	}
	if total <= 0 {
		return rng.Intn(len(traits))
	}
	target := rng.Float64() * total
	for i, trait := range traits {
		target -= trait.Weight
		if target <= 0 {
			return i
		}
	}
	return len(traits) - 1
}

func openingTraitNames(traits []OpeningRolledTrait) string {
	names := make([]string, 0, len(traits))
	for _, trait := range traits {
		names = append(names, trait.Name)
	}
	return strings.Join(names, "、")
}
