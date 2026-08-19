package generation

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"denova/config"
)

var geminiAspectRatios = []string{"1:1", "1:4", "1:8", "2:3", "3:2", "3:4", "4:1", "4:3", "4:5", "5:4", "8:1", "9:16", "16:9", "21:9"}

type GeminiAdapter struct {
	httpClient *http.Client
}

func NewGeminiAdapter(httpClient *http.Client) *GeminiAdapter {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &GeminiAdapter{httpClient: httpClient}
}

type geminiImageRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inlineData,omitempty"`
}

type geminiInlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiGenerationConfig struct {
	ResponseModalities []string             `json:"responseModalities"`
	ResponseFormat     geminiResponseFormat `json:"responseFormat,omitempty"`
}

type geminiResponseFormat struct {
	Image geminiImageConfig `json:"image"`
}

type geminiImageConfig struct {
	AspectRatio string `json:"aspectRatio,omitempty"`
	ImageSize   string `json:"imageSize,omitempty"`
}

type geminiImageResponse struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
}

func (adapter *GeminiAdapter) Generate(ctx context.Context, profile config.ResolvedImageAPIProfile, request GenerateRequest) (Result, error) {
	model := strings.TrimPrefix(strings.TrimSpace(profile.Model), "models/")
	endpoint, err := endpointURL(profile.BaseURL, "models/"+model+":generateContent")
	if err != nil {
		return Result{}, err
	}
	headers := map[string]string{"x-goog-api-key": profile.APIKey}
	for key, value := range profile.Headers {
		headers[key] = value
	}
	result := Result{
		ProfileID: profile.ProfileID,
		Provider:  profile.Provider,
		Model:     profile.Model,
		Created:   time.Now().Unix(),
		Size:      request.Size,
	}
	for index := 0; index < request.N; index++ {
		payload := geminiImageRequest{
			Contents: []geminiContent{{Role: "user", Parts: []geminiPart{{Text: request.Prompt}}}},
			GenerationConfig: geminiGenerationConfig{
				ResponseModalities: []string{"IMAGE"},
				ResponseFormat: geminiResponseFormat{Image: geminiImageConfig{
					AspectRatio: closestAspectRatio(request.Size, request.AspectRatio, geminiAspectRatios),
					ImageSize:   strings.ToUpper(strings.TrimSpace(request.Resolution)),
				}},
			},
		}
		var response geminiImageResponse
		if err := doJSON(ctx, adapter.httpClient, http.MethodPost, endpoint, headers, payload, &response); err != nil {
			result.Failures = append(result.Failures, Failure{Index: index, Code: "provider_request_failed", Message: err.Error()})
			continue
		}
		before := len(result.Images)
		for _, candidate := range response.Candidates {
			for _, part := range candidate.Content.Parts {
				if part.InlineData == nil || part.InlineData.Data == "" {
					continue
				}
				image, err := imageFromAPIItem(ctx, adapter.httpClient, imagesAPIItem{B64JSON: part.InlineData.Data}, part.InlineData.MIMEType)
				if err != nil {
					result.Failures = append(result.Failures, Failure{Index: index, Code: "invalid_image", Message: err.Error()})
					continue
				}
				result.Images = append(result.Images, image)
				if result.OutputFormat == "" {
					result.OutputFormat = image.Extension
				}
			}
		}
		if len(result.Images) == before {
			result.Failures = append(result.Failures, Failure{Index: index, Code: "empty_image", Message: "Gemini returned no image content"})
		}
	}
	if len(result.Images) == 0 {
		if len(result.Failures) > 0 {
			return Result{}, fmt.Errorf("Gemini image generation failed: %s", result.Failures[0].Message)
		}
		return Result{}, ErrImageDataMissing
	}
	return result, nil
}
