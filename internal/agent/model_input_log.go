package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	openaiprotocol "github.com/cloudwego/eino-ext/libs/acl/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var (
	modelInputLogEnabled atomic.Bool
	modelInputLogSeq     atomic.Uint64
	modelInputLogMu      sync.Mutex
	modelInputLogPath    = filepath.Join("log", "llm-inputs.jsonl")
	modelInputLogJobs    chan modelInputLogJob
	modelInputLogOnce    sync.Once
	modelInputLogWG      sync.WaitGroup
)

const (
	modelInputLogMaxLines  = 10
	modelInputLogQueueSize = 32
)

type modelInputLogOptions struct {
	CallID    string
	RunID     string
	SpanID    string
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
	RunID        string                   `json:"run_id,omitempty"`
	SpanID       string                   `json:"span_id,omitempty"`
	AgentKind    string                   `json:"agent_kind,omitempty"`
	Source       string                   `json:"source,omitempty"`
	Mode         string                   `json:"mode,omitempty"`
	ProviderID   string                   `json:"provider_request_id,omitempty"`
	ModelConfig  modelInputLogModelConfig `json:"model_config"`
	MessageCount int                      `json:"message_count"`
	ToolCount    int                      `json:"tool_count"`
	Cache        modelInputLogCache       `json:"cache_attribution"`
	Messages     []*schema.Message        `json:"messages"`
	Tools        []modelInputLogTool      `json:"tools,omitempty"`
}

type modelInputLogInputJob struct {
	Timestamp    string
	CallID       string
	RunID        string
	SpanID       string
	AgentKind    string
	Source       string
	Mode         string
	Config       openai.ChatModelConfig
	MessageCount int
	ToolCount    int
	Messages     []*schema.Message
	Tools        []*schema.ToolInfo
}

