package app

import (
	"context"
	"log"

	"nova/config"
	"nova/internal/agent"
)

// InferNovelSplitRegex runs the model-only Tool Agent for novel import chapter splitting.
func (a *App) InferNovelSplitRegex(ctx context.Context, sample string) (string, error) {
	runtimeCfg, workspace := a.toolAgentConfig()
	regex, err := agent.InferChapterSplitRegex(ctx, &runtimeCfg, sample)
	if err != nil {
		log.Printf("[tool-agent] 小说导入章节正则推断失败 workspace=%s err=%v", workspace, err)
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
	novaDir := runtimeCfg.NovaDir
	a.mu.RUnlock()

	runtimeCfg.Workspace = workspace
	if layered, err := config.LoadLayered(novaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
	} else {
		log.Printf("[tool-agent] 加载分层配置失败 workspace=%s err=%v", workspace, err)
	}
	return runtimeCfg, workspace
}
