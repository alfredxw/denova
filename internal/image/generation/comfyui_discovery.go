package generation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"

	"denova/config"
)

const (
	ComfyUIWorkflowStatusReady   = "ready"
	ComfyUIWorkflowStatusStale   = "stale"
	ComfyUIWorkflowStatusNotRun  = "not_run"
	ComfyUIWorkflowStatusInvalid = "invalid"
)

// ComfyUIWorkflowSummary reports whether one saved UI workflow has a fresh,
// successful API-format execution snapshot that Denova can safely reuse.
type ComfyUIWorkflowSummary struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	WorkflowID string `json:"workflow_id,omitempty"`
	Modified   int64  `json:"modified"`
	Status     string `json:"status"`
	JobID      string `json:"job_id,omitempty"`
	JobTime    int64  `json:"job_time,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

type ComfyUIWorkflowCatalog struct {
	Workflows []ComfyUIWorkflowSummary `json:"workflows"`
}

// ComfyUIWorkflowSnapshot is the executable graph, inferred semantic bindings,
// and transient repair candidates captured from one successful ComfyUI job.
type ComfyUIWorkflowSnapshot struct {
	ComfyUIWorkflowSummary
	Workflow   string                   `json:"workflow"`
	Bindings   *config.ComfyUIBindings  `json:"bindings,omitempty"`
	Candidates ComfyUIBindingCandidates `json:"candidates"`
}

type comfyUIUserFile struct {
	Path     string `json:"path"`
	Modified int64  `json:"modified"`
}

type comfyUISavedWorkflow struct {
	ID string `json:"id"`
}

type comfyUIJobSummary struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	CreateTime int64  `json:"create_time"`
	WorkflowID string `json:"workflow_id"`
}

type comfyUIJobsResponse struct {
	Jobs []comfyUIJobSummary `json:"jobs"`
}

type comfyUIJobDetail struct {
	Workflow struct {
		Prompt json.RawMessage `json:"prompt"`
	} `json:"workflow"`
}

// DiscoverWorkflows lists saved ComfyUI workflows and checks whether their
// latest successful jobs are new enough to represent the saved files.
func (adapter *ComfyUIAdapter) DiscoverWorkflows(ctx context.Context, profile config.ResolvedImageAPIProfile) (ComfyUIWorkflowCatalog, error) {
	files, err := adapter.listSavedWorkflows(ctx, profile)
	if err != nil {
		return ComfyUIWorkflowCatalog{}, err
	}
	if len(files) == 0 {
		return ComfyUIWorkflowCatalog{Workflows: []ComfyUIWorkflowSummary{}}, nil
	}
	workflows := make([]ComfyUIWorkflowSummary, 0, len(files))
	for _, file := range files {
		summary := ComfyUIWorkflowSummary{
			Name:     strings.TrimSuffix(path.Base(file.Path), path.Ext(file.Path)),
			Path:     file.Path,
			Modified: file.Modified,
			Status:   ComfyUIWorkflowStatusInvalid,
		}
		saved, fetchErr := adapter.savedWorkflow(ctx, profile, file.Path)
		if fetchErr != nil {
			summary.Detail = fetchErr.Error()
			workflows = append(workflows, summary)
			continue
		}
		summary.WorkflowID = strings.TrimSpace(saved.ID)
		if summary.WorkflowID == "" {
			summary.Detail = "saved workflow is missing id"
			workflows = append(workflows, summary)
			continue
		}
		job, jobErr := adapter.latestCompletedJob(ctx, profile, summary.WorkflowID)
		if jobErr != nil {
			summary.Detail = jobErr.Error()
			workflows = append(workflows, summary)
			continue
		}
		if job.ID == "" {
			summary.Status = ComfyUIWorkflowStatusNotRun
			workflows = append(workflows, summary)
			continue
		}
		summary.JobID = job.ID
		summary.JobTime = job.CreateTime
		if file.Modified > job.CreateTime {
			summary.Status = ComfyUIWorkflowStatusStale
		} else {
			summary.Status = ComfyUIWorkflowStatusReady
		}
		workflows = append(workflows, summary)
	}
	sort.Slice(workflows, func(i, j int) bool {
		return strings.ToLower(workflows[i].Path) < strings.ToLower(workflows[j].Path)
	})
	return ComfyUIWorkflowCatalog{Workflows: workflows}, nil
}

// LoadWorkflow revalidates one catalog entry, then infers runtime bindings and
// compatible repair candidates directly from its executable graph.
func (adapter *ComfyUIAdapter) LoadWorkflow(ctx context.Context, profile config.ResolvedImageAPIProfile, workflowPath string) (ComfyUIWorkflowSnapshot, error) {
	workflowPath, err := normalizeComfyUIWorkflowPath(workflowPath)
	if err != nil {
		return ComfyUIWorkflowSnapshot{}, err
	}
	files, err := adapter.listSavedWorkflows(ctx, profile)
	if err != nil {
		return ComfyUIWorkflowSnapshot{}, err
	}
	var file *comfyUIUserFile
	for index := range files {
		if files[index].Path == workflowPath {
			file = &files[index]
			break
		}
	}
	if file == nil {
		return ComfyUIWorkflowSnapshot{}, fmt.Errorf("saved workflow %q was not found", workflowPath)
	}
	saved, err := adapter.savedWorkflow(ctx, profile, workflowPath)
	if err != nil {
		return ComfyUIWorkflowSnapshot{}, err
	}
	workflowID := strings.TrimSpace(saved.ID)
	if workflowID == "" {
		return ComfyUIWorkflowSnapshot{}, fmt.Errorf("saved workflow %q is missing id", workflowPath)
	}
	job, err := adapter.latestCompletedJob(ctx, profile, workflowID)
	if err != nil {
		return ComfyUIWorkflowSnapshot{}, err
	}
	if job.ID == "" {
		return ComfyUIWorkflowSnapshot{}, fmt.Errorf("saved workflow %q has no successful execution", workflowPath)
	}
	if file.Modified > job.CreateTime {
		return ComfyUIWorkflowSnapshot{}, fmt.Errorf("saved workflow %q changed after its latest successful execution", workflowPath)
	}

	jobEndpoint, err := endpointURL(profile.BaseURL, "api/jobs/"+url.PathEscape(job.ID))
	if err != nil {
		return ComfyUIWorkflowSnapshot{}, err
	}
	var detail comfyUIJobDetail
	if err := doJSON(ctx, adapter.httpClient, http.MethodGet, jobEndpoint, bearerHeaders(profile.APIKey, profile.Headers), nil, &detail); err != nil {
		return ComfyUIWorkflowSnapshot{}, fmt.Errorf("load ComfyUI job %q: %w", job.ID, err)
	}
	if len(detail.Workflow.Prompt) > maxComfyUIWorkflowBytes {
		return ComfyUIWorkflowSnapshot{}, fmt.Errorf("ComfyUI workflow exceeds %d bytes", maxComfyUIWorkflowBytes)
	}
	workflow, err := decodeComfyUIWorkflowJSON(detail.Workflow.Prompt)
	if err != nil {
		return ComfyUIWorkflowSnapshot{}, fmt.Errorf("decode ComfyUI job workflow: %w", err)
	}
	if err := validateComfyUIWorkflow(workflow); err != nil {
		return ComfyUIWorkflowSnapshot{}, err
	}
	bindings, candidates := analyzeComfyUIBindings(workflow)
	return ComfyUIWorkflowSnapshot{
		ComfyUIWorkflowSummary: ComfyUIWorkflowSummary{
			Name:       strings.TrimSuffix(path.Base(file.Path), path.Ext(file.Path)),
			Path:       file.Path,
			WorkflowID: workflowID,
			Modified:   file.Modified,
			Status:     ComfyUIWorkflowStatusReady,
			JobID:      job.ID,
			JobTime:    job.CreateTime,
		},
		Workflow:   strings.TrimSpace(string(detail.Workflow.Prompt)),
		Bindings:   bindings,
		Candidates: candidates,
	}, nil
}

func (adapter *ComfyUIAdapter) listSavedWorkflows(ctx context.Context, profile config.ResolvedImageAPIProfile) ([]comfyUIUserFile, error) {
	endpoint, err := endpointURL(profile.BaseURL, "userdata")
	if err != nil {
		return nil, err
	}
	parsed, _ := url.Parse(endpoint)
	query := parsed.Query()
	query.Set("dir", "workflows")
	query.Set("recurse", "true")
	query.Set("full_info", "true")
	parsed.RawQuery = query.Encode()
	var files []comfyUIUserFile
	if err := doJSON(ctx, adapter.httpClient, http.MethodGet, parsed.String(), bearerHeaders(profile.APIKey, profile.Headers), nil, &files); err != nil {
		var statusErr *imageAPIHTTPError
		if errors.As(err, &statusErr) && statusErr.status == http.StatusNotFound {
			return []comfyUIUserFile{}, nil
		}
		return nil, fmt.Errorf("list saved ComfyUI workflows: %w", err)
	}
	out := make([]comfyUIUserFile, 0, len(files))
	for _, file := range files {
		normalized, normalizeErr := normalizeComfyUIWorkflowPath(file.Path)
		if normalizeErr != nil || !strings.EqualFold(path.Ext(normalized), ".json") {
			continue
		}
		file.Path = normalized
		out = append(out, file)
	}
	return out, nil
}

func (adapter *ComfyUIAdapter) savedWorkflow(ctx context.Context, profile config.ResolvedImageAPIProfile, workflowPath string) (comfyUISavedWorkflow, error) {
	workflowPath, err := normalizeComfyUIWorkflowPath(workflowPath)
	if err != nil {
		return comfyUISavedWorkflow{}, err
	}
	endpoint, err := endpointURL(profile.BaseURL, "userdata")
	if err != nil {
		return comfyUISavedWorkflow{}, err
	}
	endpoint += "/" + url.PathEscape(path.Join("workflows", workflowPath))
	var workflow comfyUISavedWorkflow
	if err := doJSON(ctx, adapter.httpClient, http.MethodGet, endpoint, bearerHeaders(profile.APIKey, profile.Headers), nil, &workflow); err != nil {
		return comfyUISavedWorkflow{}, fmt.Errorf("load saved ComfyUI workflow %q: %w", workflowPath, err)
	}
	return workflow, nil
}

func (adapter *ComfyUIAdapter) latestCompletedJob(ctx context.Context, profile config.ResolvedImageAPIProfile, workflowID string) (comfyUIJobSummary, error) {
	endpoint, err := comfyUIJobsEndpoint(profile.BaseURL, workflowID)
	if err != nil {
		return comfyUIJobSummary{}, err
	}
	var response comfyUIJobsResponse
	if err := doJSON(ctx, adapter.httpClient, http.MethodGet, endpoint, bearerHeaders(profile.APIKey, profile.Headers), nil, &response); err != nil {
		return comfyUIJobSummary{}, fmt.Errorf("find latest ComfyUI job for workflow %q: %w", workflowID, err)
	}
	if len(response.Jobs) == 0 {
		return comfyUIJobSummary{}, nil
	}
	return response.Jobs[0], nil
}

func comfyUIJobsEndpoint(baseURL, workflowID string) (string, error) {
	endpoint, err := endpointURL(baseURL, "api/jobs")
	if err != nil {
		return "", err
	}
	parsed, _ := url.Parse(endpoint)
	query := parsed.Query()
	query.Set("status", "completed")
	query.Set("sort_by", "created_at")
	query.Set("sort_order", "desc")
	query.Set("limit", "1")
	query.Set("offset", "0")
	if strings.TrimSpace(workflowID) != "" {
		query.Set("workflow_id", strings.TrimSpace(workflowID))
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func normalizeComfyUIWorkflowPath(value string) (string, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	cleaned := path.Clean(value)
	if cleaned == "." || path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("invalid saved ComfyUI workflow path %q", value)
	}
	return cleaned, nil
}
