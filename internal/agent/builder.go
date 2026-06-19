package agent

import (
	"context"
	"fmt"
	"log"
	"strings"

	localbk "github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/filesystem"
	filesystemmw "github.com/cloudwego/eino/adk/middlewares/filesystem"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/adk/prebuilt/deep"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"

	"nova/config"
	"nova/internal/book"
	"nova/internal/prompts"
	novaskills "nova/internal/skills"
)

// Build 构建小说创作 Agent（deep agent + 文件系统工具 + Skill 中间件）。
func Build(ctx context.Context, cfg *config.Config, state *book.State, teller IDEStoryTeller) (adk.Agent, error) {
	toolSettings := config.ResolveAgentTools(cfg, config.AgentKindIDE)
	var loreTools []tool.BaseTool
	if toolSettings.LoreRead {
		var err error
		loreTools, err = newLoreTools(cfg.Workspace, toolSettings.LoreWrite)
		if err != nil {
			return nil, err
		}
	}
	return buildDeepAgent(ctx, cfg, deepAgentSpec{
		Kind:         config.AgentKindIDE,
		Name:         "NovaAgent",
		Description:  "AI 小说创作助手",
		Instruction:  BuildInstruction(cfg, state, teller),
		EnableSkills: true,
		ExtraTools:   loreTools,
	})
}

func BuildInteractiveStory(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput, toolContexts ...InteractiveStoryToolContext) (adk.Agent, error) {
	toolSettings := config.ResolveAgentTools(cfg, config.AgentKindInteractiveStory)
	var extraTools []tool.BaseTool
	if toolSettings.LoreRead {
		loreTools, err := newLoreTools(cfg.Workspace, false)
		if err != nil {
			return nil, err
		}
		extraTools = append(extraTools, loreTools...)
	}
	if len(toolContexts) > 0 {
		memoryTools, err := newInteractiveMemoryTools(toolContexts[0])
		if err != nil {
			return nil, err
		}
		extraTools = append(extraTools, memoryTools...)
	}
	return buildDeepAgent(ctx, cfg, deepAgentSpec{
		Kind:              config.AgentKindInteractiveStory,
		Name:              "NovaInteractiveStoryAgent",
		Description:       "AI 互动故事叙事助手",
		Instruction:       BuildInteractiveStoryInstruction(cfg, state, teller),
		EnableSkills:      true,
		DisableWriteTodos: true,
		ExtraTools:        extraTools,
		MaxTokens:         interactiveMaxTokens(cfg),
	})
}

// BuildConfigManagerAgent 构建统一配置管理 Agent（deep agent + 通用工具 + Skill + 模块资源工具）。
func BuildConfigManagerAgent(ctx context.Context, cfg *config.Config, state *book.State) (adk.Agent, error) {
	toolSettings := config.ResolveAgentTools(cfg, config.AgentKindConfigManager)
	var extraTools []tool.BaseTool
	if toolSettings.LoreRead {
		var err error
		loreTools, err := newLoreTools(cfg.Workspace, toolSettings.LoreWrite)
		if err != nil {
			return nil, err
		}
		extraTools = append(extraTools, loreTools...)
	}
	configTools, err := newConfigManagerTools(cfg)
	if err != nil {
		return nil, err
	}
	extraTools = append(extraTools, configTools...)
	return buildDeepAgent(ctx, cfg, deepAgentSpec{
		Kind:         config.AgentKindConfigManager,
		Name:         "NovaConfigManagerAgent",
		Description:  "AI 配置与资源管理助手",
		Instruction:  BuildConfigManagerInstruction(cfg, state),
		EnableSkills: true,
		ExtraTools:   extraTools,
	})
}

// BuildAutomationAgent 构建后台自动化 Agent。工具权限由调用方按任务写入策略提前收敛到 cfg.AgentTools.Automation。
func BuildAutomationAgent(ctx context.Context, cfg *config.Config, state *book.State, task AutomationTaskInstruction) (adk.Agent, error) {
	toolSettings := config.ResolveAgentTools(cfg, config.AgentKindAutomation)
	var loreTools []tool.BaseTool
	if toolSettings.LoreRead {
		var err error
		loreTools, err = newLoreTools(cfg.Workspace, toolSettings.LoreWrite)
		if err != nil {
			return nil, err
		}
	}
	return buildDeepAgent(ctx, cfg, deepAgentSpec{
		Kind:         config.AgentKindAutomation,
		Name:         "NovaAutomationAgent",
		Description:  "AI 自动化任务助手",
		Instruction:  BuildAutomationInstruction(cfg, state, task),
		EnableSkills: true,
		ExtraTools:   loreTools,
	})
}

type deepAgentSpec struct {
	Kind              string
	Name              string
	Description       string
	Instruction       string
	EnableSkills      bool
	DisableWriteTodos bool
	ExtraHandlers     []adk.ChatModelAgentMiddleware
	ExtraTools        []tool.BaseTool
	MaxTokens         *int
}

