package generation

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"denova/config"
)

type ArkAdapter struct {
	httpClient *http.Client
}

func NewArkAdapter(httpClient *http.Client) *ArkAdapter {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &ArkAdapter{httpClient: httpClient}
}

type arkImageRequest struct {
	Model                     string `json:"model"`
	Prompt                    string `json:"prompt"`
	Size                      string `json:"size,omitempty"`
	OutputFormat              string `json:"output_format,omitempty"`
	ResponseFormat            string `json:"response_format"`
	SequentialImageGeneration string `json:"sequential_image_generation"`
	Stream                    bool   `json:"stream"`
	Watermark                 bool   `json:"watermark"`
}

func (adapter *ArkAdapter) Generate(ctx context.Context, profile config.ResolvedImageAPIProfile, request GenerateRequest) (Result, error) {
	endpoint, err := endpointURL(profile.BaseURL, "images/generations")
	if err != nil {
		return Result{}, err
	}
	combined := imagesAPIResponse{Model: profile.Model, OutputFormat: request.OutputFormat}
	providerFailures := make([]Failure, 0)
	for index := 0; index < request.N; index++ {
		size := strings.TrimSpace(request.Size)
		if size == "" {
			size = strings.TrimSpace(request.Resolution)
		}
		payload := arkImageRequest{
			Model:                     profile.Model,
			Prompt:                    request.Prompt,
			Size:                      size,
			OutputFormat:              request.OutputFormat,
			ResponseFormat:            "url",
			SequentialImageGeneration: "disabled",
			Stream:                    false,
			Watermark:                 false,
		}
		var response imagesAPIResponse
		if err := doJSON(ctx, adapter.httpClient, http.MethodPost, endpoint, bearerHeaders(profile.APIKey, profile.Headers), payload, &response); err != nil {
			providerFailures = append(providerFailures, Failure{Index: index, Code: "provider_request_failed", Message: err.Error()})
			continue
		}
		if combined.Created == 0 {
			combined.Created = response.Created
		}
		combined.Data = append(combined.Data, response.Data...)
	}
	if len(combined.Data) == 0 {
		if len(providerFailures) > 0 {
			return Result{}, fmt.Errorf("Seedream generation failed: %s", providerFailures[0].Message)
		}
		return Result{}, ErrImageDataMissing
	}
	request.N = len(combined.Data)
	result, err := imagesResultFromResponse(ctx, adapter.httpClient, profile.ProfileID, profile.Provider, profile.Model, request, combined)
	if err != nil {
		return Result{}, err
	}
	result.Failures = append(result.Failures, providerFailures...)
	return result, nil
}
