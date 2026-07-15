package app

import (
	"context"
	"encoding/json"
	"log"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
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

// ClassifyLoreItems runs the reusable model-only semantic classifier used by
// character-card import and the manual lore organization preview.
func (a *App) ClassifyLoreItems(ctx context.Context, inputs []book.LoreClassificationInput) ([]book.LoreClassificationSuggestion, error) {
	runtimeCfg, workspace := a.toolAgentConfig()
	result, err := agent.ClassifyLoreItems(ctx, &runtimeCfg, inputs)
	inputJSON, _ := json.Marshal(inputs)
	if err != nil {
		log.Printf("[tool-agent] 资料语义分类失败 workspace=%s items=%d err=%v", workspace, len(inputs), err)
		a.persistAgentCall(config.AgentKindToolAgent, string(inputJSON), "执行失败："+err.Error())
		return nil, err
	}
	outputJSON, _ := json.Marshal(result)
	a.persistAgentCall(config.AgentKindToolAgent, string(inputJSON), string(outputJSON))
	return result, nil
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
	if layered, err := config.LoadLayeredWithStartupConfig(novaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
	} else {
		log.Printf("[tool-agent] 加载分层配置失败 workspace=%s err=%v", workspace, err)
	}
	return runtimeCfg, workspace
}
