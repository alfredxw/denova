package imagegen

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"denova/config"
	"denova/internal/imagegen/minimax"
)

var (
	ErrMiniMaxPromptRequired  = errors.New("MiniMax 提示词不能为空")
	ErrMiniMaxResponseInvalid = errors.New("MiniMax 响应格式无效")
	ErrMiniMaxAPIKeyMissing   = errors.New("MiniMax API Key 未配置")
)

// MiniMaxAdapter 将 Denova 的图像生成请求适配到 MiniMax 原生 image_generation 接口。
// HTTP 协议通信由 minimax.Client 承接，本适配器只负责：
//  1. GenerateRequest ↔ minimax.Request 的格式转换（size→aspect_ratio 等）
//  2. minimax.Response → Result 的图像提取
type MiniMaxAdapter struct {
	httpClient *http.Client
}

func NewMiniMaxAdapter(httpClient *http.Client) *MiniMaxAdapter {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 120 * time.Second}
	}
	return &MiniMaxAdapter{httpClient: httpClient}
}

func (a *MiniMaxAdapter) Generate(ctx context.Context, profile config.ResolvedImageAPIProfile, request GenerateRequest) (Result, error) {
	if strings.TrimSpace(request.Prompt) == "" {
		return Result{}, ErrMiniMaxPromptRequired
	}
	if strings.TrimSpace(profile.OpenAIAPIKey) == "" {
		return Result{}, ErrMiniMaxAPIKeyMissing
	}

	// 组装 MiniMax 请求（Denova 格式 → MiniMax 格式）
	req := minimax.Request{
		Prompt:         strings.TrimSpace(request.Prompt),
		Model:          strings.TrimSpace(profile.OpenAIModel),
		AspectRatio:    sizeToAspectRatio(request.Size),
		ResponseFormat: "base64", // 明确要求 base64，避免返回 24 小时过期的 url
	}

	log.Printf("[imagegen] minimax generate begin model=%s aspect_ratio=%s prompt=%s",
		req.Model, req.AspectRatio, promptSummary(request.Prompt))

	// 调用 MiniMax API（每个 profile 的 baseURL 可能不同，按需创建 client）
	client := minimax.NewClient(a.httpClient, profile.OpenAIBaseURL)
	resp, err := client.Generate(ctx, profile.OpenAIAPIKey, req)
	if err != nil {
		log.Printf("[imagegen] minimax generate failed model=%s err=%v", req.Model, err)
		return Result{}, err
	}

	// 提取图像（MiniMax 响应 → Denova Result）
	images := a.extractImages(ctx, resp)
	if len(images) == 0 {
		log.Printf("[imagegen] minimax 未返回图像数据 model=%s status_code=%d status_msg=%q has_base64=%v has_url=%v",
			req.Model, resp.BaseResp.StatusCode, resp.BaseResp.StatusMsg,
			len(resp.Data.ImageBase64) > 0, len(resp.Data.ImageURL) > 0)
		if resp.BaseResp.StatusMsg != "" {
			return Result{}, fmt.Errorf("MiniMax 未返回图像: %s", resp.BaseResp.StatusMsg)
		}
		return Result{}, ErrMiniMaxResponseInvalid
	}

	log.Printf("[imagegen] minimax generate done provider=%s model=%s images=%d",
		profile.Provider, req.Model, len(images))

	return Result{
		ProfileID:    profile.ProfileID,
		Provider:     profile.Provider,
		Model:        req.Model,
		OutputFormat: inferOutputFormat(images, request.OutputFormat),
		Images:       images,
	}, nil
}

// inferOutputFormat 从图像数据中推断输出格式，未能识别时回退到 request.OutputFormat，再回退到 png。
func inferOutputFormat(images []Image, fallback string) string {
	if len(images) > 0 {
		switch strings.ToLower(strings.TrimSpace(images[0].Extension)) {
		case "png", "jpeg":
			return strings.ToLower(strings.TrimSpace(images[0].Extension))
		}
	}
	if format := normalizeOutputFormat(fallback); format != "" {
		return format
	}
	return "png"
}

