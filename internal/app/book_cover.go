package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"denova/config"
	"denova/internal/book"
	imageasset "denova/internal/image/asset"
	imagepreset "denova/internal/image/preset"
)

type BookCoverGenerateRequest struct {
	Path          string `json:"path"`
	ImagePresetID string `json:"image_preset_id,omitempty"`
	Instruction   string `json:"instruction,omitempty"`
	ProfileID     string `json:"profile_id,omitempty"`
}

func (a *App) GenerateBookCover(ctx context.Context, request BookCoverGenerateRequest) (imageasset.CoverResult, error) {
	absPath, err := validateBookWorkspacePath(request.Path)
	if err != nil {
		return imageasset.CoverResult{}, err
	}
	cfg, err := a.bookCoverConfig(absPath)
	if err != nil {
		return imageasset.CoverResult{}, err
	}
	meta, err := a.bookMetaStore.Read(absPath)
	if err != nil {
		return imageasset.CoverResult{}, err
	}
	preset, err := resolveBookCoverImagePreset(cfg, request.ImagePresetID)
	if err != nil {
		return imageasset.CoverResult{}, err
	}
	return imageasset.NewService().GenerateCover(ctx, &cfg, book.NewService(absPath), imageasset.CoverGenerateRequest{
		Title:             meta.Title,
		Description:       meta.Description,
		Instruction:       request.Instruction,
		ImagePresetID:     preset.ID,
		ImagePresetPrompt: preset.PromptForTargets(imagepreset.TargetToolRequest),
		ProfileID:         request.ProfileID,
	})
}

func (a *App) UploadBookCover(path, filename string, data []byte) (imageasset.CoverResult, error) {
	absPath, err := validateBookWorkspacePath(path)
	if err != nil {
		return imageasset.CoverResult{}, err
	}
	return imageasset.NewService().UploadCover(book.NewService(absPath), imageasset.CoverUploadRequest{
		Filename: filename,
		Data:     data,
	})
}

func (a *App) ReadBookCover(path string) ([]byte, string, error) {
	absPath, err := validateBookWorkspacePath(path)
	if err != nil {
		return nil, "", err
	}
	absCover, err := book.SafePath(absPath, imageasset.CoverPath)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(absCover)
	if err != nil {
		return nil, "", err
	}
	return data, "image/png", nil
}

func (a *App) bookCoverConfig(workspace string) (config.Config, error) {
	a.mu.RLock()
	novaDir := ""
	if a.cfg != nil {
		novaDir = a.cfg.DataDir()
	}
	a.mu.RUnlock()
	projectConfigPath := ""
	if layout, layoutErr := a.projectLayoutForWorkspace(workspace); layoutErr == nil {
		projectConfigPath = layout.ConfigPath()
	}
	layered, err := config.LoadLayeredWithStartupConfigAt(novaDir, workspace, projectConfigPath)
	if err != nil {
		return config.Config{}, err
	}
	effective := layered.Effective
	cfg := config.Config{
		ImageAPIKey:              effective.ImageAPIKey,
		ImageAPIBaseURL:          effective.ImageAPIBaseURL,
		ImageAPIModel:            effective.ImageAPIModel,
		DefaultImageAPIProfileID: effective.DefaultImageAPIProfileID,
		ImageAPIProfiles:         effective.ImageAPIProfiles,
		DenovaDir:                layered.Paths.DenovaDir,
		NovaDir:                  layered.Paths.DenovaDir,
		Workspace:                workspace,
		IDEImagePresetID:         effective.IDEImagePresetID,
	}
	if v := os.Getenv("OPENAI_IMAGE_API_KEY"); v != "" {
		cfg.ImageAPIKey = v
	}
	if v := os.Getenv("OPENAI_IMAGE_BASE_URL"); v != "" {
		cfg.ImageAPIBaseURL = v
	}
	if v := os.Getenv("OPENAI_IMAGE_MODEL"); v != "" {
		cfg.ImageAPIModel = v
	}
	return cfg, nil
}

func resolveBookCoverImagePreset(cfg config.Config, requestedID string) (imagepreset.Preset, error) {
	presetID := imagepreset.NormalizeID(requestedID)
	if presetID == "" {
		presetID = imagepreset.NormalizeID(cfg.IDEImagePresetID)
	}
	if presetID == "" {
		presetID = imagepreset.DefaultID
	}
	if strings.TrimSpace(cfg.DataDir()) == "" {
		return imagepreset.DefaultPreset(), nil
	}
	preset, err := imagepreset.NewLibrary(cfg.DataDir()).Get(presetID)
	if err != nil {
		return imagepreset.Preset{}, err
	}
	return preset, nil
}

func validateBookWorkspacePath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("路径无效: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("目录不存在: %s", absPath)
	}
	if !isBookWorkspace(absPath) {
		return "", fmt.Errorf("不是有效的书籍工作区: %s", absPath)
	}
	return absPath, nil
}

func bookCoverUpdatedAt(workspace string) string {
	if strings.TrimSpace(workspace) == "" {
		return ""
	}
	absCover, err := book.SafePath(workspace, imageasset.CoverPath)
	if err != nil {
		return ""
	}
	info, err := os.Stat(absCover)
	if err != nil || info.IsDir() {
		return ""
	}
	return info.ModTime().UTC().Format(time.RFC3339Nano)
}
