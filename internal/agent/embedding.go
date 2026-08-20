package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"denova/config"
	"denova/internal/providercompat"
)

// maxEmbeddingBatch 限制单次 /v1/embeddings 请求的输入条数。叙事记忆每回合
// 新增记录通常个位数,这个上限只在首次回填历史记忆时起作用。
const maxEmbeddingBatch = 64

// EmbeddingClient 是 OpenAI 兼容 /v1/embeddings 的极薄客户端。
// 该接口的请求/响应体足够简单(model + input[] → data[].embedding),
// 手写比引入额外的 embedding 组件依赖更可控,且能复用 providercompat
// 已经配置好的 HTTP 客户端(超时、重试、provider 兼容层)。
type EmbeddingClient struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewNarrativeMemoryEmbedder 按叙事记忆 Agent 解析出的 profile 构造 embedder。
// profile 未配置 embedding_model 时返回 nil —— 调用方据此退回纯关键词检索,
// 向量能力始终是可选增强,不是必需依赖。
func NewNarrativeMemoryEmbedder(cfg *config.Config) *EmbeddingClient {
	resolved := config.ResolveAgentModel(cfg, config.AgentKindNarrativeMemory)
	model := strings.TrimSpace(resolved.EmbeddingModel)
	if model == "" || strings.TrimSpace(resolved.OpenAIBaseURL) == "" {
		return nil
	}
	return &EmbeddingClient{
		apiKey:     resolved.OpenAIAPIKey,
		baseURL:    strings.TrimRight(strings.TrimSpace(resolved.OpenAIBaseURL), "/"),
		model:      model,
		httpClient: providercompat.WrapHTTPClient(nil),
	}
}

// EmbeddingModelID 标识向量的产出模型。换模型后旧向量的维度与语义空间都不再
// 可比,缓存层据此使整批旧向量失效而非静默混用。
func (c *EmbeddingClient) EmbeddingModelID() string {
	if c == nil {
		return ""
	}
	return c.model
}

// EmbedMemoryTexts 批量取文本向量,返回顺序与入参一一对应。
func (c *EmbeddingClient) EmbedMemoryTexts(ctx context.Context, texts []string) ([][]float32, error) {
	if c == nil {
		return nil, errors.New("embedding client not configured")
	}
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += maxEmbeddingBatch {
		end := min(start+maxEmbeddingBatch, len(texts))
		vectors, err := c.embedBatch(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vectors...)
	}
	if len(out) != len(texts) {
		return nil, fmt.Errorf("embedding count mismatch: got %d for %d inputs", len(out), len(texts))
	}
	return out, nil
}

func (c *EmbeddingClient) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	payload, err := json.Marshal(map[string]any{
		"model": c.model,
		"input": texts,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embeddings request failed: %s: %s", resp.Status, boundedErrorBody(body))
	}
	var decoded struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode embeddings response: %w", err)
	}
	if len(decoded.Data) != len(texts) {
		return nil, fmt.Errorf("embeddings response size mismatch: got %d for %d inputs", len(decoded.Data), len(texts))
	}
	// 响应可能乱序返回,按 index 归位而不是依赖数组顺序。
	vectors := make([][]float32, len(texts))
	for _, item := range decoded.Data {
		if item.Index < 0 || item.Index >= len(vectors) {
			return nil, fmt.Errorf("embeddings response index out of range: %d", item.Index)
		}
		if len(item.Embedding) == 0 {
			return nil, fmt.Errorf("embeddings response has empty vector at index %d", item.Index)
		}
		vectors[item.Index] = item.Embedding
	}
	for i, vector := range vectors {
		if vector == nil {
			return nil, fmt.Errorf("embeddings response missing index %d", i)
		}
	}
	return vectors, nil
}

func boundedErrorBody(body []byte) string {
	const maxErrorBody = 512
	trimmed := strings.TrimSpace(string(body))
	if len(trimmed) <= maxErrorBody {
		return trimmed
	}
	return trimmed[:maxErrorBody] + "…"
}
