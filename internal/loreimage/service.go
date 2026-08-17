package loreimage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"denova/config"
	"denova/internal/book"
	"denova/internal/imagegen"
)

const (
	ResultSchema        = "lore_item_image.v1"
	sourceTool          = "generate_image"
	defaultImageSize    = "2048x2048"
	defaultOutputFormat = "png"
	maxPresetChars      = 4000
	maxBriefChars       = 1000
	maxContentChars     = 4000
	maxInstructionChars = 1000
	maxVisualGuidance   = 2500
)

type ImageGenerator interface {
	Generate(ctx context.Context, cfg *config.Config, request imagegen.GenerateRequest) (imagegen.Result, error)
}

type visualGuidanceRefineRequest struct {
	Item                book.LoreItem
	ImagePresetGuidance string
	Instruction         string
}

type visualGuidanceRefiner interface {
	Refine(ctx context.Context, cfg *config.Config, request visualGuidanceRefineRequest) (string, error)
}

type Service struct {
	generator ImageGenerator
	refiner   visualGuidanceRefiner
	now       func() time.Time
	suffix    func() string
}

type GenerateRequest struct {
	Item                book.LoreItem
	Instruction         string
	ImagePresetID       string
	ImagePresetGuidance string
	ImagePresetPrompt   string
	ProfileID           string
	Size                string
	Quality             string
	OutputFormat        string
	visualGuidance      string
}

type Meta struct {
	Schema        string `json:"schema"`
	Source        string `json:"source"`
	ItemID        string `json:"item_id"`
	ItemType      string `json:"item_type,omitempty"`
	ItemName      string `json:"item_name,omitempty"`
	Instruction   string `json:"instruction,omitempty"`
	ImagePresetID string `json:"image_preset_id,omitempty"`
	Prompt        string `json:"prompt"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
	ImagePath     string `json:"image_path"`
	MetaPath      string `json:"meta_path"`
	AltText       string `json:"alt_text,omitempty"`
	ProfileID     string `json:"profile_id"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	Size          string `json:"size,omitempty"`
	Quality       string `json:"quality,omitempty"`
	OutputFormat  string `json:"output_format,omitempty"`
	MIMEType      string `json:"mime_type,omitempty"`
	SizeBytes     int    `json:"size_bytes,omitempty"`
	CreatedAt     string `json:"created_at"`
}

func NewService() *Service {
	return newServiceWithDependencies(imagegen.NewService(), newModelVisualGuidanceRefiner())
}

func NewServiceWithGenerator(generator ImageGenerator) *Service {
	return newServiceWithDependencies(generator, newModelVisualGuidanceRefiner())
}

func newServiceWithDependencies(generator ImageGenerator, refiner visualGuidanceRefiner) *Service {
	return &Service{
		generator: generator,
		refiner:   refiner,
		now:       time.Now,
		suffix:    randomSuffix,
	}
}

