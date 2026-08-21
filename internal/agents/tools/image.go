package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/book"
	imageasset "denova/internal/image/asset"
	imagegen "denova/internal/image/generation"
)

const (
	generateImageToolName                   = "generate_image"
	generateChapterIllustrationToolName     = "generate_chapter_illustration"
	generatedImageResultSchema              = "generated_image.v1"
	generatedImageReceiptSchema             = "generated_image.receipt.v1"
	generateImagePurposeChapterIllustration = "chapter_illustration"
	generateImagePurposeInteractiveImage    = "interactive_image"
	generateImageDefaultAltText             = "Generated image"
)

const GenerateImageToolName = generateImageToolName

type generateImageInput struct {
	Purpose      string `json:"purpose,omitempty" jsonschema:"description=Image purpose. Leave empty or use general for ordinary images; use chapter_illustration for chapter art; use interactive_image for interactive art."`
	TargetPath   string `json:"target_path,omitempty" jsonschema:"description=Related workspace-relative path. For chapter illustrations, provide a chapter path such as chapters/001.md; ordinary images may omit it."`
	StoryID      string `json:"story_id,omitempty" jsonschema:"description=Story ID for an interactive image; provide only when purpose=interactive_image."`
	BranchID     string `json:"branch_id,omitempty" jsonschema:"description=Branch ID for an interactive image; provide only when purpose=interactive_image."`
	TurnID       string `json:"turn_id,omitempty" jsonschema:"description=Turn ID for an interactive image; provide only when purpose=interactive_image."`
	Prompt       string `json:"prompt" jsonschema:"required,description=Complete visual prompt for the image model, including subject, scene, composition, style, lighting, mood, and text or watermarks to avoid."`
	AltText      string `json:"alt_text,omitempty" jsonschema:"description=Markdown image alt text; generated from the chapter name when omitted."`
	ProfileID    string `json:"profile_id,omitempty" jsonschema:"description=Optional image model profile ID; omit to use the current default image profile."`
	N            int    `json:"n,omitempty" jsonschema:"description=Number of images. Ordinary images accept 1 to 10; chapter illustrations and interactive images always generate one."`
	Size         string `json:"size,omitempty" jsonschema:"description=Optional image dimensions such as 1024x1024. Support depends on the selected provider."`
	AspectRatio  string `json:"aspect_ratio,omitempty" jsonschema:"description=Optional aspect ratio such as 1:1, 16:9, or 9:16. The provider chooses the nearest supported ratio when needed."`
	Resolution   string `json:"resolution,omitempty" jsonschema:"description=Optional provider resolution tier such as 1K or 2K."`
	Quality      string `json:"quality,omitempty" jsonschema:"description=Optional image quality, such as auto, standard, hd, low, medium, or high."`
	OutputFormat string `json:"output_format,omitempty" jsonschema:"description=Optional output format: png, jpeg, or webp."`
}

type generatedImageToolResult struct {
	Schema       string                      `json:"schema"`
	Status       string                      `json:"status"`
	Purpose      string                      `json:"purpose,omitempty"`
	TargetPath   string                      `json:"target_path,omitempty"`
	ProfileID    string                      `json:"profile_id"`
	Provider     string                      `json:"provider"`
	Model        string                      `json:"model"`
	Size         string                      `json:"size,omitempty"`
	Quality      string                      `json:"quality,omitempty"`
	OutputFormat string                      `json:"output_format,omitempty"`
	CreatedAt    string                      `json:"created_at,omitempty"`
	Images       []generatedImageToolImage   `json:"images"`
	Failures     []generatedImageToolFailure `json:"failures,omitempty"`
}

