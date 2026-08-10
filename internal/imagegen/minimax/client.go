package minimax

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL 是 MiniMax 国内服务的默认 API 端点。
const DefaultBaseURL = "https://api.minimaxi.com/v1"

// Client 封装 MiniMax image_generation API 的 HTTP 调用。
// 它只负责协议通信，返回原始 Response，不做任何业务转换。
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient 创建 MiniMax 客户端。baseURL 为空时使用 DefaultBaseURL。
func NewClient(httpClient *http.Client, baseURL string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 120 * time.Second}
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{httpClient: httpClient, baseURL: baseURL}
}

// Generate 调用 image_generation 接口，返回原始响应。
// apiKey 是 MiniMax 的 Bearer token。
func (c *Client) Generate(ctx context.Context, apiKey string, req Request) (*Response, error) {
	apiURL := strings.TrimSuffix(c.baseURL, "/") + "/image_generation"

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("编码请求失败: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("MiniMax API 错误: HTTP %d, body: %s", resp.StatusCode, string(raw))
	}

	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	// MiniMax 即使 HTTP 200，也可能在 base_resp 中返回业务错误
	if result.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("MiniMax 业务错误: status_code=%d, status_msg=%s",
			result.BaseResp.StatusCode, result.BaseResp.StatusMsg)
	}
	return &result, nil
}
