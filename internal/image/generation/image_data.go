package generation

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
)

var ErrImageDataMissing = errors.New("image provider returned no image data")

type imagesAPIResponse struct {
	Created      int64           `json:"created"`
	Model        string          `json:"model"`
	Size         string          `json:"size"`
	Quality      string          `json:"quality"`
	OutputFormat string          `json:"output_format"`
	Data         []imagesAPIItem `json:"data"`
}

type imagesAPIItem struct {
	B64JSON       string `json:"b64_json"`
	URL           string `json:"url"`
	RevisedPrompt string `json:"revised_prompt"`
	Size          string `json:"size"`
}

func imagesResultFromResponse(ctx context.Context, client *http.Client, profileID, provider, model string, request GenerateRequest, response imagesAPIResponse) (Result, error) {
	result := Result{
		ProfileID:    profileID,
		Provider:     provider,
		Model:        firstNonEmptyString(response.Model, model),
		Created:      response.Created,
		Size:         firstNonEmptyString(response.Size, request.Size),
		Quality:      firstNonEmptyString(response.Quality, request.Quality),
		OutputFormat: firstNonEmptyString(response.OutputFormat, request.OutputFormat),
	}
	for index, item := range response.Data {
		image, err := imageFromAPIItem(ctx, client, item, response.OutputFormat, request.OutputFormat)
		if err != nil {
			result.Failures = append(result.Failures, Failure{Index: index, Code: "invalid_image", Message: err.Error()})
			continue
		}
		if result.Size == "" {
			result.Size = item.Size
		}
		if result.OutputFormat == "" {
			result.OutputFormat = image.Extension
		}
		result.Images = append(result.Images, image)
	}
	if len(result.Images) == 0 {
		if len(result.Failures) > 0 {
			return Result{}, fmt.Errorf("%w: %s", ErrImageDataMissing, result.Failures[0].Message)
		}
		return Result{}, ErrImageDataMissing
	}
	return result, nil
}

func imageFromAPIItem(ctx context.Context, client *http.Client, item imagesAPIItem, formatCandidates ...string) (Image, error) {
	if item.B64JSON != "" {
		encoded := item.B64JSON
		if _, tail, ok := strings.Cut(encoded, ","); ok && strings.HasPrefix(encoded, "data:") {
			encoded = tail
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return Image{}, fmt.Errorf("decode image base64: %w", err)
		}
		format, mimeType, err := inferImageFormat(data, "", formatCandidates...)
		if err != nil {
			return Image{}, err
		}
		return Image{Data: data, MIMEType: mimeType, Extension: extensionForFormat(format), RevisedPrompt: item.RevisedPrompt}, nil
	}
	if item.URL == "" {
		return Image{}, ErrImageDataMissing
	}
	data, contentType, err := downloadImageURL(ctx, client, item.URL)
	if err != nil {
		return Image{}, err
	}
	formatCandidates = append(formatCandidates, imageFormatFromURL(item.URL))
	format, mimeType, err := inferImageFormat(data, contentType, formatCandidates...)
	if err != nil {
		return Image{}, err
	}
	return Image{Data: data, MIMEType: mimeType, Extension: extensionForFormat(format), RevisedPrompt: item.RevisedPrompt, SourceURL: item.URL}, nil
}

func downloadImageURL(ctx context.Context, client *http.Client, target string) ([]byte, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", fmt.Errorf("download image: HTTP %d", response.StatusCode)
	}
	data, err := readLimited(response.Body, 64<<20)
	if err != nil {
		return nil, "", err
	}
	return data, response.Header.Get("Content-Type"), nil
}

func inferImageFormat(data []byte, contentType string, candidates ...string) (string, string, error) {
	if format := imageFormatFromBytes(data); format != "" {
		return format, mimeTypeForFormat(format), nil
	}
	if format := imageFormatFromContentType(contentType); format != "" {
		return format, mimeTypeForFormat(format), nil
	}
	for _, candidate := range candidates {
		if format := normalizeImageFormat(candidate); format != "" {
			return format, mimeTypeForFormat(format), nil
		}
	}
	return "", "", errors.New("cannot identify generated image format")
}

func imageFormatFromContentType(contentType string) string {
	if contentType == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.ToLower(contentType))
	}
	switch mediaType {
	case "image/png":
		return "png"
	case "image/jpeg", "image/jpg":
		return "jpeg"
	case "image/webp":
		return "webp"
	default:
		return ""
	}
}

func imageFormatFromBytes(data []byte) string {
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "webp"
	}
	return imageFormatFromContentType(http.DetectContentType(data))
}

func imageFormatFromURL(target string) string {
	parsed, err := url.Parse(target)
	if err != nil {
		return ""
	}
	return normalizeImageFormat(strings.TrimPrefix(strings.ToLower(path.Ext(parsed.Path)), "."))
}

func normalizeImageFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "png":
		return "png"
	case "jpg", "jpeg":
		return "jpeg"
	case "webp":
		return "webp"
	default:
		return ""
	}
}

func mimeTypeForFormat(format string) string {
	switch normalizeImageFormat(format) {
	case "png":
		return "image/png"
	case "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return ""
	}
}

func extensionForFormat(format string) string { return normalizeImageFormat(format) }

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
