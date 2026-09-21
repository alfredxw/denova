package generation

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"denova/config"
)

// The Agnes Image API uses an OpenAI-images-shaped endpoint but rejects
// OpenAI-only fields (output_format, quality, top-level response_format).
// Sizes are tier names (1K/2K/3K/4K) paired with a ratio; exact WxH sizes are
// accepted but take no ratio. Base64 output is requested via return_base64.
var agnesAspectRatios = []string{"1:1", "3:4", "4:3", "16:9", "9:16", "2:3", "3:2", "21:9"}

type AgnesAdapter struct {
	httpClient *http.Client
}

func NewAgnesAdapter(httpClient *http.Client) *AgnesAdapter {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &AgnesAdapter{httpClient: httpClient}
}

type agnesImageRequest struct {
	Model        string `json:"model"`
	Prompt       string `json:"prompt"`
	Size         string `json:"size"`
	Ratio        string `json:"ratio,omitempty"`
	ReturnBase64 bool   `json:"return_base64,omitempty"`
}

func (adapter *AgnesAdapter) Generate(ctx context.Context, profile config.ResolvedImageAPIProfile, request GenerateRequest) (Result, error) {
	endpoint, err := endpointURL(profile.BaseURL, "images/generations")
	if err != nil {
		return Result{}, err
	}
	combined := imagesAPIResponse{Model: profile.Model}
	providerFailures := make([]Failure, 0)
	for index := 0; index < request.N; index++ {
		size := strings.TrimSpace(request.Size)
		if size == "" {
			size = "2K"
		}
		payload := agnesImageRequest{
			Model:        profile.Model,
			Prompt:       request.Prompt,
			Size:         size,
			ReturnBase64: true,
		}
		// Ratio only pairs with tier sizes; exact WxH sizes encode it already.
		if !strings.ContainsAny(size, "xX") {
			payload.Ratio = agnesAspectRatio(request.AspectRatio)
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
			return Result{}, fmt.Errorf("Agnes image generation failed: %s", providerFailures[0].Message)
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

// agnesAspectRatio passes through only ratios documented by the provider;
// anything else is dropped so one preference cannot fail the whole request.
func agnesAspectRatio(ratio string) string {
	ratio = strings.ToLower(strings.TrimSpace(ratio))
	for _, supported := range agnesAspectRatios {
		if ratio == supported {
			return ratio
		}
	}
	return ""
}
