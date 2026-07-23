package book

import "denova/internal/book/versions"

const (
	DefaultTimedVersionIntervalMinutes = versions.DefaultTimedVersionIntervalMinutes
	DefaultAutoVersionRetention        = versions.DefaultAutoVersionRetention
)

const (
	VersionSourceManual         = versions.VersionSourceManual
	VersionSourceTimer          = versions.VersionSourceTimer
	VersionSourceAgent          = versions.VersionSourceAgent
	VersionSourceRollbackBackup = versions.VersionSourceRollbackBackup
)

const (
	VersionRestoreScopeWorkspace = versions.VersionRestoreScopeWorkspace
	VersionRestoreScopePaths     = versions.VersionRestoreScopePaths
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
type VersionRestorePlan = versions.VersionRestorePlan
type VersionRestoreResult = versions.VersionRestoreResult
type VersionRestoreChange = versions.VersionRestoreChange
type VersionDiff = versions.VersionDiff
type VersionAutoSettings = versions.VersionAutoSettings
type VersionAutoResult = versions.VersionAutoResult
type VersionService = versions.Service

func NewVersionService(workspace string) *VersionService {
	return versions.NewService(workspace)
}

func DefaultVersionAutoSettings() VersionAutoSettings {
	return versions.DefaultAutoSettings()
}
