package generation

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"denova/config"
)

var xaiAspectRatios = []string{"1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3", "2:1", "1:2", "19.5:9", "9:19.5", "20:9", "9:20"}

type XAIAdapter struct {
	httpClient *http.Client
}

func NewXAIAdapter(httpClient *http.Client) *XAIAdapter {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &XAIAdapter{httpClient: httpClient}
}

type xaiImageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	AspectRatio    string `json:"aspect_ratio,omitempty"`
	Resolution     string `json:"resolution,omitempty"`
	Quality        string `json:"quality,omitempty"`
	ResponseFormat string `json:"response_format"`
}

func (adapter *XAIAdapter) Generate(ctx context.Context, profile config.ResolvedImageAPIProfile, request GenerateRequest) (Result, error) {
	endpoint, err := endpointURL(profile.BaseURL, "images/generations")
	if err != nil {
		return Result{}, err
	}
	resolution := strings.ToLower(strings.TrimSpace(request.Resolution))
	if resolution != "" && resolution != "1k" && resolution != "2k" {
		return Result{}, fmt.Errorf("xAI image resolution must be 1k or 2k")
	}
	quality := strings.ToLower(strings.TrimSpace(request.Quality))
	if quality != "" && quality != "low" && quality != "medium" {
		return Result{}, fmt.Errorf("xAI image quality must be low or medium")
	}
	payload := xaiImageRequest{
		Model:          profile.Model,
		Prompt:         request.Prompt,
		N:              request.N,
		AspectRatio:    closestAspectRatio(request.Size, request.AspectRatio, xaiAspectRatios),
		Resolution:     resolution,
		Quality:        quality,
		ResponseFormat: "b64_json",
	}
	var response imagesAPIResponse
	if err := doJSON(ctx, adapter.httpClient, http.MethodPost, endpoint, bearerHeaders(profile.APIKey, profile.Headers), payload, &response); err != nil {
		return Result{}, err
	}
	request.Size = ""
	request.OutputFormat = ""
	return imagesResultFromResponse(ctx, adapter.httpClient, profile.ProfileID, profile.Provider, profile.Model, request, response)
}
