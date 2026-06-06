package agent

import (
	"context"
	"fmt"
	"log"
	"strings"

	localbk "github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	filesystemmw "github.com/cloudwego/eino/adk/middlewares/filesystem"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/adk/prebuilt/deep"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"

	"nova/config"
	"nova/internal/book"
	"nova/internal/prompts"
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
	return buildDeepAgent(ctx, cfg, config.AgentKindIDE, "NovaAgent", "AI 小说创作助手", BuildInstruction(cfg, state, teller), true, false, nil, loreTools, nil)
}

func BuildInteractiveStory(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput) (adk.Agent, error) {
	toolSettings := config.ResolveAgentTools(cfg, config.AgentKindInteractiveStory)
	var loreTools []tool.BaseTool
	if toolSettings.LoreRead {
		var err error
		loreTools, err = newLoreTools(cfg.Workspace, false)
		if err != nil {
			return nil, err
		}
	}
	return buildDeepAgent(ctx, cfg, config.AgentKindInteractiveStory, "NovaInteractiveStoryAgent", "AI 互动故事叙事助手", BuildInteractiveStoryInstruction(cfg, state, teller), false, true, []adk.ChatModelAgentMiddleware{
		newInteractiveStoryToolMiddleware(),
	}, loreTools, interactiveMaxTokens(cfg))
}

func buildDeepAgent(
	ctx context.Context,
	cfg *config.Config,
	agentKind string,
	name string,
	description string,
	instruction string,
	enableSkills bool,
	disableWriteTodos bool,
	extraHandlers []adk.ChatModelAgentMiddleware,
	extraTools []tool.BaseTool,
	maxTokens *int,
) (adk.Agent, error) {
	modelCfg := chatModelConfigForAgent(cfg, agentKind)
	modelCfg.MaxTokens = maxTokens
	toolSettings := config.ResolveAgentTools(cfg, agentKind)
	cm, err := openai.NewChatModel(ctx, &modelCfg)
	if err != nil {
		return nil, fmt.Errorf("创建模型失败: %w", err)
	}

	backend, err := localbk.NewBackend(ctx, &localbk.Config{})
	if err != nil {
		return nil, fmt.Errorf("创建 backend 失败: %w", err)
	}

	var handlers []adk.ChatModelAgentMiddleware
	filesystemHandler, err := newFilesystemMiddleware(ctx, backend, toolSettings)
	if err != nil {
		return nil, err
	}
	if filesystemHandler != nil {
		handlers = append(handlers, filesystemHandler)
	}
	if enableSkills && toolSettings.Skills {
		if skillsDir := ResolveSkillsDir(cfg.SkillsDir); skillsDir != "" {
			skillBackend, sbErr := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
				Backend: backend,
				BaseDir: skillsDir,
			})
			if sbErr == nil {
				skillMw, smErr := skill.NewMiddleware(ctx, &skill.Config{Backend: skillBackend})
				if smErr == nil {
					handlers = append(handlers, skillMw)
				}
			}
		}
	}
	handlers = append(handlers, extraHandlers...)
	handlers = append(handlers, &safeToolMiddleware{})

	return deep.New(ctx, &deep.Config{
		Name:              name,
		Description:       description,
		ChatModel:         cm,
		Instruction:       instruction,
		WithoutWriteTodos: disableWriteTodos || !toolSettings.Todo,
		MaxIteration:      configMaxIteration(cfg),
		Handlers:          handlers,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: extraTools,
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

func newFilesystemMiddleware(ctx context.Context, backend *localbk.Local, settings config.ResolvedAgentToolSettings) (adk.ChatModelAgentMiddleware, error) {
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
		mwConfig.StreamingShell = backend
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
