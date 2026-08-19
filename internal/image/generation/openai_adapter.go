package generation

import (
	"context"
	"net/http"
	"strings"

	"denova/config"
)

type OpenAIAdapter struct {
	httpClient *http.Client
}

func NewOpenAIAdapter(httpClient *http.Client) *OpenAIAdapter {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &OpenAIAdapter{httpClient: httpClient}
}

type openAIImageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	OutputFormat   string `json:"output_format,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

func (adapter *OpenAIAdapter) Generate(ctx context.Context, profile config.ResolvedImageAPIProfile, request GenerateRequest) (Result, error) {
	endpoint, err := endpointURL(profile.BaseURL, "images/generations")
	if err != nil {
		return Result{}, err
	}
	size := request.Size
	if profile.Provider == config.ImageProviderOpenAI {
		size = openAIImageSize(size, request.AspectRatio)
	}
	payload := openAIImageRequest{
		Model:        profile.Model,
		Prompt:       request.Prompt,
		N:            request.N,
		Size:         size,
		Quality:      request.Quality,
		OutputFormat: request.OutputFormat,
	}
	if strings.HasPrefix(strings.ToLower(profile.Model), "dall-e-") {
		payload.OutputFormat = ""
		payload.ResponseFormat = "b64_json"
		request.OutputFormat = ""
	}
	var response imagesAPIResponse
	if err := doJSON(ctx, adapter.httpClient, http.MethodPost, endpoint, bearerHeaders(profile.APIKey, profile.Headers), payload, &response); err != nil {
		return Result{}, err
	}
	request.Size = size
	return imagesResultFromResponse(ctx, adapter.httpClient, profile.ProfileID, profile.Provider, profile.Model, request, response)
}

func openAIImageSize(size, aspectRatio string) string {
	size = strings.TrimSpace(size)
	switch size {
	case "", "1024x1024", "1536x1024", "1024x1536":
		return size
	}
	ratio := aspectRatioValue(aspectRatio)
	if width, height, ok := parseImageDimensions(size); ok {
		ratio = float64(width) / float64(height)
	}
	switch {
	case ratio > 1.15:
		return "1536x1024"
	case ratio > 0 && ratio < 0.87:
		return "1024x1536"
	default:
		return "1024x1024"
	}
}
