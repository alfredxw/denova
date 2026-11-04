package desktop

import (
	"context"
	"time"

	"denova/internal/update"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
)

// The official plugin owns checking, signed installation, progress and restart.
// Its preferences live beside the shell's settings, outside portable user data.
type UpdatePreferences struct {
	AutomaticChecks    bool `json:"automaticChecks"`
	AutomaticDownloads bool `json:"automaticDownloads"`
}

func (b *Bridge) OpenUpdates(ctx context.Context) error {
	if err := b.authorize(ctx); err != nil {
		return err
	}
	updater.CheckForUpdates()
	return nil
}
func (b *Bridge) UpdatePreferences(ctx context.Context) (UpdatePreferences, error) {
	if err := b.authorize(ctx); err != nil {
		return UpdatePreferences{}, err
	}
	return UpdatePreferences{updater.AutomaticChecks(), updater.AutomaticDownloads()}, nil
}
func (b *Bridge) SetUpdatePreferences(ctx context.Context, p UpdatePreferences) error {
	if err := b.authorize(ctx); err != nil {
		return err
	}
	updater.SetAutomaticDownloads(p.AutomaticDownloads)
	updater.SetAutomaticChecks(p.AutomaticChecks)
	return nil
}
func (b *Bridge) UpdateStatus(ctx context.Context) (update.Status, error) {
	if err := b.authorize(ctx); err != nil {
		return update.Status{}, err
	}
	b.shell.preferencesMu.Lock()
	completed := b.shell.preferences.CompletedUpdate
	b.shell.preferencesMu.Unlock()
	status := update.Status{Distribution: "desktop", Phase: "idle", CurrentVersion: mygo.App.Version(), CheckedAt: updater.LastCheck(), CompletedUpdate: completed}
	if version := updater.AvailableVersion(); version != "" {
		status.Check = &update.CheckResult{CurrentVersion: status.CurrentVersion, LatestVersion: version, UpdateAvailable: true, ReleaseURL: "https://github.com/alfredxw/denova/releases/tag/v" + version, PublishedAt: time.Time{}}
	}
	return status, nil
}
