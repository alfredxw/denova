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

type modelHost struct{ app *App }

func (host modelHost) ModelConfigSnapshot() config.Config {
	if host.app == nil {
		return config.Config{}
	}
	host.app.mu.RLock()
	defer host.app.mu.RUnlock()
	if host.app.cfg == nil {
		return config.Config{}
	}
	snapshot := *host.app.cfg
	snapshot.ModelProfiles = cloneModelProfiles(host.app.cfg.ModelProfiles)
	return snapshot
}

func cloneModelProfiles(profiles []config.ModelProfileSettings) []config.ModelProfileSettings {
	if profiles == nil {
		return nil
	}
	cloned := make([]config.ModelProfileSettings, len(profiles))
	for index, profile := range profiles {
		cloned[index] = profile
		if profile.Headers != nil {
			cloned[index].Headers = make(map[string]string, len(profile.Headers))
			for name, value := range profile.Headers {
				cloned[index].Headers[name] = value
			}
		}
		if profile.ProtocolOptions != nil {
			cloned[index].ProtocolOptions = cloneModelProtocolOptions(profile.ProtocolOptions)
		}
	}
	return cloned
}

func cloneModelProtocolOptions(options map[string]any) map[string]any {
	cloned := make(map[string]any, len(options))
	for key, value := range options {
		cloned[key] = cloneModelProtocolValue(value)
	}
	return cloned
}

func cloneModelProtocolValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneModelProtocolOptions(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneModelProtocolValue(item)
		}
		return cloned
	default:
		return typed
	}
}
