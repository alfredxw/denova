package book

import "nova/internal/book/versions"

const (
	DefaultTimedVersionIntervalMinutes = versions.DefaultTimedVersionIntervalMinutes
	DefaultAgentVersionCharThreshold   = versions.DefaultAgentVersionCharThreshold
	DefaultAutoVersionRetention        = versions.DefaultAutoVersionRetention
)

const (
	VersionSourceManual         = versions.VersionSourceManual
	VersionSourceTimer          = versions.VersionSourceTimer
	VersionSourceAgent          = versions.VersionSourceAgent
	VersionSourceRollbackBackup = versions.VersionSourceRollbackBackup
)

var (
	ErrVersionNotFound = versions.ErrVersionNotFound
	ErrVersionClean    = versions.ErrVersionClean
)

type VersionEntry = versions.VersionEntry
type VersionStatus = versions.VersionStatus
type VersionAutoInfo = versions.VersionAutoInfo
type VersionChange = versions.VersionChange
type VersionCommandResult = versions.VersionCommandResult
type VersionDiff = versions.VersionDiff
type VersionAutoSettings = versions.VersionAutoSettings
type VersionAutoResult = versions.VersionAutoResult
type VersionWorkspaceState = versions.VersionWorkspaceState
type VersionFileState = versions.VersionFileState
type VersionService = versions.Service

func NewVersionService(workspace string) *VersionService {
	return versions.NewService(workspace)
}

func DefaultVersionAutoSettings() VersionAutoSettings {
	return versions.DefaultAutoSettings()
}
