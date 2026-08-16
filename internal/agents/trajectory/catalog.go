// Package trajectory projects existing Agent Sessions, Run traces, and
// explicit outcome feedback as read-only resources for learning Agents.
package trajectory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"

	agent "github.com/alfredxw/denova/agent"
	agenttools "github.com/alfredxw/denova/agent/tools"
)

const Scheme = "trajectory://"

const maxResourceURIBytes = 4096

// RunURI returns the stable resource identifier shared by product surfaces and
// the Harness Optimizer. Callers never need to reproduce URI escaping rules.
func RunURI(projectID, runID string) string {
	return Scheme + "projects/" + url.PathEscape(projectID) + "/runs/" + url.PathEscape(runID)
}

type Source struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Workspace string `json:"-"`
	StateRoot string `json:"-"`
}

type SourceProvider func(context.Context) ([]Source, error)

type Catalog struct {
	Sources  SourceProvider
	Outcomes *OutcomeStore
	Limit    int
}

// Resource is one redacted, model-visible trajectory document. Product
// surfaces use the same projection as the read adapter so viewing evidence and
// asking Harness Optimizer to analyze it can never drift into two contracts.
type Resource struct {
	URI     string `json:"uri"`
	Kind    string `json:"kind"`
	Content string `json:"content"`
}

type readInput struct {
	Path  string `json:"path" jsonschema_description:"Trajectory resource URI: trajectory://index, trajectory://outcomes, trajectory://projects/{project_id}/sessions/{session_id}, or trajectory://projects/{project_id}/runs/{run_id}."`
	Limit int    `json:"limit,omitempty" jsonschema:"minimum=1" jsonschema_description:"Maximum index or outcome entries. Defaults to the configured trajectory cap and cannot exceed 500."`
}

type projectIndex struct {
	Source   Source                     `json:"source"`
	Sessions []session.SessionMeta      `json:"sessions"`
	Runs     []agentrun.RunTraceSummary `json:"runs"`
}

type indexDocument struct {
	Schema   string         `json:"schema"`
	Projects []projectIndex `json:"projects"`
	Outcomes string         `json:"outcomes"`
}

// NewReadAdapter creates the trajectory:// contribution to the ordinary read
// tool. It never exposes raw filesystem paths to the model.
func NewReadAdapter(catalog Catalog) (agenttools.ReadAdapter, error) {
	if catalog.Sources == nil {
		return nil, errors.New("trajectory source provider is required")
	}
	return agenttools.NewReadAdapter(agent.CapabilityIdentity{
		Kind: "denova.read.trajectory", Version: 1,
		ConfigHash: fmt.Sprintf("limit=%d", effectiveTrajectoryLimit(catalog.Limit)),
	}, "trajectory", func(_ context.Context, resource string) (bool, error) {
		return strings.HasPrefix(strings.ToLower(strings.TrimSpace(resource)), Scheme), nil
	}, catalog.read)
}

// Read returns the exact redacted resource exposed through the ordinary Agent
// read tool. The limit applies only to index and outcome resources.
func (catalog Catalog) Read(ctx context.Context, resource string, limit int) (Resource, error) {
	if catalog.Sources == nil {
		return Resource{}, errors.New("trajectory source provider is required")
	}
	result, err := catalog.read(ctx, readInput{Path: resource, Limit: limit})
	if err != nil {
		return Resource{}, err
	}
	return Resource{URI: result.Path, Kind: result.Kind, Content: result.Content}, nil
}

func (catalog Catalog) read(ctx context.Context, input readInput) (agenttools.ReadResult, error) {
	resource := strings.TrimSpace(input.Path)
	if len(resource) > maxResourceURIBytes {
		return agenttools.ReadResult{}, fmt.Errorf("trajectory resource URI exceeds %d bytes", maxResourceURIBytes)
	}
	parsed, err := url.Parse(resource)
	if err != nil || parsed.Scheme != "trajectory" {
		return agenttools.ReadResult{}, fmt.Errorf("invalid trajectory resource %q", resource)
	}
	if input.Limit > 500 {
		return agenttools.ReadResult{}, errors.New("trajectory limit cannot exceed 500")
	}
	limit := input.Limit
	if limit <= 0 {
		limit = effectiveTrajectoryLimit(catalog.Limit)
	}
	segments := pathSegments(parsed)
	if parsed.Host == "index" && len(segments) == 0 {
		return catalog.readIndex(ctx, resource, limit)
	}
	if parsed.Host == "outcomes" && len(segments) == 0 {
		return catalog.readOutcomes(resource, limit)
	}
	if parsed.Host != "projects" || len(segments) != 3 {
		return agenttools.ReadResult{}, fs.ErrNotExist
	}
	source, err := catalog.source(ctx, segments[0])
	if err != nil {
		return agenttools.ReadResult{}, err
	}
	switch segments[1] {
	case "sessions":
		directory := sessionDir(source.StateRoot)
		if _, err := os.Stat(directory); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return agenttools.ReadResult{}, fs.ErrNotExist
			}
			return agenttools.ReadResult{}, err
		}
		store, err := session.NewStore(directory)
		if err != nil {
			return agenttools.ReadResult{}, err
		}
		defer store.Close()
		target, err := store.Get(segments[2])
		if err != nil {
			return agenttools.ReadResult{}, err
		}
		return redactedJSONResult(resource, "trajectory_session", map[string]any{
			"schema": "denova.trajectory.session.v1", "project": source, "session_id": segments[2], "history": trajectorySessionHistory(target.History()),
		}, source)
	case "runs":
		trace, err := agentrun.ReadRunTrace(agentrun.TraceLocation{Workspace: source.Workspace, StateRoot: source.StateRoot}, segments[2])
		if err != nil {
			return agenttools.ReadResult{}, err
		}
		return redactedJSONResult(resource, "trajectory_run", trace, source)
	default:
		return agenttools.ReadResult{}, fs.ErrNotExist
	}
}

