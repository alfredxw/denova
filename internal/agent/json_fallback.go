package agent

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
)

// generateWithJSONFallback 先以 JSON response_format 调用模型，失败后降级为普通文本模式重试。
// 用于兼容不支持 response_format=json_object 的本地 LM 服务器（如 LM Studio）。
//
// baseCfg 不应设置 ResponseFormat，由本函数管理。
// agentLabel 用于结构化日志，便于定位。
func generateWithJSONFallback(
	ctx context.Context,
	baseCfg openai.ChatModelConfig,
	messages []*schema.Message,
	agentKind string,
	source string,
	agentLabel string,
) (string, error) {
	jsonCfg := baseCfg
	jsonCfg.ResponseFormat = &openai.ChatCompletionResponseFormat{
		Type: openai.ChatCompletionResponseFormatTypeJSONObject,
	}
	content, err := generateContentOnce(ctx, jsonCfg, messages, agentKind, source, agentLabel, "json_mode")
	if err == nil {
		return content, nil
	}
	if ctx.Err() != nil {
		return "", err
	}
	log.Printf("[%s] json_mode failed, retry without response_format err=%v", agentLabel, err)
	content, retryErr := generateContentOnce(ctx, baseCfg, messages, agentKind, source, agentLabel, "plain_text_retry")
	if retryErr != nil {
		return "", retryErr
	}
	return content, nil
}

func generateContentOnce(ctx context.Context, modelCfg openai.ChatModelConfig, messages []*schema.Message, agentKind, source, agentLabel, attempt string) (string, error) {
	log.Printf("[%s] generate begin attempt=%s model=%q base_url=%q json_mode=%t",
		agentLabel, attempt, modelCfg.Model, modelCfg.BaseURL, modelCfg.ResponseFormat != nil)
	cm, err := openai.NewChatModel(ctx, &modelCfg)
	if err != nil {
		log.Printf("[%s] create model failed attempt=%s err=%v", agentLabel, attempt, err)
		return "", fmt.Errorf("创建模型失败: %w", err)
	}
	mode := "generate_" + attempt
	span, callID, traceCtx := beginLLMCallTrace(ctx, agentKind, source, mode, modelCfg, messages, nil, false)
	msg, err := cm.Generate(traceCtx, messages)
	if err != nil {
		finishLLMCallTrace(span, callID, agentKind, source, mode, modelCfg.Model, 0, nil, err, nil)
		return "", describeModelError(agentLabel, attempt, err)
	}
	if msg == nil {
		finishLLMCallTrace(span, callID, agentKind, source, mode, modelCfg.Model, 0, nil, fmt.Errorf("模型返回为空"), nil)
		log.Printf("[%s] nil response attempt=%s", agentLabel, attempt)
		return "", fmt.Errorf("模型返回为空")
	}
	finishLLMCallTrace(span, callID, agentKind, source, mode, modelCfg.Model, 0, msg, nil, nil)
	log.Printf("[%s] generate done attempt=%s content=%s", agentLabel, attempt, promptPartSummary(msg.Content))
	return msg.Content, nil
}

// describeModelError 处理本地 LM 返回空错误消息的情况，补充可读的错误描述。
// 部分 eino-ext / OpenAI SDK 错误在 response_format 不被支持时 Error() 返回空字符串，
// 导致上层 fmt.Errorf("...: %w", err) 日志只显示前缀，丢失诊断信息。
func describeModelError(agentLabel, attempt string, err error) error {
	errText := strings.TrimSpace(err.Error())
	if errText != "" {
		log.Printf("[%s] generate failed attempt=%s err=%v", agentLabel, attempt, err)
		return err
	}
	log.Printf("[%s] generate failed attempt=%s err_type=%T err=<empty>", agentLabel, attempt, err)
	return fmt.Errorf("模型调用失败（错误详情为空，可能是本地 LM 不支持 response_format=json_object）")
}
