package app

import (
	"denova/config"
	agentrun "denova/internal/agents/run"
	novaskills "denova/internal/agents/skills"
	appsettings "denova/internal/app/settings"
)

type resourceCatalogHost struct{ app *App }

func (host resourceCatalogHost) SkillDirectories() []novaskills.Directory {
	if host.app == nil {
		return nil
	}
	host.app.mu.RLock()
	defer host.app.mu.RUnlock()
	if host.app.cfg == nil {
		return nil
	}
	return novaskills.NewDirectories(host.app.cfg.SkillsDir, host.app.cfg.DataDir(), host.app.workspace)
}

type settingsHost struct{ app *App }

func (host settingsHost) SettingsRuntime() appsettings.Runtime {
	if host.app == nil {
		return appsettings.Runtime{}
	}
	host.app.mu.RLock()
	runtime := appsettings.Runtime{Workspace: host.app.workspace, BookState: host.app.bookState}
	if host.app.cfg != nil {
		runtime.Config = *host.app.cfg
		runtime.ProjectConfigPath = config.ProjectConfigPath(host.app.cfg.ProjectStateDir)
	}
	registry := host.app.projectRegistry
	host.app.mu.RUnlock()

	if runtime.ProjectConfigPath == "" && runtime.Workspace != "" && registry != nil {
		if _, layout, err := registry.ResolveByPath(runtime.Workspace, true); err == nil {
			runtime.ProjectConfigPath = layout.ConfigPath()
		}
	}
	return runtime
}

func (host settingsHost) ApplySettings(layered config.LayeredSettings, layer config.SettingsLayer) {
	if host.app == nil {
		return
	}
	host.app.mu.Lock()
	appsettings.ApplyLayered(host.app.cfg, layered)
	syncRuntimeDiagnostics(host.app.cfg)
	versionService := host.app.versionService
	autoSettings := versionAutoSettingsForConfig(host.app.cfg)
	host.app.mu.Unlock()

	if layer == config.SettingsLayerUser && versionService != nil {
		versionService.ConfigureAutoVersion(autoSettings)
	}
}

func syncRuntimeDiagnostics(cfg *config.Config) {
	if cfg == nil {
		agentrun.SetModelInputLoggingEnabled(false)
		return
	}
	agentrun.SetModelInputLoggingEnabled(cfg.DevMode && cfg.LLMInputLogEnabled)
	agentrun.SetTraceRuntimeConfig(cfg.TraceCaptureLevel, cfg.TraceExporter, cfg.TraceRetentionRuns)
}
