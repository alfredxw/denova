package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"nova/config"
	"nova/internal/book"
	"nova/internal/illustration"
	"nova/internal/imagegen"
	"nova/internal/interactiveimage"
)

const (
	generateImageToolName                   = "generate_image"
	generateChapterIllustrationToolName     = "generate_chapter_illustration"
	generatedImageResultSchema              = "generated_image.v1"
	generateImagePurposeChapterIllustration = "chapter_illustration"
	generateImagePurposeInteractiveImage    = "interactive_image"
	generateImageSupportedSizeDescription   = "可选图像尺寸，留空时由 Agent 按生成意图决定；仅支持 2K: 2048x2048、2304x1728、1728x2304、2848x1600、1600x2848、2496x1664、1664x2496、3136x1344；3K: 3072x3072、3456x2592、2592x3456、4096x2304、2304x4096、2496x3744、3744x2496、4704x2016；4K: 4096x4096、3520x4704、4704x3520、5504x3040、3040x5504、3328x4992、4992x3328、6240x2656"
	generateImageDefaultAltText             = "生成图像"
)

type generateImageInput struct {
	Purpose      string `json:"purpose,omitempty" jsonschema:"description=图像用途，普通图像留空或填 general；章节插画填 chapter_illustration；互动图像填 interactive_image"`
	TargetPath   string `json:"target_path,omitempty" jsonschema:"description=关联的 workspace 相对路径。章节插画时填写章节路径，例如 chapters/001.md；普通图像可留空"`
	StoryID      string `json:"story_id,omitempty" jsonschema:"description=互动图像所属故事 ID，仅 purpose=interactive_image 时填写"`
	BranchID     string `json:"branch_id,omitempty" jsonschema:"description=互动图像所属分支 ID，仅 purpose=interactive_image 时填写"`
	TurnID       string `json:"turn_id,omitempty" jsonschema:"description=互动图像所属回合 ID，仅 purpose=interactive_image 时填写"`
	Prompt       string `json:"prompt" jsonschema:"required,description=给图像模型的完整视觉提示词，应说明主体、场景、构图、风格、光线、情绪和需要避免的文字水印"`
	AltText      string `json:"alt_text,omitempty" jsonschema:"description=Markdown 图像 alt 文案；不填时由章节名生成"`
	ProfileID    string `json:"profile_id,omitempty" jsonschema:"description=可选图像模型配置 ID；不填使用当前默认 image profile"`
	N            int    `json:"n,omitempty" jsonschema:"description=生成图像数量，普通图像可填 1 到 10；章节插画和互动图像固定生成 1 张"`
	Size         string `json:"size,omitempty" jsonschema:"description=可选图像尺寸，留空时由 Agent 按生成意图决定；仅支持 2K/3K/4K 预设尺寸，详见工具说明"`
	Quality      string `json:"quality,omitempty" jsonschema:"description=可选图像质量，例如 auto、standard、hd、low、medium、high"`
	OutputFormat string `json:"output_format,omitempty" jsonschema:"description=可选输出格式：png 或 jpeg"`
}

type generatedImageToolResult struct {
	Schema       string                    `json:"schema"`
	Purpose      string                    `json:"purpose,omitempty"`
	TargetPath   string                    `json:"target_path,omitempty"`
	ProfileID    string                    `json:"profile_id"`
	Provider     string                    `json:"provider"`
	Model        string                    `json:"model"`
	Size         string                    `json:"size,omitempty"`
	Quality      string                    `json:"quality,omitempty"`
	OutputFormat string                    `json:"output_format,omitempty"`
	CreatedAt    string                    `json:"created_at,omitempty"`
	Images       []generatedImageToolImage `json:"images"`
}

