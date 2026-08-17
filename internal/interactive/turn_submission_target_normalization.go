package interactive

import (
	"context"
	interactivestate "denova/internal/interactive/state"
	"fmt"
	"log/slog"
	"sort"
)

// normalizeTurnSubmissionStateUpdateTargets repairs an unambiguous model error:
// a field belonging to exactly one foundational runtime Actor was attached to
// another existing Actor. The repair is deliberately narrow. It never guesses
// among multiple owners, redirects to a dynamic Actor, or touches an Actor that
// is created by the same atomic submission.
func normalizeTurnSubmissionStateUpdateTargets(system StoryDirectorActorStateSystem, currentState map[string]any, updates []interactivestate.Update) []interactivestate.Update {
	normalized := interactivestate.NormalizeUpdates(updates)
	if len(normalized) == 0 {
		return normalized
	}
	system = normalizeActorStateSystem(system)
	createdActors := map[string]bool{}
	for _, update := range normalized {
		if update.Op != interactivestate.Create {
			continue
		}
		segments, err := interactivestate.ParsePath(update.Path)
		if err == nil && len(segments) == 1 {
			createdActors[segments[0]] = true
		}
	}
	actorIDs := turnSubmissionExistingActorIDs(system, currentState)
	for index, update := range normalized {
		if update.Op != interactivestate.Replace && update.Op != interactivestate.Delta {
			continue
		}
		segments, err := interactivestate.ParsePath(update.Path)
		if err != nil || len(segments) < 2 || createdActors[segments[0]] {
			continue
		}
		sourceTemplateID, sourceExists := actorTemplateIDFromStateOrSystem(currentState, system, segments[0])
		if !sourceExists {
			continue
		}
		if _, found := actorStateFieldByID(actorStateTemplateByID(system, sourceTemplateID), segments[1]); found {
			continue
		}
		matches := make([]string, 0, 2)
		for _, actorID := range actorIDs {
			if actorID == segments[0] || createdActors[actorID] {
				continue
			}
			templateID, found := actorTemplateIDFromStateOrSystem(currentState, system, actorID)
			if !found {
				continue
			}
			if _, found := actorStateFieldByID(actorStateTemplateByID(system, templateID), segments[1]); found {
				matches = append(matches, actorID)
			}
		}
		if len(matches) != 1 || !foundationalTurnSubmissionActor(matches[0]) {
			continue
		}
		originalPath := update.Path
		segments[0] = matches[0]
		update.Path = interactivestate.FormatPath(segments)
		normalized[index] = update
		slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-turn-submission] repaired unambiguous foundational Actor target from=%q to=%q field_id=%q location=internal/interactive/turn_submission_target_normalization.go", originalPath, update.Path, segments[1]))
	}
	return normalized
}

func turnSubmissionExistingActorIDs(system StoryDirectorActorStateSystem, currentState map[string]any) []string {
	seen := map[string]bool{}
	for _, actor := range system.InitialActors {
		if actor.ID != "" {
			seen[actor.ID] = true
		}
	}
	if actors, ok := currentState[actorStateRoot].(map[string]any); ok {
		for actorID := range actors {
			actorID = normalizeStatePanelActorID(actorID)
			if actorID != "" {
				seen[actorID] = true
			}
		}
	}
	actorIDs := make([]string, 0, len(seen))
	for actorID := range seen {
		actorIDs = append(actorIDs, actorID)
	}
	sort.Strings(actorIDs)
	return actorIDs
}

func foundationalTurnSubmissionActor(actorID string) bool {
	switch actorID {
	case DefaultActorID, DefaultStoryContextActorID, DefaultWorldEntitiesActorID:
		return true
	default:
		return false
	}
}
