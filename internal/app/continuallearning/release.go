package continuallearning

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"

	agentstate "github.com/alfredxw/denova/agent/state"
)

// Publish atomically replaces the complete runtime-facing Harness State with
// one validated Draft snapshot. Publication is intentionally global because
// prompts, contexts, tools, and subagents can target more than one Agent.
func (service *Service) Publish(ctx context.Context, request StatePublishRequest) (StatePublishResult, error) {
	if _, err := service.requireEnabled(); err != nil {
		return StatePublishResult{}, err
	}
	draftRevision := strings.TrimSpace(request.DraftRevision)
	publishedRevision := strings.TrimSpace(request.PublishedRevision)
	if draftRevision == "" || publishedRevision == "" {
		return StatePublishResult{}, errors.New("draft_revision and published_revision are required")
	}
	var result StatePublishResult
	err := service.history.withLock(ctx, func() error {
		draft, _, err := service.manager.ValidatedAtRevision(ctx, draftRevision)
		if err != nil {
			return err
		}
		published, err := service.published.Store().Current(ctx)
		if err != nil {
			return err
		}
		if published.Revision != publishedRevision {
			return fmt.Errorf("%w: expected published=%s current=%s", agentstate.ErrConflict, publishedRevision, published.Revision)
		}
		result = StatePublishResult{
			DraftRevision: draft.Revision, PublishedRevision: published.Revision,
		}
		if draft.Revision == published.Revision {
			return nil
		}
		version, _, err := service.history.record(draft, normalizePublishSummary(request.Summary))
		if err != nil {
			return fmt.Errorf("record Harness Draft before publish: %w", err)
		}
		currentDraft, err := service.manager.Store().Current(ctx)
		if err != nil {
			return err
		}
		if currentDraft.Revision != draft.Revision {
			return fmt.Errorf("%w: expected draft=%s current=%s", agentstate.ErrConflict, draft.Revision, currentDraft.Revision)
		}
		updated, err := service.published.Store().Update(ctx, agentstate.ChangeSet{
			BaseRevision: published.Revision,
			Changes:      stateReplacementChanges(published.Files(), draft.Files()),
		})
		if err != nil {
			return err
		}
		if updated.CleanupError != nil {
			slog.WarnContext(ctx, "[continual-learning] published Harness State with deferred cleanup", "error", updated.CleanupError)
		}
		result.Version = version
		result.PublishedRevision = updated.Snapshot.Revision
		result.Changed = updated.Changed
		var publishedVersion StateVersionID
		if version != nil {
			publishedVersion = version.ID
		}
		slog.InfoContext(ctx, "[continual-learning] published complete Harness State",
			"revision", updated.Snapshot.Revision, "version", publishedVersion)
		return nil
	})
	return result, err
}

// Debug returns the exact model-free Draft projection for one Agent. The
// revision check makes the preview a reproducible pre-publication artifact.
func (service *Service) Debug(ctx context.Context, agentKind, revision string) (StateDebugResult, error) {
	if _, err := service.requireEnabled(); err != nil {
		return StateDebugResult{}, err
	}
	agentKind = strings.TrimSpace(agentKind)
	if _, ok := config.LookupAgentKind(agentKind); !ok {
		return StateDebugResult{}, fmt.Errorf("unknown Agent kind %q", agentKind)
	}
	revision = strings.TrimSpace(revision)
	if revision == "" {
		return StateDebugResult{}, errors.New("revision is required")
	}
	snapshot, harness, err := service.manager.ValidatedAtRevision(ctx, revision)
	if err != nil {
		return StateDebugResult{}, err
	}
	return StateDebugResult{Revision: snapshot.Revision, AgentDebugProjection: harness.DebugProjection(agentKind)}, nil
}

func normalizePublishSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return "Publish Harness State"
	}
	return summary
}