type modelInputLogProviderRequestIDRecord struct {
	Type       string `json:"type"`
	Timestamp  string `json:"timestamp"`
	CallID     string `json:"call_id"`
	AgentKind  string `json:"agent_kind,omitempty"`
	Source     string `json:"source,omitempty"`
	Mode       string `json:"mode,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	CallIndex  int    `json:"call_index,omitempty"`
	Model      string `json:"model,omitempty"`
	ProviderID string `json:"provider_request_id"`
}

type modelInputLogJob struct {
	input             *modelInputLogInputJob
	providerRequestID *modelInputLogProviderRequestIDRecord
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

type modelInputLogCache struct {
	MessageFingerprint      string                         `json:"message_fingerprint,omitempty"`
	SystemPromptFingerprint string                         `json:"system_prompt_fingerprint,omitempty"`
	ToolSchemaFingerprint   string                         `json:"tool_schema_fingerprint,omitempty"`
	ToolNames               []string                       `json:"tool_names,omitempty"`
	ToolFingerprints        []modelInputLogToolFingerprint `json:"tool_fingerprints,omitempty"`
	MessageCount            int                            `json:"message_count"`
	ToolCount               int                            `json:"tool_count"`
}

type modelInputLogToolFingerprint struct {
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
}

// SetModelInputLoggingEnabled controls full model input logging.
// Enable it only for developer starts because records include complete model-visible content.
func SetModelInputLoggingEnabled(enabled bool) {
	modelInputLogEnabled.Store(enabled)
}

func newModelInputCallID() string {
	callSeq := modelInputLogSeq.Add(1)
	return fmt.Sprintf("llm-%d", callSeq)
}

func logFullModelInput(opts modelInputLogOptions) string {
	if !modelInputLogEnabled.Load() {
		return ""
	}
	callID := strings.TrimSpace(opts.CallID)
	if callID == "" {
		callID = newModelInputCallID()
	}

	input := modelInputLogInputJob{
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		CallID:       callID,
		RunID:        strings.TrimSpace(opts.RunID),
		SpanID:       strings.TrimSpace(opts.SpanID),
		AgentKind:    opts.AgentKind,
		Source:       opts.Source,
		Mode:         opts.Mode,
		Config:       opts.Config,
		MessageCount: len(opts.Messages),
		ToolCount:    len(opts.Tools),
		Messages:     append([]*schema.Message(nil), opts.Messages...),
		Tools:        cloneToolInfos(opts.Tools),
	}

	if !enqueueModelInputLogJob(modelInputLogJob{input: &input}) {
		log.Printf("[llm-input-log] dropped agent=%s source=%s mode=%s call_id=%s reason=queue_full", opts.AgentKind, opts.Source, opts.Mode, callID)
		return ""
	}
	log.Printf("[llm-input-log] queued agent=%s source=%s mode=%s call_id=%s path=%s messages=%d tools=%d", opts.AgentKind, opts.Source, opts.Mode, callID, modelInputLogPath, input.MessageCount, input.ToolCount)
	return callID
}

func logModelProviderRequestID(agentKind, source, mode, modelName, runID string, callIndex int, msg *schema.Message) string {
	return logModelProviderRequestIDForCall("", agentKind, source, mode, modelName, runID, callIndex, msg)
}

func logModelProviderRequestIDForCall(callID, agentKind, source, mode, modelName, runID string, callIndex int, msg *schema.Message) string {
	requestID := providerRequestIDFromMessage(msg)
	if requestID == "" {
		return ""
	}
	log.Printf(
		"[model-response] provider_request_id=%s agent=%s source=%s mode=%s model=%q run_id=%s call_index=%d",
		requestID,
		strings.TrimSpace(agentKind),
		strings.TrimSpace(source),
		strings.TrimSpace(mode),
		strings.TrimSpace(modelName),
		strings.TrimSpace(runID),
		callIndex,
	)
	attachProviderRequestIDToModelInputLog(callID, agentKind, source, mode, modelName, runID, callIndex, requestID)
	return requestID
}

func providerRequestIDFromMessage(msg *schema.Message) string {
	if msg == nil {
		return ""
	}
	if requestID := strings.TrimSpace(openaiprotocol.GetRequestID(msg)); requestID != "" {
		return requestID
	}
	if msg.Extra == nil {
		return ""
	}
	if requestID, ok := msg.Extra["openai-request-id"].(string); ok {
		return strings.TrimSpace(requestID)
	}
	return ""
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

func enqueueModelInputLogJob(job modelInputLogJob) bool {
	modelInputLogOnce.Do(func() {
		modelInputLogJobs = make(chan modelInputLogJob, modelInputLogQueueSize)
		go runModelInputLogWorker(modelInputLogJobs)
	})
	modelInputLogWG.Add(1)
	select {
	case modelInputLogJobs <- job:
		return true
	default:
		modelInputLogWG.Done()
		return false
	}
}

func runModelInputLogWorker(jobs <-chan modelInputLogJob) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[llm-input-log] worker panic recovered err=%v", recovered)
		}
	}()
	for job := range jobs {
		func() {
			defer modelInputLogWG.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Printf("[llm-input-log] job panic recovered err=%v", recovered)
				}
			}()
			if err := writeModelInputLogJob(job); err != nil {
				log.Printf("[llm-input-log] write failed path=%s err=%v", modelInputLogPath, err)
			}
		}()
	}
}

func writeModelInputLogJob(job modelInputLogJob) error {
	switch {
	case job.input != nil:
		input := job.input
		tools := modelInputLogTools(input.Tools)
		record := modelInputLogRecord{
			Type:         "llm_input",
			Timestamp:    input.Timestamp,
			CallID:       input.CallID,
			RunID:        input.RunID,
			SpanID:       input.SpanID,
			AgentKind:    input.AgentKind,
			Source:       input.Source,
			Mode:         input.Mode,
			ModelConfig:  modelInputLogConfigFromOpenAI(input.Config),
			MessageCount: input.MessageCount,
			ToolCount:    input.ToolCount,
			Cache:        modelInputLogCacheAttribution(input.Messages, tools),
			Messages:     input.Messages,
			Tools:        tools,
		}
		payload, err := marshalModelInputLogRecord(record)
		if err != nil {
			return err
		}
		return appendModelInputLog(payload)
	case job.providerRequestID != nil:
		payload, err := marshalModelInputLogProviderRequestIDRecord(*job.providerRequestID)
		if err != nil {
			return err
		}
		return appendModelInputLog(payload)
	default:
		return nil
	}
}

func attachProviderRequestIDToModelInputLog(callID, agentKind, source, mode, modelName, runID string, callIndex int, requestID string) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || !modelInputLogEnabled.Load() {
		return
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return
	}
	record := &modelInputLogProviderRequestIDRecord{
		Type:       "llm_provider_request_id",
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		CallID:     callID,
		AgentKind:  strings.TrimSpace(agentKind),
		Source:     strings.TrimSpace(source),
		Mode:       strings.TrimSpace(mode),
		RunID:      strings.TrimSpace(runID),
		CallIndex:  callIndex,
		Model:      strings.TrimSpace(modelName),
		ProviderID: requestID,
	}
	if !enqueueModelInputLogJob(modelInputLogJob{providerRequestID: record}) {
		log.Printf("[llm-input-log] provider_request_id dropped call_id=%s reason=queue_full", callID)
	}
}

func marshalModelInputLogRecord(record modelInputLogRecord) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(record); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func marshalModelInputLogProviderRequestIDRecord(record modelInputLogProviderRequestIDRecord) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(record); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
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

func modelInputLogCacheAttribution(messages []*schema.Message, tools []modelInputLogTool) modelInputLogCache {
	return modelInputLogCache{
		MessageFingerprint:      modelInputLogFingerprint(messages),
		SystemPromptFingerprint: modelInputLogFingerprint(modelInputLogSystemMessages(messages)),
		ToolSchemaFingerprint:   modelInputLogFingerprint(tools),
		ToolNames:               modelInputLogToolNames(tools),
		ToolFingerprints:        modelInputLogToolFingerprints(tools),
		MessageCount:            len(messages),
		ToolCount:               len(tools),
	}
}

func modelInputLogSystemMessages(messages []*schema.Message) []*schema.Message {
	if len(messages) == 0 {
		return nil
	}
	var result []*schema.Message
	for _, msg := range messages {
		if msg == nil || msg.Role != schema.System {
			continue
		}
		result = append(result, msg)
	}
	return result
}

func modelInputLogToolNames(tools []modelInputLogTool) []string {
	if len(tools) == 0 {
		return nil
	}
	names := make([]string, 0, len(tools))
	for _, item := range tools {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names
}

func modelInputLogToolFingerprints(tools []modelInputLogTool) []modelInputLogToolFingerprint {
	if len(tools) == 0 {
		return nil
	}
	result := make([]modelInputLogToolFingerprint, 0, len(tools))
	for _, item := range tools {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		result = append(result, modelInputLogToolFingerprint{
			Name:        name,
			Fingerprint: modelInputLogFingerprint(item),
		})
	}
	return result
}

func modelInputLogFingerprint(value any) string {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) == 0 || bytes.Equal(payload, []byte("null")) {
		return ""
	}
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("sha256:%x", sum[:8])
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
	span, callID, spanCtx := beginLLMCallTrace(ctx, m.agentKind, "adk", "generate", m.config, input, m.tools, false)
	msg, err := m.inner.Generate(spanCtx, input, stableToolModelOptions(opts, m.tools)...)
	finishLLMCallTrace(span, callID, m.agentKind, "adk", "generate", m.config.Model, 0, msg, err, nil)
	return msg, err
}

func (m *modelInputLoggingChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	span, callID, spanCtx := beginLLMCallTrace(ctx, m.agentKind, "adk", "stream", m.config, input, m.tools, true)
	started := time.Now()
	var firstChunk time.Time
	var chunks []*schema.Message
	stream, err := m.inner.Stream(spanCtx, input, stableToolModelOptions(opts, m.tools)...)
	if err != nil {
		finishLLMCallTrace(span, callID, m.agentKind, "adk", "stream", m.config.Model, 0, nil, err, nil)
		return nil, err
	}
	return schema.StreamReaderWithConvert(stream, func(msg *schema.Message) (*schema.Message, error) {
		if msg != nil {
			if firstChunk.IsZero() {
				firstChunk = time.Now()
			}
			chunks = append(chunks, msg)
		}
		return msg, nil
	}, schema.WithErrWrapper(func(err error) error {
		finishLLMCallTrace(span, callID, m.agentKind, "adk", "stream", m.config.Model, 0, nil, err, map[string]any{
			"ttft_ms": durationMilliseconds(started, firstChunk),
		})
		return err
	}), schema.WithOnEOF(func() (any, error) {
		msg, concatErr := schema.ConcatMessages(chunks)
		finishLLMCallTrace(span, callID, m.agentKind, "adk", "stream", m.config.Model, 0, msg, concatErr, map[string]any{
			"ttft_ms": durationMilliseconds(started, firstChunk),
		})
		return nil, io.EOF
	})), nil
}

func modelInputToolsFromContext(mc *adk.ModelContext) []*schema.ToolInfo {
	if mc == nil || len(mc.Tools) == 0 {
		return nil
	}
	return cloneToolInfos(mc.Tools)
}

func stableToolModelOptions(opts []model.Option, tools []*schema.ToolInfo) []model.Option {
	if len(tools) == 0 {
		return opts
	}
	next := make([]model.Option, 0, len(opts)+1)
	next = append(next, opts...)
	next = append(next, model.WithTools(cloneToolInfos(tools)))
	return next
}

func cloneToolInfos(tools []*schema.ToolInfo) []*schema.ToolInfo {
	if len(tools) == 0 {
		return nil
	}
	result := make([]*schema.ToolInfo, 0, len(tools))
	for _, item := range tools {
		if item == nil {
			continue
		}
		result = append(result, cloneToolInfo(item))
	}
	return result
}

func cloneToolInfo(item *schema.ToolInfo) *schema.ToolInfo {
	if item == nil {
		return nil
	}
	data, err := json.Marshal(item)
	if err == nil {
		var cloned schema.ToolInfo
		if unmarshalErr := json.Unmarshal(data, &cloned); unmarshalErr == nil {
			return &cloned
		}
	}
	cloned := &schema.ToolInfo{
		Name:        item.Name,
		Desc:        item.Desc,
		Extra:       cloneStringAnyMap(item.Extra),
		ParamsOneOf: cloneParamsOneOf(item.ParamsOneOf),
	}
	return cloned
}

func cloneParamsOneOf(params *schema.ParamsOneOf) *schema.ParamsOneOf {
	if params == nil {
		return nil
	}
	data, err := json.Marshal(&schema.ToolInfo{ParamsOneOf: params})
	if err != nil {
		return params
	}
	var cloned schema.ToolInfo
	if err := json.Unmarshal(data, &cloned); err != nil {
		return params
	}
	return cloned.ParamsOneOf
}

func cloneStringAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
