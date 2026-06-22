package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var (
	modelInputLogSeq  atomic.Uint64
	modelInputLogMu   sync.Mutex
	modelInputLogPath = filepath.Join("log", "llm-inputs.jsonl")
)

const modelInputLogMaxLines = 10

type modelInputLogOptions struct {
	AgentKind string
	Source    string
	Mode      string
	Config    openai.ChatModelConfig
	Messages  []*schema.Message
	Tools     []*schema.ToolInfo
}

type modelInputLogRecord struct {
	Type         string                   `json:"type"`
	Timestamp    string                   `json:"timestamp"`
	CallID       string                   `json:"call_id"`
	AgentKind    string                   `json:"agent_kind,omitempty"`
	Source       string                   `json:"source,omitempty"`
	Mode         string                   `json:"mode,omitempty"`
	ModelConfig  modelInputLogModelConfig `json:"model_config"`
	MessageCount int                      `json:"message_count"`
	ToolCount    int                      `json:"tool_count"`
	Messages     []*schema.Message        `json:"messages"`
	Tools        []modelInputLogTool      `json:"tools,omitempty"`
}

type modelInputLogModelConfig struct {
	Model               string                      `json:"model,omitempty"`
	BaseURL             string                      `json:"base_url,omitempty"`
	MaxTokens           *int                        `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int                        `json:"max_completion_tokens,omitempty"`
	Temperature         *float32                    `json:"temperature,omitempty"`
	TopP                *float32                    `json:"top_p,omitempty"`
	Stop                []string                    `json:"stop,omitempty"`
	PresencePenalty     *float32                    `json:"presence_penalty,omitempty"`
	ResponseFormat      any                         `json:"response_format,omitempty"`
	Seed                *int                        `json:"seed,omitempty"`
	FrequencyPenalty    *float32                    `json:"frequency_penalty,omitempty"`
	LogitBias           map[string]int              `json:"logit_bias,omitempty"`
	User                *string                     `json:"user,omitempty"`
	ExtraFields         map[string]any              `json:"extra_fields,omitempty"`
	ReasoningEffort     openai.ReasoningEffortLevel `json:"reasoning_effort,omitempty"`
	Modalities          []openai.Modality           `json:"modalities,omitempty"`
}

type modelInputLogTool struct {
	Name            string         `json:"name"`
	Description     string         `json:"description,omitempty"`
	Extra           map[string]any `json:"extra,omitempty"`
	Parameters      any            `json:"parameters,omitempty"`
	ParametersError string         `json:"parameters_error,omitempty"`
}

func logFullModelInput(opts modelInputLogOptions) {
	callSeq := modelInputLogSeq.Add(1)
	record := modelInputLogRecord{
		Type:         "llm_input",
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		CallID:       fmt.Sprintf("llm-%d", callSeq),
		AgentKind:    opts.AgentKind,
		Source:       opts.Source,
		Mode:         opts.Mode,
		ModelConfig:  modelInputLogConfigFromOpenAI(opts.Config),
		MessageCount: len(opts.Messages),
		ToolCount:    len(opts.Tools),
		Messages:     opts.Messages,
		Tools:        modelInputLogTools(opts.Tools),
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(record); err != nil {
		log.Printf("[llm-input-log] marshal failed agent=%s source=%s mode=%s err=%v", opts.AgentKind, opts.Source, opts.Mode, err)
		return
	}

	if err := appendModelInputLog(buf.Bytes()); err != nil {
		log.Printf("[llm-input-log] write failed agent=%s source=%s mode=%s call_id=%s path=%s bytes=%d err=%v", opts.AgentKind, opts.Source, opts.Mode, record.CallID, modelInputLogPath, buf.Len(), err)
		return
	}
	log.Printf("[llm-input-log] captured agent=%s source=%s mode=%s call_id=%s path=%s bytes=%d messages=%d tools=%d", opts.AgentKind, opts.Source, opts.Mode, record.CallID, modelInputLogPath, buf.Len(), record.MessageCount, record.ToolCount)
}

func appendModelInputLog(payload []byte) error {
	modelInputLogMu.Lock()
	defer modelInputLogMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(modelInputLogPath), 0755); err != nil {
		return err
	}
	if len(payload) == 0 || payload[len(payload)-1] != '\n' {
		payload = append(append([]byte(nil), payload...), '\n')
	}
	previous, err := readLastModelInputLogLines(modelInputLogPath, modelInputLogMaxLines-1)
	if err != nil {
		return err
	}
	tmpPath := modelInputLogPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if len(previous) > 0 {
		if _, err := f.Write(previous); err != nil {
			_ = f.Close()
			_ = os.Remove(tmpPath)
			return err
		}
	}
	if _, err := f.Write(payload); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, modelInputLogPath)
}

func readLastModelInputLogLines(path string, maxLines int) ([]byte, error) {
	if maxLines <= 0 {
		return nil, nil
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size <= 0 {
		return nil, nil
	}

	const chunkSize int64 = 64 * 1024
	offset := size
	var data []byte
	for offset > 0 && bytes.Count(data, []byte{'\n'}) <= maxLines {
		readSize := chunkSize
		if offset < readSize {
			readSize = offset
		}
		offset -= readSize
		chunk := make([]byte, readSize)
		if _, err := f.ReadAt(chunk, offset); err != nil {
			return nil, err
		}
		data = append(chunk, data...)
	}
	return lastModelInputLogLines(data, maxLines), nil
}

func lastModelInputLogLines(data []byte, maxLines int) []byte {
	if maxLines <= 0 || len(data) == 0 {
		return nil
	}
	searchEnd := len(data)
	if data[searchEnd-1] == '\n' {
		searchEnd--
	}
	seen := 0
	for i := searchEnd - 1; i >= 0; i-- {
		if data[i] != '\n' {
			continue
		}
		seen++
		if seen == maxLines {
			return data[i+1:]
		}
	}
	return data
}

func modelInputLogConfigFromOpenAI(cfg openai.ChatModelConfig) modelInputLogModelConfig {
	return modelInputLogModelConfig{
		Model:               cfg.Model,
		BaseURL:             cfg.BaseURL,
		MaxTokens:           cfg.MaxTokens,
		MaxCompletionTokens: cfg.MaxCompletionTokens,
		Temperature:         cfg.Temperature,
		TopP:                cfg.TopP,
		Stop:                cfg.Stop,
		PresencePenalty:     cfg.PresencePenalty,
		ResponseFormat:      cfg.ResponseFormat,
		Seed:                cfg.Seed,
		FrequencyPenalty:    cfg.FrequencyPenalty,
		LogitBias:           cfg.LogitBias,
		User:                cfg.User,
		ExtraFields:         cfg.ExtraFields,
		ReasoningEffort:     cfg.ReasoningEffort,
		Modalities:          cfg.Modalities,
	}
}

func modelInputLogTools(tools []*schema.ToolInfo) []modelInputLogTool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]modelInputLogTool, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		item := modelInputLogTool{
			Name:        tool.Name,
			Description: tool.Desc,
			Extra:       tool.Extra,
		}
		if tool.ParamsOneOf != nil {
			parameters, err := tool.ParamsOneOf.ToJSONSchema()
			if err != nil {
				item.ParametersError = err.Error()
			} else {
				item.Parameters = parameters
			}
		}
		result = append(result, item)
	}
	return result
}

type modelInputLoggingMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	agentKind string
	config    openai.ChatModelConfig
}

func (m *modelInputLoggingMiddleware) WrapModel(ctx context.Context, wrapped model.BaseChatModel, mc *adk.ModelContext) (model.BaseChatModel, error) {
	return &modelInputLoggingChatModel{
		inner:     wrapped,
		agentKind: m.agentKind,
		config:    m.config,
		tools:     modelInputToolsFromContext(mc),
	}, nil
}

type modelInputLoggingChatModel struct {
	inner     model.BaseChatModel
	agentKind string
	config    openai.ChatModelConfig
	tools     []*schema.ToolInfo
}

func (m *modelInputLoggingChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	logFullModelInput(modelInputLogOptions{
		AgentKind: m.agentKind,
		Source:    "adk",
		Mode:      "generate",
		Config:    m.config,
		Messages:  input,
		Tools:     m.tools,
	})
	return m.inner.Generate(ctx, input, opts...)
}

func (m *modelInputLoggingChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	logFullModelInput(modelInputLogOptions{
		AgentKind: m.agentKind,
		Source:    "adk",
		Mode:      "stream",
		Config:    m.config,
		Messages:  input,
		Tools:     m.tools,
	})
	return m.inner.Stream(ctx, input, opts...)
}

func modelInputToolsFromContext(mc *adk.ModelContext) []*schema.ToolInfo {
	if mc == nil || len(mc.Tools) == 0 {
		return nil
	}
	return append([]*schema.ToolInfo(nil), mc.Tools...)
}
