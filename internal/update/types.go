package update

import "time"

type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	DownloadURL        string `json:"download_url"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type CheckResult struct {
	CurrentVersion  string    `json:"current_version"`
	LatestVersion   string    `json:"latest_version"`
	UpdateAvailable bool      `json:"update_available"`
	CanInstall      bool      `json:"can_install"`
	Platform        string    `json:"platform"`
	ReleaseURL      string    `json:"release_url"`
	PublishedAt     time.Time `json:"published_at"`
	ReleaseNotes    string    `json:"release_notes,omitempty"`
	Asset           *Asset    `json:"asset,omitempty"`
	Message         string    `json:"message,omitempty"`
}

type InstallResult struct {
	PreviousVersion  string `json:"previous_version"`
	InstalledVersion string `json:"installed_version"`
	Installed        bool   `json:"installed"`
	RestartRequired  bool   `json:"restart_required"`
	BackupPath       string `json:"backup_path,omitempty"`
	StagedPath       string `json:"staged_path,omitempty"`
	Message          string `json:"message,omitempty"`
}

type InstallProgress struct {
	Phase           string  `json:"phase"`
	AssetName       string  `json:"asset_name,omitempty"`
	ArchivePath     string  `json:"archive_path,omitempty"`
	DownloadedBytes int64   `json:"downloaded_bytes,omitempty"`
	TotalBytes      int64   `json:"total_bytes,omitempty"`
	Percent         float64 `json:"percent,omitempty"`
	Message         string  `json:"message,omitempty"`
}
