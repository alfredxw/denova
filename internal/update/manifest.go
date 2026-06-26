package update

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	manifestFileName       = "manifest.json"
	pendingManifestRefName = "pending-manifest.json"
	applyLogFileName       = "apply.log"
)

// ApplyManifest is the updater contract written by Nova and consumed by
// nova-updater. Paths are absolute so the updater can run after Nova exits.
type ApplyManifest struct {
	SourceDir         string   `json:"source_dir"`
	InstallDir        string   `json:"install_dir"`
	BackupDir         string   `json:"backup_dir"`
	CurrentPID        int      `json:"current_pid"`
	TargetExecutable  string   `json:"target_executable"`
	UpdaterExecutable string   `json:"updater_executable"`
	RelaunchArgs      []string `json:"relaunch_args"`
	Version           string   `json:"version"`
	LogPath           string   `json:"log_path"`
}

type pendingManifestRef struct {
	ManifestPath string `json:"manifest_path"`
}

func writeManifest(path string, manifest ApplyManifest) error {
	if err := writeJSONFile(path, manifest, 0o644); err != nil {
		return fmt.Errorf("写入更新清单失败: %w", err)
	}
	return nil
}

func readManifest(path string) (ApplyManifest, error) {
	var manifest ApplyManifest
	if err := readJSONFile(path, &manifest); err != nil {
		return ApplyManifest{}, fmt.Errorf("读取更新清单失败: %w", err)
	}
	return manifest, nil
}

func writePendingManifestRef(updateDir, manifestPath string) error {
	path := filepath.Join(updateDir, pendingManifestRefName)
	if err := writeJSONFile(path, pendingManifestRef{ManifestPath: manifestPath}, 0o644); err != nil {
		return fmt.Errorf("记录待应用更新失败: %w", err)
	}
	return nil
}

func readPendingManifestRef(updateDir string) (string, error) {
	path := filepath.Join(updateDir, pendingManifestRefName)
	var ref pendingManifestRef
	if err := readJSONFile(path, &ref); err != nil {
		return "", fmt.Errorf("读取待应用更新失败: %w", err)
	}
	if ref.ManifestPath == "" {
		return "", fmt.Errorf("待应用更新记录缺少 manifest_path")
	}
	return ref.ManifestPath, nil
}

func writeJSONFile(path string, value any, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, mode)
}

func readJSONFile(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
