package bookapp

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
	projectdomain "denova/internal/project"
)

type CoverGenerateRequest struct {
	Path          string `json:"path"`
	Mode          string `json:"mode,omitempty"`
	CommandID     string `json:"command_id,omitempty"`
	Prompt        string `json:"prompt,omitempty"`
	ImagePresetID string `json:"image_preset_id,omitempty"`
	Instruction   string `json:"instruction,omitempty"`
	ProfileID     string `json:"profile_id,omitempty"`
}

func (service *Service) ProjectIDForPath(path string) (string, error) {
	if service == nil || service.registry == nil {
		return "", fmt.Errorf("project registry is unavailable")
	}
	record, _, err := service.registry.ResolveByPath(path, true)
	if err != nil {
		return "", err
	}
	return record.ID, nil
}

func (service *Service) GenerateCover(ctx context.Context, request CoverGenerateRequest) (imageasset.CoverResult, error) {
	absPath, err := validateWorkspacePath(request.Path)
	if err != nil {
		return imageasset.CoverResult{}, err
	}
	cfg, err := service.coverConfig(absPath)
	if err != nil {
		return imageasset.CoverResult{}, err
	}
	meta, err := service.metadata.Read(absPath)
	if err != nil {
		return imageasset.CoverResult{}, err
	}
	preset := imagepreset.Preset{}
	if strings.TrimSpace(request.Prompt) == "" {
		preset, err = resolveBookCoverImagePreset(cfg, request.ImagePresetID)
		if err != nil {
			return imageasset.CoverResult{}, err
		}
	}
	return imageasset.NewService().GenerateCover(ctx, &cfg, book.NewService(absPath), imageasset.CoverGenerateRequest{
		Title:             meta.Title,
		Description:       meta.Description,
		Prompt:            request.Prompt,
		Instruction:       request.Instruction,
		ImagePresetID:     preset.ID,
		ImagePresetPrompt: preset.PromptForTargets(imagepreset.TargetToolRequest),
		ProfileID:         request.ProfileID,
	})
}

func (service *Service) UploadCover(path, filename string, data []byte) (imageasset.CoverResult, error) {
	absPath, err := validateWorkspacePath(path)
	if err != nil {
		return imageasset.CoverResult{}, err
	}
	return imageasset.NewService().UploadCover(book.NewService(absPath), imageasset.CoverUploadRequest{
		Filename: filename,
		Data:     data,
	})
}

func (service *Service) ReadCover(path string) ([]byte, string, error) {
	absPath, err := validateWorkspacePath(path)
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

func (service *Service) coverConfig(workspace string) (config.Config, error) {
	novaDir := service.dataDir
	projectConfigPath := ""
	if service.registry != nil {
		if record, found, findErr := service.registry.FindByPath(workspace, false); findErr == nil && found {
			if layout, layoutErr := service.registry.Layout(record); layoutErr == nil {
				projectConfigPath = layout.ConfigPath()
			}
		}
	}
	layered, err := config.LoadLayeredWithStartupConfigAt(novaDir, workspace, projectConfigPath)
	if err != nil {
		return config.Config{}, err
	}
	effective := layered.Effective
	cfg := config.Config{
		DefaultImageAPIProfileID: effective.DefaultImageAPIProfileID,
		ImageAPIEndpoints:        effective.ImageAPIEndpoints,
		ImageAPIProfiles:         effective.ImageAPIProfiles,
		DenovaDir:                layered.Paths.DenovaDir,
		NovaDir:                  layered.Paths.DenovaDir,
		Workspace:                workspace,
		IDEImagePresetID:         effective.IDEImagePresetID,
	}
	config.ApplyImageAPIEnvironment(&cfg)
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

func validateWorkspacePath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path / 路径无效: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("directory does not exist / 目录不存在: %s", absPath)
	}
	projectType, err := projectdomain.DetectType(absPath)
	if err != nil || projectType != projectdomain.TypeBook {
		return "", fmt.Errorf("not a valid book workspace / 不是有效的书籍工作区: %s", absPath)
	}
	return absPath, nil
}

func CoverUpdatedAt(workspace string) string {
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