// extractImages 从 MiniMax 响应中提取图像，优先使用 base64，回退到 URL 下载。
func (a *MiniMaxAdapter) extractImages(ctx context.Context, resp *minimax.Response) []Image {
	var images []Image
	for _, b64 := range resp.Data.ImageBase64 {
		if strings.TrimSpace(b64) == "" {
			continue
		}
		if image, err := decodeBase64Image(b64); err == nil {
			images = append(images, image)
		} else {
			log.Printf("[imagegen] minimax 解码 base64 失败: %v", err)
		}
	}
	if len(images) == 0 { // 没有 base64 时回退到 URL
		for _, imageURL := range resp.Data.ImageURL {
			if strings.TrimSpace(imageURL) == "" {
				continue
			}
			if image, err := a.fetchImageURL(ctx, imageURL); err == nil {
				images = append(images, image)
			} else {
				log.Printf("[imagegen] minimax 下载 URL 失败: %v", err)
			}
		}
	}
	return images
}

// fetchImageURL 下载图像 URL，复用 imageFormatFromBytes 检测格式。
func (a *MiniMaxAdapter) fetchImageURL(ctx context.Context, imageURL string) (Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return Image{}, fmt.Errorf("创建图像下载请求失败: %w", err)
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return Image{}, fmt.Errorf("下载图像失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Image{}, fmt.Errorf("下载图像失败: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Image{}, fmt.Errorf("读取图像响应失败: %w", err)
	}
	format := imageFormatFromBytes(data)
	if format == "" {
		format = "png"
	}
	return Image{
		Data:      data,
		MIMEType:  mimeTypeForFormat(format),
		Extension: extensionForFormat(format),
		SourceURL: imageURL,
	}, nil
}

// --- 尺寸转长宽比 ---

// sizeToAspectRatio 将 OpenAI 的 size（如 "1024x1024"）转为 MiniMax 的 aspect_ratio（如 "1:1"）。
// 任意尺寸都能匹配到最接近的支持长宽比，不会失败。
func sizeToAspectRatio(size string) string {
	size = strings.TrimSpace(size)
	if size == "" || size == "auto" {
		return "1:1"
	}
	if strings.Contains(size, ":") {
		return size
	}
	switch size {
	case "1024x1024", "2048x2048":
		return "1:1"
	case "1280x720", "1920x1080":
		return "16:9"
	case "720x1280", "1080x1920":
		return "9:16"
	case "1152x864":
		return "4:3"
	case "864x1152":
		return "3:4"
	case "1248x832":
		return "3:2"
	case "832x1248":
		return "2:3"
	}
	return nearestAspectRatio(size)
}

var supportedAspectRatios = []struct {
	label string
	value float64
}{
	{"1:1", 1.0},
	{"16:9", 16.0 / 9.0},
	{"9:16", 9.0 / 16.0},
	{"4:3", 4.0 / 3.0},
	{"3:4", 3.0 / 4.0},
	{"3:2", 3.0 / 2.0},
	{"2:3", 2.0 / 3.0},
	{"21:9", 21.0 / 9.0},
}

func nearestAspectRatio(size string) string {
	parts := strings.SplitN(size, "x", 2)
	if len(parts) != 2 {
		return "1:1"
	}
	width, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || width <= 0 || height <= 0 {
		return "1:1"
	}
	target := float64(width) / float64(height)
	best := supportedAspectRatios[0]
	bestDiff := target - best.value
	if bestDiff < 0 {
		bestDiff = -bestDiff
	}
	for _, ratio := range supportedAspectRatios[1:] {
		diff := target - ratio.value
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			best, bestDiff = ratio, diff
		}
	}
	return best.label
}
