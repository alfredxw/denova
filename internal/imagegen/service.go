package imagegen

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"denova/config"
)

var (
	ErrPromptRequired       = errors.New("图像提示词不能为空")
	ErrUnsupportedProvider  = errors.New("不支持的图像模型 provider")
	ErrImageCountOutOfRange = errors.New("图像数量必须在 1 到 10 之间")
)

// supportedImageSizes 是 OpenAI 兼容 provider 共享的尺寸白名单。
// MiniMax 走 aspect_ratio 协议，使用单独的 supportedMinimaxImageSizes。
var supportedImageSizes = map[string]struct{}{
	"2048x2048": {}, "2304x1728": {}, "1728x2304": {}, "2848x1600": {}, "1600x2848": {}, "2496x1664": {}, "1664x2496": {}, "3136x1344": {},
	"3072x3072": {}, "3456x2592": {}, "2592x3456": {}, "4096x2304": {}, "2304x4096": {}, "2496x3744": {}, "3744x2496": {}, "4704x2016": {},
	"4096x4096": {}, "3520x4704": {}, "4704x3520": {}, "5504x3040": {}, "3040x5504": {}, "3328x4992": {}, "4992x3328": {}, "6240x2656": {},
}

// supportedMinimaxImageSizes 是 MiniMax 图像生成支持的尺寸集合。
// MiniMax 不接受直接尺寸字符串，但用户在 Denova 的设置里仍可配置这些值，
// 适配器会按比例（size→aspect_ratio）转换为 MiniMax 协议。
var supportedMinimaxImageSizes = map[string]struct{}{
	"1024x1024": {}, "1280x720": {}, "1920x1080": {},
	"720x1280": {}, "1080x1920": {},
	"1152x864": {}, "864x1152": {},
	"1248x832": {}, "832x1248": {},
	"2048x2048": {}, // 也会落到 1:1，等价于 1024x1024
}

type Service struct {
	adapters map[string]Adapter
}

func NewService() *Service {
	return &Service{adapters: map[string]Adapter{
		config.DefaultImageAPIProvider: NewOpenAIAdapter(nil),
		"minimax":                      NewMiniMaxAdapter(nil), // MiniMax 原生支持
	}}
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
	request, err = normalizeRequestOptionsForProvider(profile.Provider, request)
	if err != nil {
		return Result{}, err
	}

	adapter := s.adapters[profile.Provider]
	if adapter == nil {
		return Result{}, fmt.Errorf("%w: %s", ErrUnsupportedProvider, profile.Provider)
	}
	log.Printf("[imagegen] generate begin provider=%s profile_id=%s model=%q size=%q quality=%q format=%q n=%d prompt=%s", profile.Provider, profile.ProfileID, profile.OpenAIModel, request.Size, request.Quality, request.OutputFormat, request.N, promptSummary(request.Prompt))
	result, err := adapter.Generate(ctx, profile, request)
	if err != nil {
		log.Printf("[imagegen] generate failed provider=%s profile_id=%s model=%q err=%v", profile.Provider, profile.ProfileID, profile.OpenAIModel, err)
		return Result{}, err
	}
	log.Printf("[imagegen] generate done provider=%s profile_id=%s model=%q images=%d", profile.Provider, profile.ProfileID, profile.OpenAIModel, len(result.Images))
	return result, nil
}

func normalizeRequestOptions(request GenerateRequest) (GenerateRequest, error) {
	return normalizeRequestOptionsForProvider("", request)
}

func normalizeRequestOptionsForProvider(provider string, request GenerateRequest) (GenerateRequest, error) {
	if request.Size != "" {
		size, ok := NormalizeSizeForProvider(provider, request.Size)
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

// NormalizeSizeForProvider 校验某个尺寸是否被指定 provider 接受。
// 不同 provider 支持的尺寸集合不同：OpenAI 走 2K/3K/4K，MiniMax 走 1024 等。
func NormalizeSizeForProvider(provider, size string) (string, bool) {
	trimmed := strings.TrimSpace(size)
	if trimmed == "" || trimmed == "auto" {
		return "", true
	}
	set := supportedImageSizes
	if strings.EqualFold(strings.TrimSpace(provider), "minimax") {
		set = supportedMinimaxImageSizes
	}
	if _, ok := set[trimmed]; ok {
		return trimmed, true
	}
	return "", false
}

func promptSummary(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	sum := sha256.Sum256([]byte(prompt))
	preview := prompt
	const limit = 160
	if len(preview) > limit {
		preview, _ = truncateUTF8Bytes(preview, limit)
	}
	return fmt.Sprintf("chars=%d bytes=%d hash=sha256:%x preview=%q", utf8.RuneCountInString(prompt), len(prompt), sum[:8], preview)
}

func truncateUTF8Bytes(value string, limit int) (string, bool) {
	if limit <= 0 || len(value) <= limit {
		return value, false
	}
	out := value[:limit]
	for !utf8.ValidString(out) && len(out) > 0 {
		out = out[:len(out)-1]
	}
	return out + "...", true
}

func normalizeSize(size string) (string, bool) {
	return NormalizeSizeForProvider("", size)
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
