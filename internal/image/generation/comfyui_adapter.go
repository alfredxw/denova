package generation

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"denova/config"

	"github.com/google/uuid"
)

const comfyUIHistoryPollInterval = 500 * time.Millisecond

type ComfyUIAdapter struct {
	httpClient *http.Client
}

func NewComfyUIAdapter(httpClient *http.Client) *ComfyUIAdapter {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &ComfyUIAdapter{httpClient: httpClient}
}

type comfyUIPromptRequest struct {
	Prompt   comfyUIWorkflow `json:"prompt"`
	ClientID string          `json:"client_id"`
}

type comfyUIPromptResponse struct {
	PromptID   string         `json:"prompt_id"`
	NodeErrors map[string]any `json:"node_errors"`
}

type comfyUIHistoryEntry struct {
	Outputs map[string]comfyUIOutput `json:"outputs"`
	Status  comfyUIStatus            `json:"status"`
}

type comfyUIOutput struct {
	Images []comfyUIImageRef `json:"images"`
}

type comfyUIImageRef struct {
	Filename  string `json:"filename"`
	Subfolder string `json:"subfolder"`
	Type      string `json:"type"`
}

type comfyUIStatus struct {
	StatusString string `json:"status_str"`
	Completed    bool   `json:"completed"`
	Messages     []any  `json:"messages"`
}

func (adapter *ComfyUIAdapter) Generate(ctx context.Context, profile config.ResolvedImageAPIProfile, request GenerateRequest) (Result, error) {
	workflow, err := prepareComfyUIWorkflow(profile, request)
	if err != nil {
		return Result{}, err
	}
	promptEndpoint, err := endpointURL(profile.BaseURL, "prompt")
	if err != nil {
		return Result{}, err
	}
	var submitted comfyUIPromptResponse
	if err := doJSON(ctx, adapter.httpClient, http.MethodPost, promptEndpoint, bearerHeaders(profile.APIKey, profile.Headers), comfyUIPromptRequest{
		Prompt: workflow, ClientID: uuid.NewString(),
	}, &submitted); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(submitted.PromptID) == "" {
		if len(submitted.NodeErrors) > 0 {
			return Result{}, fmt.Errorf("ComfyUI rejected the workflow: %v", submitted.NodeErrors)
		}
		return Result{}, fmt.Errorf("ComfyUI response is missing prompt_id")
	}
	entry, err := adapter.waitForHistory(ctx, profile, submitted.PromptID)
	if err != nil {
		return Result{}, err
	}
	return adapter.resultFromHistory(ctx, profile, request, entry)
}

func (adapter *ComfyUIAdapter) waitForHistory(ctx context.Context, profile config.ResolvedImageAPIProfile, promptID string) (comfyUIHistoryEntry, error) {
	historyEndpoint, err := endpointURL(profile.BaseURL, "history/"+url.PathEscape(promptID))
	if err != nil {
		return comfyUIHistoryEntry{}, err
	}
	ticker := time.NewTicker(comfyUIHistoryPollInterval)
	defer ticker.Stop()
	for {
		var history map[string]comfyUIHistoryEntry
		if err := doJSON(ctx, adapter.httpClient, http.MethodGet, historyEndpoint, bearerHeaders(profile.APIKey, profile.Headers), nil, &history); err != nil {
			return comfyUIHistoryEntry{}, err
		}
		if entry, ok := history[promptID]; ok {
			status := strings.ToLower(strings.TrimSpace(entry.Status.StatusString))
			if entry.Status.Completed {
				if status != "" && status != "success" {
					return comfyUIHistoryEntry{}, fmt.Errorf("ComfyUI execution failed with status %q", entry.Status.StatusString)
				}
				return entry, nil
			}
			if status == "error" || status == "failed" {
				return comfyUIHistoryEntry{}, fmt.Errorf("ComfyUI execution failed with status %q", entry.Status.StatusString)
			}
		}
		select {
		case <-ctx.Done():
			return comfyUIHistoryEntry{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (adapter *ComfyUIAdapter) resultFromHistory(ctx context.Context, profile config.ResolvedImageAPIProfile, request GenerateRequest, entry comfyUIHistoryEntry) (Result, error) {
	result := Result{
		ProfileID:    profile.ProfileID,
		Provider:     profile.Provider,
		Model:        profile.Model,
		Created:      time.Now().Unix(),
		Size:         request.Size,
		Quality:      request.Quality,
		OutputFormat: request.OutputFormat,
	}
	nodeIDs := make([]string, 0, len(entry.Outputs))
	for nodeID := range entry.Outputs {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Strings(nodeIDs)
	for _, nodeID := range nodeIDs {
		for _, ref := range entry.Outputs[nodeID].Images {
			image, err := adapter.downloadImage(ctx, profile, ref)
			if err != nil {
				result.Failures = append(result.Failures, Failure{Index: len(result.Images), Code: "invalid_image", Message: err.Error()})
				continue
			}
			if result.OutputFormat == "" {
				result.OutputFormat = image.Extension
			}
			result.Images = append(result.Images, image)
		}
	}
	if len(result.Images) == 0 {
		if len(result.Failures) > 0 {
			return Result{}, fmt.Errorf("%w: %s", ErrImageDataMissing, result.Failures[0].Message)
		}
		return Result{}, ErrImageDataMissing
	}
	return result, nil
}

func (adapter *ComfyUIAdapter) downloadImage(ctx context.Context, profile config.ResolvedImageAPIProfile, ref comfyUIImageRef) (Image, error) {
	viewEndpoint, err := endpointURL(profile.BaseURL, "view")
	if err != nil {
		return Image{}, err
	}
	parsed, err := url.Parse(viewEndpoint)
	if err != nil {
		return Image{}, err
	}
	query := parsed.Query()
	query.Set("filename", ref.Filename)
	query.Set("subfolder", ref.Subfolder)
	query.Set("type", ref.Type)
	parsed.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return Image{}, fmt.Errorf("create ComfyUI image request: %w", err)
	}
	for key, value := range bearerHeaders(profile.APIKey, profile.Headers) {
		req.Header.Set(key, value)
	}
	response, err := adapter.httpClient.Do(req)
	if err != nil {
		return Image{}, fmt.Errorf("download ComfyUI image: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		payload, _ := readLimited(response.Body, 64<<10)
		return Image{}, imageAPIStatusError(response.StatusCode, payload)
	}
	data, err := readLimited(response.Body, 64<<20)
	if err != nil {
		return Image{}, fmt.Errorf("read ComfyUI image: %w", err)
	}
	format, mimeType, err := inferImageFormat(data, response.Header.Get("Content-Type"), ref.Filename)
	if err != nil {
		return Image{}, err
	}
	return Image{Data: data, MIMEType: mimeType, Extension: extensionForFormat(format), SourceURL: parsed.String()}, nil
}