func (s *Service) Generate(ctx context.Context, cfg *config.Config, bookService *book.Service, request GenerateRequest) (book.LoreItemImage, error) {
	if s == nil {
		s = NewService()
	}
	if s.generator == nil {
		s.generator = imagegen.NewService()
	}
	if cfg == nil {
		return book.LoreItemImage{}, fmt.Errorf("运行配置不可用")
	}
	if bookService == nil || strings.TrimSpace(bookService.Workspace()) == "" {
		return book.LoreItemImage{}, fmt.Errorf("workspace 不可用")
	}
	item := request.Item
	if strings.TrimSpace(item.ID) == "" {
		return book.LoreItemImage{}, fmt.Errorf("资料 ID 不能为空")
	}
	if strings.TrimSpace(item.Name) == "" {
		return book.LoreItemImage{}, fmt.Errorf("资料名称不能为空")
	}
	if s.refiner == nil {
		return book.LoreItemImage{}, fmt.Errorf("资料图片视觉指导提炼器不可用")
	}
	guidance, err := s.refiner.Refine(ctx, cfg, visualGuidanceRefineRequest{
		Item:                item,
		ImagePresetGuidance: request.ImagePresetGuidance,
		Instruction:         request.Instruction,
	})
	if err != nil {
		return book.LoreItemImage{}, fmt.Errorf("提炼资料图片视觉指导失败: %w", err)
	}
	request.visualGuidance = guidance
	prompt := BuildPrompt(request)
	if prompt == "" {
		return book.LoreItemImage{}, imagegen.ErrPromptRequired
	}

	generated, err := s.generator.Generate(ctx, cfg, imagegen.GenerateRequest{
		ProfileID:    strings.TrimSpace(request.ProfileID),
		Prompt:       prompt,
		N:            1,
		Size:         firstNonEmpty(request.Size, defaultImageSize),
		Quality:      strings.TrimSpace(request.Quality),
		OutputFormat: firstNonEmpty(request.OutputFormat, defaultOutputFormat),
	})
	if err != nil {
		return book.LoreItemImage{}, err
	}
	if len(generated.Images) == 0 {
		return book.LoreItemImage{}, fmt.Errorf("图像模型未返回图像")
	}
	image := generated.Images[0]
	if len(image.Data) == 0 {
		return book.LoreItemImage{}, fmt.Errorf("图像模型返回了空图像")
	}
	if err := ctx.Err(); err != nil {
		return book.LoreItemImage{}, err
	}
	ext := normalizeImageExtension(image.Extension, generated.OutputFormat, request.OutputFormat, defaultOutputFormat)
	if ext == "" {
		return book.LoreItemImage{}, fmt.Errorf("无法识别图像格式")
	}

	createdAt := s.now().UTC()
	dir := filepath.ToSlash(filepath.Join(
		"assets",
		"lore",
		"images",
		safePathSegment(item.ID),
		fmt.Sprintf("%s-%s", createdAt.Format("20060102-150405"), s.suffix()),
	))
	imagePath := filepath.ToSlash(filepath.Join(dir, "image."+ext))
	metaPath := filepath.ToSlash(filepath.Join(dir, "meta.json"))
	if err := bookService.WriteBinaryFile(imagePath, image.Data); err != nil {
		return book.LoreItemImage{}, fmt.Errorf("保存资料项图像失败: %w", err)
	}

	result := book.LoreItemImage{
		Schema:        ResultSchema,
		ImagePath:     imagePath,
		MetaPath:      metaPath,
		AltText:       defaultAltText(item),
		ImagePresetID: strings.TrimSpace(request.ImagePresetID),
		ProfileID:     generated.ProfileID,
		Provider:      generated.Provider,
		Model:         generated.Model,
		Size:          generated.Size,
		Quality:       generated.Quality,
		OutputFormat:  firstNonEmpty(generated.OutputFormat, ext),
		CreatedAt:     createdAt.Format(time.RFC3339),
		RevisedPrompt: image.RevisedPrompt,
		MIMEType:      image.MIMEType,
		SizeBytes:     len(image.Data),
	}
	meta := Meta{
		Schema:        ResultSchema,
		Source:        sourceTool,
		ItemID:        item.ID,
		ItemType:      item.Type,
		ItemName:      item.Name,
		Instruction:   trimRunes(request.Instruction, maxInstructionChars),
		ImagePresetID: result.ImagePresetID,
		Prompt:        prompt,
		RevisedPrompt: result.RevisedPrompt,
		ImagePath:     result.ImagePath,
		MetaPath:      result.MetaPath,
		AltText:       result.AltText,
		ProfileID:     result.ProfileID,
		Provider:      result.Provider,
		Model:         result.Model,
		Size:          result.Size,
		Quality:       result.Quality,
		OutputFormat:  result.OutputFormat,
		MIMEType:      result.MIMEType,
		SizeBytes:     result.SizeBytes,
		CreatedAt:     result.CreatedAt,
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return book.LoreItemImage{}, err
	}
	if err := bookService.WriteFile(metaPath, string(data)+"\n"); err != nil {
		return book.LoreItemImage{}, fmt.Errorf("保存资料项图像元数据失败: %w", err)
	}
	return result, nil
}

func BuildPrompt(request GenerateRequest) string {
	item := request.Item
	preset := sanitizePromptText(trimRunes(request.ImagePresetPrompt, maxPresetChars))
	visualGuidance := sanitizePromptText(trimRunes(request.visualGuidance, maxVisualGuidance))
	var sb strings.Builder
	if preset != "" {
		sb.WriteString("# 图像风格要求\n\n")
		sb.WriteString(preset)
		sb.WriteString("\n\n")
	}
	sb.WriteString("# 本次资料项图片请求\n\n")
	sb.WriteString("为资料库条目生成一张可作为设定预览和创作参考的视觉图。画面应突出主体、身份或规则意象，适合在资料库列表中识别；不要生成任何文字、标题、作者名、水印、logo、UI 面板或二维码。\n\n")
	writePromptLine(&sb, "资料类型", loreTypeLabel(item.Type))
	writePromptLine(&sb, "资料名称", item.Name)
	if visualGuidance != "" {
		sb.WriteString("\n## 视觉绘制指导\n\n")
		sb.WriteString(visualGuidance)
		sb.WriteString("\n")
	}
	return sanitizePromptText(strings.TrimSpace(sb.String()))
}

func sanitizePromptText(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(value, "角色资料卡", ""))
}

func writePromptLine(sb *strings.Builder, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	sb.WriteString("- ")
	sb.WriteString(key)
	sb.WriteString("：")
	sb.WriteString(value)
	sb.WriteString("\n")
}

func defaultAltText(item book.LoreItem) string {
	name := strings.TrimSpace(item.Name)
	if name == "" {
		return "资料项图片"
	}
	return "资料项图片：" + name
}

func loreTypeLabel(value string) string {
	switch strings.TrimSpace(value) {
	case "character":
		return "角色"
	case "world":
		return "世界观"
	case "location":
		return "地点"
	case "faction":
		return "势力"
	case "rule":
		return "规则"
	case "item":
		return "物品"
	default:
		return "资料"
	}
}

func normalizeImageExtension(values ...string) string {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func safePathSegment(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
		case r == '-' || r == '_':
			if !lastDash && b.Len() > 0 {
				b.WriteRune(r)
				lastDash = true
			}
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	if segment := strings.Trim(b.String(), "-_"); segment != "" {
		return segment
	}
	return "lore-item"
}

func trimRunes(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func randomSuffix() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