func effectiveTrajectoryLimit(limit int) int {
	if limit <= 0 || limit > 500 {
		return 50
	}
	return limit
}

func (catalog Catalog) readIndex(ctx context.Context, resource string, limit int) (agenttools.ReadResult, error) {
	sources, err := catalog.Sources(ctx)
	if err != nil {
		return agenttools.ReadResult{}, err
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].ProjectID < sources[j].ProjectID })
	document := indexDocument{Schema: "denova.trajectory.index.v1", Outcomes: Scheme + "outcomes"}
	for _, source := range sources {
		entry := projectIndex{Source: source}
		directory := sessionDir(source.StateRoot)
		_, statErr := os.Stat(directory)
		if statErr == nil {
			store, openErr := session.NewStore(directory)
			if openErr == nil {
				entry.Sessions, openErr = store.List("")
				if closeErr := store.Close(); openErr == nil {
					openErr = closeErr
				}
			}
			if openErr != nil {
				return agenttools.ReadResult{}, fmt.Errorf("read trajectory sessions for project %s: %w", source.ProjectID, openErr)
			}
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return agenttools.ReadResult{}, fmt.Errorf("inspect trajectory sessions for project %s: %w", source.ProjectID, statErr)
		}
		sort.Slice(entry.Sessions, func(i, j int) bool { return entry.Sessions[i].UpdatedAt.After(entry.Sessions[j].UpdatedAt) })
		if len(entry.Sessions) > limit {
			entry.Sessions = entry.Sessions[:limit]
		}
		entry.Runs, err = agentrun.ListRunTraces(agentrun.TraceLocation{Workspace: source.Workspace, StateRoot: source.StateRoot}, limit)
		if err != nil {
			return agenttools.ReadResult{}, fmt.Errorf("read trajectory runs for project %s: %w", source.ProjectID, err)
		}
		for index := range entry.Runs {
			entry.Runs[index].Path = ""
		}
		document.Projects = append(document.Projects, entry)
	}
	return jsonResult(resource, "trajectory_index", document)
}

func (catalog Catalog) readOutcomes(resource string, limit int) (agenttools.ReadResult, error) {
	if catalog.Outcomes == nil {
		return jsonResult(resource, "trajectory_outcomes", []Outcome{})
	}
	outcomes, err := catalog.Outcomes.List(limit)
	if err != nil {
		return agenttools.ReadResult{}, err
	}
	return jsonResult(resource, "trajectory_outcomes", outcomes)
}

func (catalog Catalog) source(ctx context.Context, projectID string) (Source, error) {
	sources, err := catalog.Sources(ctx)
	if err != nil {
		return Source{}, err
	}
	for _, source := range sources {
		if source.ProjectID == projectID {
			return source, nil
		}
	}
	return Source{}, fs.ErrNotExist
}

func jsonResult(resource, kind string, value any) (agenttools.ReadResult, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return agenttools.ReadResult{}, err
	}
	return agenttools.ReadResult{Path: resource, Kind: kind, Content: string(encoded), Total: len(encoded)}, nil
}

func redactedJSONResult(resource, kind string, value any, source Source) (agenttools.ReadResult, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return agenttools.ReadResult{}, err
	}
	var document any
	if err := json.Unmarshal(encoded, &document); err != nil {
		return agenttools.ReadResult{}, err
	}
	return jsonResult(resource, kind, redactTrajectoryValue(document, source))
}

func redactTrajectoryValue(value any, source Source) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = redactTrajectoryValue(item, source)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = redactTrajectoryValue(item, source)
		}
		return result
	case string:
		result := typed
		for _, privateRoot := range []string{source.StateRoot, source.Workspace} {
			privateRoot = strings.TrimSpace(privateRoot)
			if privateRoot != "" {
				result = strings.ReplaceAll(result, privateRoot, "[private-root]")
			}
		}
		return result
	default:
		return value
	}
}

// trajectorySessionHistory exposes observable interaction evidence, not the
// model's private reasoning stream. Final/progress assistant output and tool
// outcomes remain available for critique.
func trajectorySessionHistory(history []session.HistoryEntry) []session.HistoryEntry {
	result := make([]session.HistoryEntry, 0, len(history))
	for _, entry := range history {
		if entry.Role == "thinking" {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func pathSegments(parsed *url.URL) []string {
	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		return nil
	}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		decoded, err := url.PathUnescape(part)
		if err != nil || decoded == "" || decoded == "." || decoded == ".." || strings.ContainsAny(decoded, `/\\`) {
			return []string{"__invalid__", strconv.Itoa(len(parts))}
		}
		result = append(result, decoded)
	}
	return result
}

func sessionDir(stateRoot string) string { return filepath.Join(stateRoot, "sessions") }
