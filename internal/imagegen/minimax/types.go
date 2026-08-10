// Package minimax 封装 MiniMax image_generation 原生 API 的客户端调用。
// 它只负责 HTTP 通信和协议解析，不关心 Denova 的业务概念。
// 未来若 MiniMax 发布官方 Go SDK，只需替换本包的实现，上层适配器无需改动。
//
// API 文档: https://platform.minimaxi.com/docs/api-reference/image-generation-t2i
package minimax

import (
	"encoding/json"
	"errors"
)

// errInvalidResponse 表示响应字段无法解析为字符串或字符串数组。
var errInvalidResponse = errors.New("MiniMax 响应字段无法解析为字符串或字符串数组")

// Request 是 image_generation 接口的请求参数。
type Request struct {
	Prompt         string `json:"prompt"`
	Model          string `json:"model,omitempty"`
	AspectRatio    string `json:"aspect_ratio,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"` // url 或 base64
}

// BaseResp 是 MiniMax 所有接口共有的状态信息。status_code 为 0 表示成功。
type BaseResp struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

// flexibleStrings 兼容 JSON 字符串或字符串数组。
// MiniMax 的 image_base64 / image_url 在 n=1 时可能返回单个字符串，n>1 时返回数组。
type flexibleStrings []string

func (f *flexibleStrings) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		if single != "" {
			*f = []string{single}
		}
		return nil
	}
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*f = arr
		return nil
	}
	return errInvalidResponse
}

// ImageData 是响应中生成的图像数据。
type ImageData struct {
	ImageBase64 flexibleStrings `json:"image_base64,omitempty"`
	ImageURL    flexibleStrings `json:"image_url,omitempty"`
}

// Response 是 image_generation 接口的完整响应。
type Response struct {
	BaseResp BaseResp  `json:"base_resp"`
	Data     ImageData `json:"data"`
}