type generatedImageToolFailure struct {
	Index   int    `json:"index"`
	Path    string `json:"path,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type generatedImageToolImage struct {
	Path          string `json:"path"`
	Markdown      string `json:"markdown,omitempty"`
	AltText       string `json:"alt_text,omitempty"`
	MIMEType      string `json:"mime_type,omitempty"`
	SizeBytes     int    `json:"size_bytes,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// generatedImageReceiptDetails is the stable cross-turn projection of a
// non-idempotent image generation result. Binary data already lives in the
// workspace, so the receipt retains every canonical path without duplicating
// prompts or provider output in future model context.
type generatedImageReceiptDetails struct {
	Schema       string                      `json:"schema"`
	ResultSchema string                      `json:"result_schema"`
	Status       string                      `json:"status"`
	Purpose      string                      `json:"purpose,omitempty"`
	TargetPath   string                      `json:"target_path,omitempty"`
	ChapterPath  string                      `json:"chapter_path,omitempty"`
	StoryID      string                      `json:"story_id,omitempty"`
	BranchID     string                      `json:"branch_id,omitempty"`
	TurnID       string                      `json:"turn_id,omitempty"`
	ProfileID    string                      `json:"profile_id,omitempty"`
	Provider     string                      `json:"provider,omitempty"`
	Model        string                      `json:"model,omitempty"`
	Size         string                      `json:"size,omitempty"`
	Quality      string                      `json:"quality,omitempty"`
	OutputFormat string                      `json:"output_format,omitempty"`
	CreatedAt    string                      `json:"created_at,omitempty"`
	Images       []generatedImageReceiptFile `json:"images"`
	Failures     []generatedImageToolFailure `json:"failures,omitempty"`
}

type generatedImageReceiptFile struct {
	Path      string `json:"path"`
	MetaPath  string `json:"meta_path,omitempty"`
	Markdown  string `json:"markdown,omitempty"`
	AltText   string `json:"alt_text,omitempty"`
	MIMEType  string `json:"mime_type,omitempty"`
	SizeBytes int    `json:"size_bytes,omitempty"`
}

func newIllustrationTools(cfg *config.Config) ([]agent.ToolDefinition, error) {
	if cfg == nil {
		return nil, nil
	}
	workspace := strings.TrimSpace(cfg.Workspace)
	description := "Generate images with the selected image-provider profile and save them to the workspace. Ordinary images go to assets/image/generated/. With purpose=chapter_illustration, generate one spoiler-free illustration from the chapter at target_path, save it under assets/illustrations/, and return a Markdown image reference for manual insertion. With purpose=interactive_image, story_id, branch_id, and turn_id are required and the image goes to assets/interactive/images/. Provider-specific size, aspect ratio, resolution, quality, and format support is validated by the configured adapter. The tool writes only image files and metadata; it never edits prose automatically."
	generateTool, err := agent.InferTool(generateImageToolName, description, func(ctx context.Context, input generateImageInput) (agent.ToolResult, error) {
		if workspace == "" {
			return agent.ToolResult{}, fmt.Errorf("cannot generate an image because the current workspace is unavailable")
		}
		bookService := book.NewService(workspace)
		result, err := generateImageForTool(ctx, cfg, bookService, input)
		if err != nil {
			return agent.ToolResult{}, err
		}
		return newGeneratedImageToolResult(result)
	})
	if err != nil {
		return nil, err
	}
	definedGenerateTool, err := defineTool(generateTool, workspaceWriteDescriptor(ToolSourceImage, config.AgentToolImageGeneration, agent.ToolRecoveryNonIdempotent))
	if err != nil {
		return nil, err
	}
	return []agent.ToolDefinition{definedGenerateTool}, nil
}

func newGeneratedImageToolResult(value any) (agent.ToolResult, error) {
	content, err := json.Marshal(value)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("encode generated image result: %w", err)
	}
	receipt, target, err := generatedImageReceipt(value)
	if err != nil {
		return agent.ToolResult{}, err
	}
	details, err := json.Marshal(receipt)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("encode generated image receipt: %w", err)
	}
	result := agent.TextToolResult(string(content))
	result.Details = details
	result.Metadata.Target = target
	return result, nil
}