type generatedImageToolImage struct {
	Path          string `json:"path"`
	Markdown      string `json:"markdown,omitempty"`
	AltText       string `json:"alt_text,omitempty"`
	MIMEType      string `json:"mime_type,omitempty"`
	SizeBytes     int    `json:"size_bytes,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

func newIllustrationTools(cfg *config.Config) ([]tool.BaseTool, error) {
	if cfg == nil {
		return nil, nil
	}
	workspace := strings.TrimSpace(cfg.Workspace)
	description := "生成图像并保存到 workspace。普通图像保存到 assets/image/generated/；purpose=chapter_illustration 时基于 target_path 指向的章节生成一张非剧透插画，保存到 assets/illustrations/ 并返回可手动插入正文的 Markdown 图像引用；purpose=interactive_image 时必须填写 story_id、branch_id、turn_id，保存到 assets/interactive/images/。只写图像和元数据，不会自动修改正文。" + generateImageSupportedSizeDescription
	generateTool, err := utils.InferTool(generateImageToolName, description, func(ctx context.Context, input generateImageInput) (string, error) {
		if workspace == "" {
			return "", fmt.Errorf("当前 workspace 不可用，无法生成图像")
		}
		bookService := book.NewService(workspace)
		result, err := generateImageForTool(ctx, cfg, bookService, input)
		if err != nil {
			return "", err
		}
		data, err := json.Marshal(result)
		if err != nil {
			return "", err
		}
		return string(data), nil
	})
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{generateTool}, nil
}

func generateImageForTool(ctx context.Context, cfg *config.Config, bookService *book.Service, input generateImageInput) (any, error) {
	input.Prompt = mergeImagePresetToolPrompt(cfg, input.Prompt)
	purpose := normalizeGenerateImagePurpose(input.Purpose)
	if purpose == generateImagePurposeChapterIllustration {
		return illustration.NewService().Generate(ctx, cfg, bookService, illustration.GenerateRequest{
			ChapterPath:  input.TargetPath,
			Prompt:       input.Prompt,
			AltText:      input.AltText,
			ProfileID:    input.ProfileID,
			Size:         input.Size,
			Quality:      input.Quality,
			OutputFormat: input.OutputFormat,
		})
	}
	if purpose == generateImagePurposeInteractiveImage {
		return interactiveimage.NewService().Generate(ctx, cfg, bookService, interactiveimage.GenerateRequest{
			StoryID:      input.StoryID,
			BranchID:     input.BranchID,
			TurnID:       input.TurnID,
			Prompt:       input.Prompt,
			AltText:      input.AltText,
			ProfileID:    input.ProfileID,
			Size:         input.Size,
			Quality:      input.Quality,
			OutputFormat: input.OutputFormat,
		})
	}
	return generateGeneralImageForTool(ctx, cfg, bookService, input)
}

func mergeImagePresetToolPrompt(cfg *config.Config, prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || cfg == nil || strings.TrimSpace(cfg.ImagePresetToolPrompt) == "" {
		return prompt
	}
	return strings.TrimSpace(fmt.Sprintf("# 图像方案（原样注入）\n\n%s\n\n# 本次图像请求\n\n%s", strings.TrimSpace(cfg.ImagePresetToolPrompt), prompt))
}

func generateGeneralImageForTool(ctx context.Context, cfg *config.Config, bookService *book.Service, input generateImageInput) (generatedImageToolResult, error) {
	if bookService == nil || strings.TrimSpace(bookService.Workspace()) == "" {
		return generatedImageToolResult{}, fmt.Errorf("workspace 不可用")
	}
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return generatedImageToolResult{}, imagegen.ErrPromptRequired
	}
	n := input.N
	if n == 0 {
		n = 1
	}
	generated, err := imagegen.NewService().Generate(ctx, cfg, imagegen.GenerateRequest{
		ProfileID:    strings.TrimSpace(input.ProfileID),
		Prompt:       prompt,
		N:            n,
		Size:         strings.TrimSpace(input.Size),
		Quality:      strings.TrimSpace(input.Quality),
		OutputFormat: strings.TrimSpace(input.OutputFormat),
	})
	if err != nil {
		return generatedImageToolResult{}, err
	}
	createdAt := time.Now().UTC()
	result := generatedImageToolResult{
		Schema:       generatedImageResultSchema,
		Purpose:      normalizeGenerateImagePurpose(input.Purpose),
		TargetPath:   filepath.ToSlash(strings.TrimSpace(input.TargetPath)),
		ProfileID:    generated.ProfileID,
		Provider:     generated.Provider,
		Model:        generated.Model,
		Size:         generated.Size,
		Quality:      generated.Quality,
		OutputFormat: generated.OutputFormat,
		CreatedAt:    createdAt.Format(time.RFC3339),
		Images:       make([]generatedImageToolImage, 0, len(generated.Images)),
	}
	for index, image := range generated.Images {
		if len(image.Data) == 0 {
			return generatedImageToolResult{}, fmt.Errorf("图像模型返回了空图像")
		}
		ext := normalizeGeneratedImageExtension(image.Extension, generated.OutputFormat, input.OutputFormat)
		if ext == "" {
			return generatedImageToolResult{}, fmt.Errorf("无法识别图像格式")
		}
		if result.OutputFormat == "" {
			result.OutputFormat = ext
		}
		imagePath := generatedToolImagePath(createdAt, index, ext)
		if err := bookService.WriteBinaryFile(imagePath, image.Data); err != nil {
			return generatedImageToolResult{}, fmt.Errorf("保存生成图像失败: %w", err)
		}
		altText := strings.TrimSpace(input.AltText)
		if altText == "" {
			altText = generateImageDefaultAltText
		}
		markdown := fmt.Sprintf("![%s](%s)", escapeGeneratedImageAlt(altText), imagePath)
		result.Images = append(result.Images, generatedImageToolImage{
			Path:          imagePath,
			Markdown:      markdown,
			AltText:       altText,
			MIMEType:      image.MIMEType,
			SizeBytes:     len(image.Data),
			RevisedPrompt: image.RevisedPrompt,
		})
	}
	if len(result.Images) == 0 {
		return generatedImageToolResult{}, fmt.Errorf("图像模型未返回图像")
	}
	return result, nil
}

func parseChapterIllustrationToolResult(toolName, content string) (*illustration.Result, error) {
	if !isImageGenerationToolName(toolName) {
		return nil, nil
	}
	body := strings.TrimSpace(content)
	if before, _, ok := strings.Cut(body, "\n\n[Nova tool result metadata]"); ok {
		body = strings.TrimSpace(before)
	}
	if body == "" {
		return nil, nil
	}
	var result illustration.Result
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}
	if result.Schema != illustration.ResultSchema {
		return nil, nil
	}
	return &result, nil
}

func parseGeneratedImageToolTarget(toolName, content string) string {
	if !isImageGenerationToolName(toolName) {
		return ""
	}
	body := strings.TrimSpace(content)
	if before, _, ok := strings.Cut(body, "\n\n[Nova tool result metadata]"); ok {
		body = strings.TrimSpace(before)
	}
	if body == "" {
		return ""
	}
	var result generatedImageToolResult
	if err := json.Unmarshal([]byte(body), &result); err != nil || result.Schema != generatedImageResultSchema {
		return ""
	}
	if len(result.Images) == 0 {
		return ""
	}
	return strings.TrimSpace(result.Images[0].Path)
}

func parseInteractiveImageToolResult(toolName, content string) (*interactiveimage.Result, error) {
	if !isImageGenerationToolName(toolName) {
		return nil, nil
	}
	body := strings.TrimSpace(content)
	if before, _, ok := strings.Cut(body, "\n\n[Nova tool result metadata]"); ok {
		body = strings.TrimSpace(before)
	}
	if body == "" {
		return nil, nil
	}
	var result interactiveimage.Result
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}
	if result.Schema != interactiveimage.ResultSchema {
		return nil, nil
	}
	return &result, nil
}

func isImageGenerationToolName(toolName string) bool {
	normalized := normalizeToolName(toolName)
	return normalized == generateImageToolName || normalized == generateChapterIllustrationToolName
}

func normalizeGenerateImagePurpose(purpose string) string {
	switch strings.ToLower(strings.TrimSpace(purpose)) {
	case "", "general":
		return ""
	case generateImagePurposeChapterIllustration:
		return generateImagePurposeChapterIllustration
	case generateImagePurposeInteractiveImage:
		return generateImagePurposeInteractiveImage
	default:
		return strings.ToLower(strings.TrimSpace(purpose))
	}
}

func normalizeGeneratedImageExtension(values ...string) string {
	for _, value := range values {
		value = strings.ToLower(strings.Trim(strings.TrimSpace(value), "."))
		switch value {
		case "jpg":
			return "jpeg"
		case "jpeg", "png":
			return value
		}
	}
	return ""
}

func generatedToolImagePath(createdAt time.Time, index int, extension string) string {
	return filepath.ToSlash(filepath.Join(
		"assets",
		"image",
		"generated",
		fmt.Sprintf("%s-%s-%02d.%s", createdAt.Format("20060102-150405"), imageToolRandomSuffix(), index+1, extension),
	))
}

func imageToolRandomSuffix() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}

func escapeGeneratedImageAlt(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\\", "\\\\"), "]", "\\]")
}
