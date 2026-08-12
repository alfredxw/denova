package continuallearning

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/agents/trajectory"
)

const (
	trajectoryKindSession = "session"
	trajectoryKindRun     = "run"
)

// Trajectories returns a bounded, newest-first evidence catalog. Project-local
// failures stay attached to their Project so one damaged dormant Project does
// not hide healthy evidence from every other Project.
func (service *Service) Trajectories(ctx context.Context, since time.Time, limit int) (TrajectoryList, error) {
	runtime, err := service.requireEnabled()
	if err != nil {
		return TrajectoryList{}, err
	}
	if since.IsZero() {
		since = time.Now().UTC().Add(-24 * time.Hour)
	} else {
		since = since.UTC()
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	sources, err := service.host.TrajectorySources(ctx)
	if err != nil {
		return TrajectoryList{}, err
	}
	result := TrajectoryList{Since: since, Items: make([]TrajectorySummary, 0), Issues: make([]TrajectoryIssue, 0)}
	perProjectLimit := runtime.Config.Labs.ContinualLearningTrajectoryCap
	if perProjectLimit <= 0 || perProjectLimit > 500 {
		perProjectLimit = 50
	}
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return TrajectoryList{}, err
		}
		result.Items, result.Issues = appendSessionTrajectories(result.Items, result.Issues, source, since, perProjectLimit)
		result.Items, result.Issues = appendRunTrajectories(result.Items, result.Issues, source, since, perProjectLimit)
	}
	sort.SliceStable(result.Items, func(left, right int) bool {
		leftTime := result.Items[left].UpdatedAt
		rightTime := result.Items[right].UpdatedAt
		if leftTime.Equal(rightTime) {
			return result.Items[left].URI < result.Items[right].URI
		}
		return leftTime.After(rightTime)
	})
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
	}
	return result, nil
}

func (service *Service) Trajectory(ctx context.Context, uri string) (TrajectoryContent, error) {
	runtime, err := service.requireEnabled()
	if err != nil {
		return TrajectoryContent{}, err
	}
	uri = strings.TrimSpace(uri)
	if !strings.HasPrefix(uri, trajectory.Scheme+"projects/") {
		return TrajectoryContent{}, fs.ErrNotExist
	}
	resource, err := (trajectory.Catalog{
		Sources:  service.host.TrajectorySources,
		Outcomes: service.outcomes,
		Limit:    runtime.Config.Labs.ContinualLearningTrajectoryCap,
	}).Read(ctx, uri, 1)
	if err != nil {
		return TrajectoryContent{}, err
	}
	return TrajectoryContent{URI: resource.URI, Kind: resource.Kind, Content: resource.Content}, nil
}

func appendSessionTrajectories(
	items []TrajectorySummary,
	issues []TrajectoryIssue,
	source trajectory.Source,
	since time.Time,
	limit int,
) ([]TrajectorySummary, []TrajectoryIssue) {
	directory := filepath.Join(source.StateRoot, "sessions")
	if _, err := os.Stat(directory); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return items, issues
		}
		return items, appendTrajectoryIssue(issues, source, err)
	}
	store, err := session.NewStore(directory)
	if err != nil {
		return items, appendTrajectoryIssue(issues, source, err)
	}
	metas, listErr := store.List("")
	closeErr := store.Close()
	if listErr != nil {
		return items, appendTrajectoryIssue(issues, source, listErr)
	}
	if closeErr != nil {
		return items, appendTrajectoryIssue(issues, source, closeErr)
	}
	appended := 0
	for _, meta := range metas {
		if meta.UpdatedAt.Before(since) {
			continue
		}
		items = append(items, TrajectorySummary{
			URI:          trajectoryResourceURI(source.ProjectID, "sessions", meta.ID),
			Kind:         trajectoryKindSession,
			ProjectID:    source.ProjectID,
			ProjectName:  source.Name,
			ID:           meta.ID,
			Title:        meta.Title,
			CreatedAt:    meta.CreatedAt.UTC(),
			UpdatedAt:    meta.UpdatedAt.UTC(),
			MessageCount: meta.MessageCount,
		})
		appended++
		if appended >= limit {
			break
		}
	}
	return items, issues
}

func appendRunTrajectories(
	items []TrajectorySummary,
	issues []TrajectoryIssue,
	source trajectory.Source,
	since time.Time,
	limit int,
) ([]TrajectorySummary, []TrajectoryIssue) {
	runs, err := agentrun.ListRunTraces(agentrun.TraceLocation{Workspace: source.Workspace, StateRoot: source.StateRoot}, limit)
	if err != nil {
		return items, appendTrajectoryIssue(issues, source, err)
	}
	for _, run := range runs {
		createdAt := run.CreatedAt.UTC()
		if createdAt.Before(since) {
			continue
		}
		items = append(items, TrajectorySummary{
			URI:         trajectoryResourceURI(source.ProjectID, "runs", run.ID),
			Kind:        trajectoryKindRun,
			ProjectID:   source.ProjectID,
			ProjectName: source.Name,
			ID:          run.ID,
			AgentKind:   run.AgentKind,
			Status:      run.Status,
			CreatedAt:   createdAt,
			UpdatedAt:   createdAt,
			EventCount:  run.Events,
			ToolCalls:   run.ToolCalls,
			DurationMS:  run.DurationMS,
		})
	}
	return items, issues
}

func appendTrajectoryIssue(issues []TrajectoryIssue, source trajectory.Source, err error) []TrajectoryIssue {
	slog.Warn("[harness-optimization] trajectory project read failed", "project_id", source.ProjectID, "error", err)
	return append(issues, TrajectoryIssue{ProjectID: source.ProjectID, Message: "trajectory evidence is unavailable for this project"})
}

func trajectoryResourceURI(projectID, category, id string) string {
	return trajectory.Scheme + "projects/" + url.PathEscape(projectID) + "/" + category + "/" + url.PathEscape(id)
}
