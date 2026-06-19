package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	"nova/config"
	"nova/internal/session"
)

const interactiveMemoryAgentMaxAttempts = 3

type interactiveMemoryOutputGenerator func(context.Context, *config.Config, string) (string, error)

func runInteractiveMemoryAgentWithRetry(ctx context.Context, cfg *config.Config, instruction string, sessionStore *session.Store, generate interactiveMemoryOutputGenerator, apply func(interactiveMemoryAgentResult) error) (interactiveMemoryAgentResult, error) {
	var lastErr error
	for attempt := 1; attempt <= interactiveMemoryAgentMaxAttempts; attempt++ {
		attemptInstruction := interactiveMemoryInstructionForAttempt(instruction, attempt, lastErr)
		output, err := generate(ctx, cfg, attemptInstruction)
		if err != nil {
			lastErr = err
			log.Printf("[interactive-memory-agent] attempt failed phase=generate attempt=%d/%d err=%v", attempt, interactiveMemoryAgentMaxAttempts, err)
			persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveState, attemptInstruction, interactiveMemoryAttemptFailure(attempt, err))
			continue
		}
		persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveState, attemptInstruction, output)

		result, err := parseInteractiveMemoryOutput(output)
		if err != nil {
			lastErr = err
			log.Printf("[interactive-memory-agent] attempt failed phase=parse attempt=%d/%d err=%v output=%q", attempt, interactiveMemoryAgentMaxAttempts, err, output)
			continue
		}
		if apply != nil {
			if err := apply(result); err != nil {
				lastErr = err
				log.Printf("[interactive-memory-agent] attempt failed phase=apply attempt=%d/%d err=%v", attempt, interactiveMemoryAgentMaxAttempts, err)
				continue
			}
		}
		return result, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("未知错误")
	}
	return interactiveMemoryAgentResult{}, fmt.Errorf("互动记忆 Agent 重试 %d 次仍失败: %w", interactiveMemoryAgentMaxAttempts, lastErr)
}

func interactiveMemoryInstructionForAttempt(instruction string, attempt int, previousErr error) string {
	if attempt <= 1 || previousErr == nil {
		return instruction
	}
	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(instruction))
	sb.WriteString("\n\n## 上次输出失败，需要修正后重试\n")
	sb.WriteString(fmt.Sprintf("- 当前重试: 第 %d/%d 次。\n", attempt, interactiveMemoryAgentMaxAttempts))
	sb.WriteString("- 失败原因: ")
	sb.WriteString(previousErr.Error())
	sb.WriteString("\n")
	sb.WriteString("- 请根据失败原因修正输出，只返回合法 JSON object。\n")
	sb.WriteString("- story_memory_patches[].values 的所有值必须是文本；数字、布尔值、枚举和序号都要写成字符串。\n")
	sb.WriteString("- 不要输出 Markdown、解释、代码块或不在故事记忆结构协议内的字段。\n")
	return sb.String()
}

func interactiveMemoryAttemptFailure(attempt int, err error) string {
	return fmt.Sprintf("第 %d/%d 次执行失败：%s", attempt, interactiveMemoryAgentMaxAttempts, err.Error())
}