func generatedImageReceipt(value any) (generatedImageReceiptDetails, string, error) {
	receipt := generatedImageReceiptDetails{Schema: generatedImageReceiptSchema, Status: "success"}
	switch result := value.(type) {
	case generatedImageToolResult:
		receipt.ResultSchema = result.Schema
		receipt.Status = result.Status
		receipt.Purpose = result.Purpose
		receipt.TargetPath = result.TargetPath
		receipt.ProfileID = result.ProfileID
		receipt.Provider = result.Provider
		receipt.Model = result.Model
		receipt.Size = result.Size
		receipt.Quality = result.Quality
		receipt.OutputFormat = result.OutputFormat
		receipt.CreatedAt = result.CreatedAt
		receipt.Failures = append([]generatedImageToolFailure(nil), result.Failures...)
		for _, image := range result.Images {
			receipt.Images = append(receipt.Images, generatedImageReceiptFile{
				Path: image.Path, Markdown: image.Markdown, AltText: image.AltText,
				MIMEType: image.MIMEType, SizeBytes: image.SizeBytes,
			})
		}
		if len(receipt.Images) > 0 {
			return receipt, receipt.Images[0].Path, nil
		}
	case imageasset.IllustrationResult:
		receipt.ResultSchema = result.Schema
		receipt.Purpose = generateImagePurposeChapterIllustration
		receipt.ChapterPath = result.ChapterPath
		receipt.ProfileID = result.ProfileID
		receipt.Provider = result.Provider
		receipt.Model = result.Model
		receipt.Size = result.Size
		receipt.Quality = result.Quality
		receipt.OutputFormat = result.OutputFormat
		receipt.CreatedAt = result.CreatedAt
		receipt.Images = []generatedImageReceiptFile{{
			Path: result.ImagePath, MetaPath: result.MetaPath, Markdown: result.Markdown,
			AltText: result.AltText, MIMEType: result.MIMEType, SizeBytes: result.SizeBytes,
		}}
		return receipt, result.MetaPath, nil
	case imageasset.InteractiveResult:
		receipt.ResultSchema = result.Schema
		receipt.Purpose = generateImagePurposeInteractiveImage
		receipt.StoryID = result.StoryID
		receipt.BranchID = result.BranchID
		receipt.TurnID = result.TurnID
		receipt.ProfileID = result.ProfileID
		receipt.Provider = result.Provider
		receipt.Model = result.Model
		receipt.Size = result.Size
		receipt.Quality = result.Quality
		receipt.OutputFormat = result.OutputFormat
		receipt.CreatedAt = result.CreatedAt
		receipt.Images = []generatedImageReceiptFile{{
			Path: result.ImagePath, MetaPath: result.MetaPath, AltText: result.AltText,
			MIMEType: result.MIMEType, SizeBytes: result.SizeBytes,
		}}
		return receipt, result.MetaPath, nil
	default:
		return generatedImageReceiptDetails{}, "", fmt.Errorf("unsupported generated image result %T", value)
	}
	return receipt, "", nil
}

