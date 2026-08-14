package asset

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"denova/config"
	"denova/internal/book"
	"denova/internal/book/lore"
	imagegen "denova/internal/image/generation"
)

const (
	LoreResultSchema        = "lore_item_image.v1"
	MaxLoreImageUploadBytes = 16 * 1024 * 1024
	loreSourceTool          = "generate_image"
	loreSourceUpload        = "user_upload"
	defaultImageSize        = "2048x2048"
	defaultOutputFormat     = "png"
	maxPresetChars          = 4000
	maxBriefChars           = 1000
	maxContentChars         = 4000
	maxInstructionChars     = 1000
)

var (
	ErrLoreImageUploadEmpty    = errors.New("uploaded lore image is empty")
	ErrLoreImageUploadTooLarge = errors.New("uploaded lore image exceeds 16 MB")
	ErrLoreImageUploadInvalid  = errors.New("uploaded lore image must be PNG or JPEG")
)

type LoreGenerateRequest struct {
	Item              lore.Item
	Instruction       string
	ImagePresetID     string
	ImagePresetPrompt string
	ProfileID         string
	Size              string
	Quality           string
	OutputFormat      string
}

type LoreUploadRequest struct {
	Item     lore.Item
	Filename string
	Data     []byte
}

type loreMeta struct {
	Schema        string `json:"schema"`
	Source        string `json:"source"`
	SourceName    string `json:"source_name,omitempty"`
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

func (s *Service) GenerateLore(ctx context.Context, cfg *config.Config, bookService *book.Service, request LoreGenerateRequest) (lore.Image, error) {
	if s == nil {
		s = NewService()
	}
	if s.generator == nil {
		s.generator = imagegen.NewService()
	}
	if cfg == nil {
		return lore.Image{}, fmt.Errorf("运行配置不可用")
	}
	if bookService == nil || strings.TrimSpace(bookService.Workspace()) == "" {
		return lore.Image{}, fmt.Errorf("workspace 不可用")
	}
	item := request.Item
	if strings.TrimSpace(item.ID) == "" {
		return lore.Image{}, fmt.Errorf("资料 ID 不能为空")
	}
	if strings.TrimSpace(item.Name) == "" {
		return lore.Image{}, fmt.Errorf("资料名称不能为空")
	}
	prompt := BuildLorePrompt(request)
	if prompt == "" {
		return lore.Image{}, imagegen.ErrPromptRequired
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
		return lore.Image{}, err
	}
	if len(generated.Images) == 0 {
		return lore.Image{}, fmt.Errorf("图像模型未返回图像")
	}
	image := generated.Images[0]
	if len(image.Data) == 0 {
		return lore.Image{}, fmt.Errorf("图像模型返回了空图像")
	}
	if err := ctx.Err(); err != nil {
		return lore.Image{}, err
	}
	ext := normalizeImageExtension(image.Extension, generated.OutputFormat, request.OutputFormat, defaultOutputFormat)
	if ext == "" {
		return lore.Image{}, fmt.Errorf("无法识别图像格式")
	}

	createdAt := s.now().UTC()
	imagePath, metaPath := newLoreImageRunPaths(item.ID, createdAt, s.suffix(), ext)
	if err := bookService.WriteBinaryFile(imagePath, image.Data); err != nil {
		return lore.Image{}, fmt.Errorf("保存资料项图像失败: %w", err)
	}

	result := lore.Image{
		Schema:        LoreResultSchema,
		ImagePath:     imagePath,
		MetaPath:      metaPath,
		AltText:       defaultLoreAltText(item),
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
	meta := loreMeta{
		Schema:        LoreResultSchema,
		Source:        loreSourceTool,
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
		return lore.Image{}, err
	}
	if err := bookService.WriteFile(metaPath, string(data)+"\n"); err != nil {
		return lore.Image{}, fmt.Errorf("保存资料项图像元数据失败: %w", err)
	}
	return result, nil
}

// UploadLore validates and stores a user-provided image using the same durable
// asset and metadata shape as generated lore images.
func (s *Service) UploadLore(ctx context.Context, bookService *book.Service, request LoreUploadRequest) (lore.Image, error) {
	if s == nil {
		s = NewService()
	}
	if bookService == nil || strings.TrimSpace(bookService.Workspace()) == "" {
		return lore.Image{}, fmt.Errorf("workspace is unavailable")
	}
	item := request.Item
	if strings.TrimSpace(item.ID) == "" {
		return lore.Image{}, fmt.Errorf("lore item ID is required")
	}
	if len(request.Data) == 0 {
		return lore.Image{}, ErrLoreImageUploadEmpty
	}
	if len(request.Data) > MaxLoreImageUploadBytes {
		return lore.Image{}, ErrLoreImageUploadTooLarge
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(request.Data))
	if err != nil {
		return lore.Image{}, fmt.Errorf("%w: %v", ErrLoreImageUploadInvalid, err)
	}
	ext := normalizeImageExtension(format)
	if ext == "" {
		return lore.Image{}, ErrLoreImageUploadInvalid
	}
	if err := ctx.Err(); err != nil {
		return lore.Image{}, err
	}

	createdAt := s.now().UTC()
	imagePath, metaPath := newLoreImageRunPaths(item.ID, createdAt, s.suffix(), ext)
	if err := bookService.WriteBinaryFile(imagePath, request.Data); err != nil {
		return lore.Image{}, fmt.Errorf("save uploaded lore image: %w", err)
	}

	result := lore.Image{
		Schema:       LoreResultSchema,
		ImagePath:    imagePath,
		MetaPath:     metaPath,
		AltText:      defaultLoreAltText(item),
		ProfileID:    "manual",
		Provider:     loreSourceUpload,
		Model:        "manual",
		OutputFormat: ext,
		CreatedAt:    createdAt.Format(time.RFC3339),
		MIMEType:     loreImageMIMEType(ext),
		SizeBytes:    len(request.Data),
	}
	meta := loreMeta{
		Schema:       LoreResultSchema,
		Source:       loreSourceUpload,
		SourceName:   filepath.Base(strings.TrimSpace(request.Filename)),
		ItemID:       item.ID,
		ItemType:     item.Type,
		ItemName:     item.Name,
		ImagePath:    result.ImagePath,
		MetaPath:     result.MetaPath,
		AltText:      result.AltText,
		ProfileID:    result.ProfileID,
		Provider:     result.Provider,
		Model:        result.Model,
		OutputFormat: result.OutputFormat,
		MIMEType:     result.MIMEType,
		SizeBytes:    result.SizeBytes,
		CreatedAt:    result.CreatedAt,
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return lore.Image{}, err
	}
	if err := bookService.WriteFile(metaPath, string(data)+"\n"); err != nil {
		return lore.Image{}, fmt.Errorf("save lore image metadata: %w", err)
	}
	return result, nil
}

func newLoreImageRunPaths(itemID string, createdAt time.Time, suffix, ext string) (string, string) {
	dir := filepath.ToSlash(filepath.Join(
		"assets",
		"lore",
		"images",
		safeLorePathSegment(itemID),
		fmt.Sprintf("%s-%s", createdAt.Format("20060102-150405"), suffix),
	))
	return filepath.ToSlash(filepath.Join(dir, "image."+ext)), filepath.ToSlash(filepath.Join(dir, "meta.json"))
}

func loreImageMIMEType(ext string) string {
	if ext == "jpeg" {
		return "image/jpeg"
	}
	return "image/png"
}

func BuildLorePrompt(request LoreGenerateRequest) string {
	item := request.Item
	preset := trimRunes(request.ImagePresetPrompt, maxPresetChars)
	brief := trimRunes(item.BriefDescription, maxBriefChars)
	content := trimRunes(item.Content, maxContentChars)
	instruction := trimRunes(request.Instruction, maxInstructionChars)
	var sb strings.Builder
	if preset != "" {
		sb.WriteString("# Image Style Requirements\n\n")
		sb.WriteString(preset)
		sb.WriteString("\n\n")
	}
	sb.WriteString("# Current Lore Image Request\n\n")
	sb.WriteString("Generate one visual reference for this lore item that works as a setting-card preview and creative reference. Emphasize the subject, identity, or rule imagery so the item is recognizable in a lore list. Do not generate text, titles, author names, watermarks, logos, UI panels, or QR codes.\n\n")
	writePromptLine(&sb, "Lore type", item.Type)
	writePromptLine(&sb, "Lore name", item.Name)
	if len(item.Tags) > 0 {
		writePromptLine(&sb, "Tags", strings.Join(item.Tags, ", "))
	}
	if len(item.Keywords) > 0 {
		writePromptLine(&sb, "Keywords", strings.Join(item.Keywords, ", "))
	}
	if brief != "" {
		sb.WriteString("\n## Brief\n\n")
		sb.WriteString(brief)
		sb.WriteString("\n")
	}
	if content != "" {
		sb.WriteString("\n## Lore Body Excerpt\n\n")
		sb.WriteString(content)
		sb.WriteString("\n")
	}
	if instruction != "" {
		sb.WriteString("\n## Additional User Requirements\n\n")
		sb.WriteString(instruction)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

func writePromptLine(sb *strings.Builder, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	sb.WriteString("- ")
	sb.WriteString(key)
	sb.WriteString(": ")
	sb.WriteString(value)
	sb.WriteString("\n")
}

func defaultLoreAltText(item lore.Item) string {
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

func safeLorePathSegment(value string) string {
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