func buildDeepAgent(ctx context.Context, cfg *config.Config, spec deepAgentSpec) (adk.Agent, error) {
	modelCfg := chatModelConfigForAgent(cfg, spec.Kind)
	modelCfg.MaxTokens = spec.MaxTokens
	toolSettings := config.ResolveAgentTools(cfg, spec.Kind)
	cm, err := openai.NewChatModel(ctx, &modelCfg)
	if err != nil {
		return nil, fmt.Errorf("创建模型失败: %w", err)
	}

	localBackend, err := localbk.NewBackend(ctx, &localbk.Config{})
	if err != nil {
		return nil, fmt.Errorf("创建 backend 失败: %w", err)
	}
	backend := newAgentFilesystemBackend(localBackend)

	var handlers []adk.ChatModelAgentMiddleware
	filesystemHandler, err := newFilesystemMiddleware(ctx, backend, localBackend, toolSettings)
	if err != nil {
		return nil, err
	}
	if filesystemHandler != nil {
		handlers = append(handlers, filesystemHandler)
	}
	if spec.EnableSkills && toolSettings.Skills {
		skillBackend := novaskills.NewAgentBackend(
			novaskills.NewDirectories(cfg.SkillsDir, cfg.NovaDir, cfg.Workspace),
			spec.Kind,
			config.ResolveAgentSkillOverrides(cfg, spec.Kind),
		)
		availableSkills, listErr := skillBackend.List(ctx)
		if listErr != nil {
			log.Printf("[agent] 加载 Skills 列表失败 agent=%s err=%v", spec.Kind, listErr)
		} else if len(availableSkills) > 0 {
			skillMw, smErr := skill.NewMiddleware(ctx, &skill.Config{Backend: skillBackend})
			if smErr != nil {
				log.Printf("[agent] 创建 Skill middleware 失败 agent=%s err=%v", spec.Kind, smErr)
			} else {
				handlers = append(handlers, skillMw)
			}
		}
	}
	tools := append([]tool.BaseTool{}, spec.ExtraTools...)
	if toolSettings.WebSearch {
		webSearchTools, wsErr := newWebSearchTools()
		if wsErr != nil {
			return nil, wsErr
		}
		tools = append(tools, webSearchTools...)
	}
	handlers = append(handlers, spec.ExtraHandlers...)
	handlers = append(handlers, &toolOrchestratorMiddleware{agentKind: spec.Kind})

	return deep.New(ctx, &deep.Config{
		Name:              spec.Name,
		Description:       spec.Description,
		ChatModel:         cm,
		Instruction:       spec.Instruction,
		WithoutWriteTodos: spec.DisableWriteTodos || !toolSettings.Todo,
		MaxIteration:      configMaxIteration(cfg),
		Handlers:          handlers,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools,
				// 当 LLM 幻觉出不存在的工具时，把错误信息以 ToolMessage 形式回传，
				// 让 Agent 在下一轮自行修正工具名或改用其他方案，避免整次任务被 NodeRunError 中断。
				UnknownToolsHandler: handleUnknownTool,
			},
		},
		ModelRetryConfig: &adk.ModelRetryConfig{
			MaxRetries: configModelMaxRetries(cfg),
			IsRetryAble: func(_ context.Context, err error) bool {
				return strings.Contains(err.Error(), "429") ||
					strings.Contains(err.Error(), "Too Many Requests") ||
					strings.Contains(err.Error(), "qpm limit")
			},
		},
	})
}

func newFilesystemMiddleware(ctx context.Context, backend filesystem.Backend, streamingShell filesystem.StreamingShell, settings config.ResolvedAgentToolSettings) (adk.ChatModelAgentMiddleware, error) {
	if backend == nil {
		return nil, nil
	}
	if !settings.FileRead && !settings.FileWrite && !settings.ShellExecute {
		return nil, nil
	}
	mwConfig := &filesystemmw.MiddlewareConfig{
		Backend: backend,
		LsToolConfig: &filesystemmw.ToolConfig{
			Disable: !settings.FileRead,
		},
		ReadFileToolConfig: &filesystemmw.ToolConfig{
			Disable: !settings.FileRead,
		},
		GlobToolConfig: &filesystemmw.ToolConfig{
			Disable: !settings.FileRead,
		},
		GrepToolConfig: &filesystemmw.ToolConfig{
			Disable: !settings.FileRead,
		},
		WriteFileToolConfig: &filesystemmw.ToolConfig{
			Disable: !settings.FileWrite,
		},
		EditFileToolConfig: &filesystemmw.ToolConfig{
			Disable: !settings.FileWrite,
		},
	}
	if settings.ShellExecute {
		mwConfig.StreamingShell = streamingShell
	}
	return filesystemmw.New(ctx, mwConfig)
}

func configMaxIteration(cfg *config.Config) int {
	if cfg == nil || cfg.MaxIteration <= 0 {
		return 50
	}
	return cfg.MaxIteration
}

func configModelMaxRetries(cfg *config.Config) int {
	if cfg == nil || cfg.ModelMaxRetries < 0 {
		return 5
	}
	return cfg.ModelMaxRetries
}

func interactiveMaxTokens(cfg *config.Config) *int {
	if cfg == nil || cfg.InteractiveMaxTokens <= 0 {
		return nil
	}
	tokens := cfg.InteractiveMaxTokens
	return &tokens
}

// handleUnknownTool 拦截 LLM 调用未知工具的错误，把可读提示作为工具结果回传给模型，
// 引导 Agent 在后续轮次基于该反馈自我修正（例如改用正确的工具名）。
func handleUnknownTool(_ context.Context, name, input string) (string, error) {
	log.Printf("[agent] LLM 调用了不存在的工具 name=%s args=%s", name, input)
	return prompts.UnknownToolMessage(name), nil
}