func generateImageForTool(ctx context.Context, cfg *config.Config, bookService *book.Service, input generateImageInput) (any, error) {
	input.Prompt = mergeImagePresetToolPrompt(cfg, input.Prompt)
	purpose := normalizeGenerateImagePurpose(input.Purpose)
	if purpose == generateImagePurposeChapterIllustration {
		return imageasset.NewService().GenerateIllustration(ctx, cfg, bookService, imageasset.IllustrationGenerateRequest{
			ChapterPath:  input.TargetPath,
			Prompt:       input.Prompt,
			AltText:      input.AltText,
			ProfileID:    input.ProfileID,
			Size:         input.Size,
			AspectRatio:  input.AspectRatio,
			Resolution:   input.Resolution,
			Quality:      input.Quality,
			OutputFormat: input.OutputFormat,
		})
	}
	if purpose == generateImagePurposeInteractiveImage {
		return imageasset.NewService().GenerateInteractive(ctx, cfg, bookService, imageasset.InteractiveGenerateRequest{
			StoryID:      input.StoryID,
			BranchID:     input.BranchID,
			TurnID:       input.TurnID,
			Prompt:       input.Prompt,
			AltText:      input.AltText,
			ProfileID:    input.ProfileID,
			Size:         input.Size,
			AspectRatio:  input.AspectRatio,
			Resolution:   input.Resolution,
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
	return strings.TrimSpace(fmt.Sprintf("# Image Style Requirements\n\n%s\n\n# Current Image Request\n\n%s", strings.TrimSpace(cfg.ImagePresetToolPrompt), prompt))
}

func generateGeneralImageForTool(ctx context.Context, cfg *config.Config, bookService *book.Service, input generateImageInput) (generatedImageToolResult, error) {
	if bookService == nil || strings.TrimSpace(bookService.Workspace()) == "" {
		return generatedImageToolResult{}, fmt.Errorf("workspace is unavailable")
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
		AspectRatio:  strings.TrimSpace(input.AspectRatio),
		Resolution:   strings.TrimSpace(input.Resolution),
		Quality:      strings.TrimSpace(input.Quality),
		OutputFormat: strings.TrimSpace(input.OutputFormat),
	})
	if err != nil {
		return generatedImageToolResult{}, err
	}
	createdAt := time.Now().UTC()
	return persistGeneratedImages(bookService, input, generated, createdAt)
}

func persistGeneratedImages(bookService *book.Service, input generateImageInput, generated imagegen.Result, createdAt time.Time) (generatedImageToolResult, error) {
	result := generatedImageToolResult{
		Schema:       generatedImageResultSchema,
		Status:       "success",
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
	for _, failure := range generated.Failures {
		result.Failures = append(result.Failures, generatedImageToolFailure{
			Index: failure.Index, Code: failure.Code, Message: failure.Message,
		})
	}
	for index, image := range generated.Images {
		if len(image.Data) == 0 {
			result.Failures = append(result.Failures, generatedImageToolFailure{
				Index: index, Code: "empty_image", Message: "The image provider returned an empty image.",
			})
			continue
		}
		ext := normalizeGeneratedImageExtension(image.Extension, generated.OutputFormat, input.OutputFormat)
		if ext == "" {
			result.Failures = append(result.Failures, generatedImageToolFailure{
				Index: index, Code: "unknown_format", Message: "The generated image format is unknown.",
			})
			continue
		}
		if result.OutputFormat == "" {
			result.OutputFormat = ext
		}
		imagePath := generatedToolImagePath(createdAt, index, ext)
		if err := bookService.WriteBinaryFile(imagePath, image.Data); err != nil {
			message := fmt.Sprintf("Failed to save generated image: %v", err)
			result.Failures = append(result.Failures, generatedImageToolFailure{
				Index: index, Path: imagePath, Code: "save_failed", Message: message,
			})
			slog.ErrorContext(context.Background(), fmt.Sprintf("[image-tool] generated image save failed index=%d path=%s err=%v", index, imagePath, err))
			continue
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
		slog.InfoContext(context.Background(), fmt.Sprintf("[image-tool] generated image saved index=%d path=%s bytes=%d", index, imagePath, len(image.Data)))
	}
	if len(result.Images) == 0 {
		if len(result.Failures) == 0 {
			return generatedImageToolResult{}, fmt.Errorf("the image provider returned no images")
		}
		return generatedImageToolResult{}, fmt.Errorf("all generated images failed: %s", result.Failures[0].Message)
	}
	if len(result.Failures) > 0 {
		result.Status = "partial"
	}
	return result, nil
}

func parseChapterIllustrationToolResult(toolName, content string) (*imageasset.IllustrationResult, error) {
	if !isImageGenerationToolName(toolName) {
		return nil, nil
	}
	body := strings.TrimSpace(content)
	if before, _, ok := strings.Cut(body, "\n\n[Denova tool result metadata]"); ok {
		body = strings.TrimSpace(before)
	}
	if body == "" {
		return nil, nil
	}
	var envelope struct {
		Schema       string `json:"schema"`
		ResultSchema string `json:"result_schema"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return nil, err
	}
	if envelope.Schema == generatedImageReceiptSchema && envelope.ResultSchema == imageasset.IllustrationResultSchema {
		var receipt generatedImageReceiptDetails
		if err := json.Unmarshal([]byte(body), &receipt); err != nil {
			return nil, err
		}
		if len(receipt.Images) == 0 || strings.TrimSpace(receipt.Images[0].Path) == "" {
			return nil, nil
		}
		image := receipt.Images[0]
		return &imageasset.IllustrationResult{
			Schema: receipt.ResultSchema, ChapterPath: receipt.ChapterPath,
			ImagePath: image.Path, MetaPath: image.MetaPath, Markdown: image.Markdown, AltText: image.AltText,
			ProfileID: receipt.ProfileID, Provider: receipt.Provider, Model: receipt.Model,
			Size: receipt.Size, Quality: receipt.Quality, OutputFormat: receipt.OutputFormat, CreatedAt: receipt.CreatedAt,
			MIMEType: image.MIMEType, SizeBytes: image.SizeBytes,
		}, nil
	}
	var result imageasset.IllustrationResult
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}
	if result.Schema != imageasset.IllustrationResultSchema {
		return nil, nil
	}
	return &result, nil
}

// ParseChapterIllustrationResult decodes a chapter illustration emitted by
// the image tool, ignoring unrelated tool results.
func ParseChapterIllustrationResult(toolName, content string) (*imageasset.IllustrationResult, error) {
	return parseChapterIllustrationToolResult(toolName, content)
}

func parseGeneratedImageToolTarget(toolName, content string) string {
	if !isImageGenerationToolName(toolName) {
		return ""
	}
	body := strings.TrimSpace(content)
	if before, _, ok := strings.Cut(body, "\n\n[Denova tool result metadata]"); ok {
		body = strings.TrimSpace(before)
	}
	if body == "" {
		return ""
	}
	var receipt generatedImageReceiptDetails
	if err := json.Unmarshal([]byte(body), &receipt); err == nil && receipt.Schema == generatedImageReceiptSchema {
		if len(receipt.Images) == 0 {
			return ""
		}
		return strings.TrimSpace(receipt.Images[0].Path)
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

// ParseGeneratedImageTarget returns the first generated workspace path.
func ParseGeneratedImageTarget(toolName, content string) string {
	return parseGeneratedImageToolTarget(toolName, content)
}

func parseInteractiveImageToolResult(toolName, content string) (*imageasset.InteractiveResult, error) {
	if !isImageGenerationToolName(toolName) {
		return nil, nil
	}
	body := strings.TrimSpace(content)
	if before, _, ok := strings.Cut(body, "\n\n[Denova tool result metadata]"); ok {
		body = strings.TrimSpace(before)
	}
	if body == "" {
		return nil, nil
	}
	var envelope struct {
		Schema       string `json:"schema"`
		ResultSchema string `json:"result_schema"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return nil, err
	}
	if envelope.Schema == generatedImageReceiptSchema && envelope.ResultSchema == imageasset.InteractiveResultSchema {
		var receipt generatedImageReceiptDetails
		if err := json.Unmarshal([]byte(body), &receipt); err != nil {
			return nil, err
		}
		if len(receipt.Images) == 0 || strings.TrimSpace(receipt.Images[0].Path) == "" {
			return nil, nil
		}
		image := receipt.Images[0]
		return &imageasset.InteractiveResult{
			Schema: receipt.ResultSchema, StoryID: receipt.StoryID, BranchID: receipt.BranchID, TurnID: receipt.TurnID,
			ImagePath: image.Path, MetaPath: image.MetaPath, AltText: image.AltText,
			ProfileID: receipt.ProfileID, Provider: receipt.Provider, Model: receipt.Model,
			Size: receipt.Size, Quality: receipt.Quality, OutputFormat: receipt.OutputFormat, CreatedAt: receipt.CreatedAt,
			MIMEType: image.MIMEType, SizeBytes: image.SizeBytes,
		}, nil
	}
	var result imageasset.InteractiveResult
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}
	if result.Schema != imageasset.InteractiveResultSchema {
		return nil, nil
	}
	return &result, nil
}

// ParseInteractiveImageResult decodes an interactive image tool result.
func ParseInteractiveImageResult(toolName, content string) (*imageasset.InteractiveResult, error) {
	return parseInteractiveImageToolResult(toolName, content)
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
		case "jpeg", "png", "webp":
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
