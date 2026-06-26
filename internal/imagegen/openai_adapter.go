package imagegen

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"nova/config"
)

var ErrImageDataMissing = errors.New("图片 API 未返回图片数据")

type OpenAIAdapter struct {
	httpClient *http.Client
}

func NewOpenAIAdapter(httpClient *http.Client) *OpenAIAdapter {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &OpenAIAdapter{httpClient: httpClient}
}

func (a *OpenAIAdapter) Generate(ctx context.Context, profile config.ResolvedImageAPIProfile, request GenerateRequest) (Result, error) {
	opts := []option.RequestOption{
		option.WithAPIKey(profile.OpenAIAPIKey),
		option.WithBaseURL(profile.OpenAIBaseURL),
		option.WithHTTPClient(a.httpClient),
	}
	client := openai.NewClient(opts...)
	params := openai.ImageGenerateParams{
		Prompt: request.Prompt,
		Model:  openai.ImageModel(profile.OpenAIModel),
		N:      openai.Int(int64(request.N)),
	}
	if request.Size != "" {
		params.Size = openai.ImageGenerateParamsSize(request.Size)
	}
	if request.Quality != "" {
		params.Quality = openai.ImageGenerateParamsQuality(request.Quality)
	}
	if request.OutputFormat != "" && !isDallEModel(profile.OpenAIModel) {
		params.OutputFormat = openai.ImageGenerateParamsOutputFormat(request.OutputFormat)
	}
	if isDallEModel(profile.OpenAIModel) {
		params.ResponseFormat = openai.ImageGenerateParamsResponseFormatB64JSON
	}

	response, err := client.Images.Generate(ctx, params)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		ProfileID:    profile.ProfileID,
		Provider:     profile.Provider,
		Model:        profile.OpenAIModel,
		Created:      response.Created,
		Size:         string(response.Size),
		Quality:      string(response.Quality),
		OutputFormat: string(response.OutputFormat),
	}
	for index, item := range response.Data {
		image, err := a.openAIImageToBytes(ctx, item, response, request, index)
		if err != nil {
			return Result{}, err
		}
		result.Images = append(result.Images, image)
	}
	if len(result.Images) == 0 {
		return Result{}, ErrImageDataMissing
	}
	return result, nil
}

func (a *OpenAIAdapter) openAIImageToBytes(ctx context.Context, item openai.Image, response *openai.ImagesResponse, request GenerateRequest, index int) (Image, error) {
	if item.B64JSON != "" {
		data, err := base64.StdEncoding.DecodeString(item.B64JSON)
		if err != nil {
			return Image{}, fmt.Errorf("解析第 %d 张图片 base64 失败: %w", index+1, err)
		}
		format, mimeType, err := inferImageFormat(data, "", string(response.OutputFormat), request.OutputFormat)
		if err != nil {
			return Image{}, err
		}
		return Image{
			Data:          data,
			MIMEType:      mimeType,
			Extension:     extensionForFormat(format),
			RevisedPrompt: item.RevisedPrompt,
		}, nil
	}
	if item.URL != "" {
		data, contentType, err := a.downloadImageURL(ctx, item.URL)
		if err != nil {
			return Image{}, err
		}
		format, mimeType, err := inferImageFormat(data, contentType, string(response.OutputFormat), request.OutputFormat, imageFormatFromURL(item.URL))
		if err != nil {
			return Image{}, err
		}
		return Image{
			Data:          data,
			MIMEType:      mimeType,
			Extension:     extensionForFormat(format),
			RevisedPrompt: item.RevisedPrompt,
			SourceURL:      item.URL,
		}, nil
	}
	return Image{}, ErrImageDataMissing
}

func (a *OpenAIAdapter) downloadImageURL(ctx context.Context, target string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("下载图片失败: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func inferImageFormat(data []byte, contentType string, candidates ...string) (string, string, error) {
	for _, candidate := range candidates {
		if format := normalizeImageFormat(candidate); format != "" {
			return format, mimeTypeForFormat(format), nil
		}
	}
	if format := imageFormatFromContentType(contentType); format != "" {
		return format, mimeTypeForFormat(format), nil
	}
	if format := imageFormatFromBytes(data); format != "" {
		return format, mimeTypeForFormat(format), nil
	}
	return "", "", errors.New("无法识别图片格式")
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
	contentType := http.DetectContentType(data)
	if format := imageFormatFromContentType(contentType); format != "" {
		return format
	}
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "webp"
	}
	return ""
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

func extensionForFormat(format string) string {
	switch normalizeImageFormat(format) {
	case "png":
		return "png"
	case "jpeg":
		return "jpg"
	case "webp":
		return "webp"
	default:
		return ""
	}
}

func isDallEModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "dall-e-")
}
