package imagegen

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"nova/config"
)

var (
	ErrPromptRequired       = errors.New("图像提示词不能为空")
	ErrUnsupportedProvider  = errors.New("不支持的图像模型 provider")
	ErrImageCountOutOfRange = errors.New("图像数量必须在 1 到 10 之间")
)

var supportedImageSizes = map[string]struct{}{
	"2048x2048": {}, "2304x1728": {}, "1728x2304": {}, "2848x1600": {}, "1600x2848": {}, "2496x1664": {}, "1664x2496": {}, "3136x1344": {},
	"3072x3072": {}, "3456x2592": {}, "2592x3456": {}, "4096x2304": {}, "2304x4096": {}, "2496x3744": {}, "3744x2496": {}, "4704x2016": {},
	"4096x4096": {}, "3520x4704": {}, "4704x3520": {}, "5504x3040": {}, "3040x5504": {}, "3328x4992": {}, "4992x3328": {}, "6240x2656": {},
}

type Service struct {
	adapters map[string]Adapter
}

func NewService() *Service {
	return &Service{adapters: map[string]Adapter{
		config.DefaultImageAPIProvider: NewOpenAIAdapter(nil),
	}}
}

func NewServiceWithAdapters(adapters map[string]Adapter) *Service {
	out := make(map[string]Adapter, len(adapters))
	for key, adapter := range adapters {
		out[strings.TrimSpace(key)] = adapter
	}
	return &Service{adapters: out}
}

func (s *Service) Generate(ctx context.Context, cfg *config.Config, request GenerateRequest) (Result, error) {
	if strings.TrimSpace(request.Prompt) == "" {
		return Result{}, ErrPromptRequired
	}
	profile, err := config.ResolveImageAPIProfile(cfg, request.ProfileID)
	if err != nil {
		return Result{}, err
	}
	request.Prompt = strings.TrimSpace(request.Prompt)
	if request.N == 0 {
		request.N = 1
	}
	if request.N < 1 || request.N > 10 {
		return Result{}, ErrImageCountOutOfRange
	}
	if request.Quality == "" {
		request.Quality = profile.Quality
	}
	if request.OutputFormat == "" {
		request.OutputFormat = profile.OutputFormat
	}
	request, err = normalizeRequestOptions(request)
	if err != nil {
		return Result{}, err
	}

	adapter := s.adapters[profile.Provider]
	if adapter == nil {
		return Result{}, fmt.Errorf("%w: %s", ErrUnsupportedProvider, profile.Provider)
	}
	log.Printf("[imagegen] generate begin provider=%s profile_id=%s model=%q size=%q quality=%q format=%q n=%d prompt_chars=%d, prompt: %s", profile.Provider, profile.ProfileID, profile.OpenAIModel, request.Size, request.Quality, request.OutputFormat, request.N, len([]rune(request.Prompt)), request.Prompt)
	result, err := adapter.Generate(ctx, profile, request)
	if err != nil {
		log.Printf("[imagegen] generate failed provider=%s profile_id=%s model=%q err=%v", profile.Provider, profile.ProfileID, profile.OpenAIModel, err)
		return Result{}, err
	}
	log.Printf("[imagegen] generate done provider=%s profile_id=%s model=%q images=%d", profile.Provider, profile.ProfileID, profile.OpenAIModel, len(result.Images))
	return result, nil
}

func normalizeRequestOptions(request GenerateRequest) (GenerateRequest, error) {
	if request.Size != "" {
		size, ok := normalizeSize(request.Size)
		if !ok {
			return GenerateRequest{}, fmt.Errorf("不支持的图像尺寸: %s", request.Size)
		}
		request.Size = size
	}
	if request.Quality != "" {
		quality := normalizeQuality(request.Quality)
		if quality == "" {
			return GenerateRequest{}, fmt.Errorf("不支持的图像质量: %s", request.Quality)
		}
		request.Quality = quality
	}
	if request.OutputFormat != "" {
		format := normalizeOutputFormat(request.OutputFormat)
		if format == "" {
			return GenerateRequest{}, fmt.Errorf("不支持的图像格式: %s", request.OutputFormat)
		}
		request.OutputFormat = format
	}
	return request, nil
}

func normalizeSize(size string) (string, bool) {
	trimmed := strings.TrimSpace(size)
	if trimmed == "" || trimmed == "auto" {
		return "", true
	}
	if _, ok := supportedImageSizes[trimmed]; ok {
		return trimmed, true
	}
	return "", false
}

func normalizeQuality(quality string) string {
	switch strings.TrimSpace(quality) {
	case "auto", "standard", "hd", "low", "medium", "high":
		return strings.TrimSpace(quality)
	default:
		return ""
	}
}

func normalizeOutputFormat(format string) string {
	switch strings.TrimSpace(format) {
	case "png", "jpeg":
		return strings.TrimSpace(format)
	default:
		return ""
	}
}
