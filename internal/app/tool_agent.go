package app

import (
	"context"
	"fmt"
	"log/slog"

	"denova/config"
	agentmodeltask "denova/internal/agents/modeltask"
	appsettings "denova/internal/app/settings"
)

// InferNovelSplitRegex runs the model-only Tool Agent for novel import chapter splitting.
func (a *App) InferNovelSplitRegex(ctx context.Context, sample string) (string, error) {
	runtimeCfg, workspace := a.toolAgentConfig()
	regex, err := agentmodeltask.InferChapterSplitRegex(ctx, &runtimeCfg, sample)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[tool-agent] Novel import chapter regex inference failed workspace=%s err=%v", workspace, err))
		a.persistAgentCall(config.AgentKindToolAgent, sample, "执行失败："+err.Error())
		return "", err
	}
	a.persistAgentCall(config.AgentKindToolAgent, sample, regex)
	return regex, nil
}

func (a *App) toolAgentConfig() (config.Config, string) {
	a.mu.RLock()
	var runtimeCfg config.Config
	if a.cfg != nil {
		runtimeCfg = *a.cfg
	}
	workspace := a.workspace
	novaDir := runtimeCfg.DataDir()
	a.mu.RUnlock()

	runtimeCfg.Workspace = workspace
	if layered, err := config.LoadLayeredWithStartupConfigAt(
		novaDir, workspace, config.ProjectConfigPath(runtimeCfg.ProjectStoreDir),
	); err == nil {
		appsettings.ApplyLayered(&runtimeCfg, layered)
	} else {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[tool-agent] Failed to load layered settings workspace=%s err=%v", workspace, err))
	}
	return runtimeCfg, workspace
}
