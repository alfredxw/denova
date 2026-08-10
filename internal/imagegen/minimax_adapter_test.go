package imagegen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"denova/config"
)

// TestSizeToAspectRatio 测试尺寸到长宽比的转换
func TestSizeToAspectRatio(t *testing.T) {
	tests := []struct {
		size          string
		expectedRatio string
	}{
		// 精确匹配
		{"1024x1024", "1:1"},
		{"1280x720", "16:9"},
		{"720x1280", "9:16"},
		{"1152x864", "4:3"},
		{"864x1152", "3:4"},
		{"1248x832", "3:2"},
		{"832x1248", "2:3"},
		{"2048x2048", "1:1"},
		{"1920x1080", "16:9"},
		{"1080x1920", "9:16"},
		// 默认 / auto
		{"", "1:1"},
		{"auto", "1:1"},
		// 已经是长宽比格式
		{"16:9", "16:9"},
		{"4:3", "4:3"},
		// 自动匹配最接近的长宽比（不报错）
		{"9999x9999", "1:1"},
		{"768x1024", "3:4"},
		{"600x800", "3:4"},
		{"2000x800", "21:9"},
		{"800x2000", "9:16"},
	}
	for _, tt := range tests {
		t.Run(tt.size, func(t *testing.T) {
			if result := sizeToAspectRatio(tt.size); result != tt.expectedRatio {
				t.Errorf("尺寸 %q: 得到 %q, 期望 %q", tt.size, result, tt.expectedRatio)
			}
		})
	}
}

// TestNearestAspectRatio 测试自动匹配最接近的长宽比
func TestNearestAspectRatio(t *testing.T) {
	tests := []struct {
		size     string
		expected string
	}{
		{"1000x1000", "1:1"},
		{"1600x900", "16:9"},
		{"900x1600", "9:16"},
		{"800x600", "4:3"},
		{"600x800", "3:4"},
		{"invalid", "1:1"},
		{"0x0", "1:1"},
	}
	for _, tt := range tests {
		t.Run(tt.size, func(t *testing.T) {
			if result := nearestAspectRatio(tt.size); result != tt.expected {
				t.Errorf("尺寸 %q: 得到 %q, 期望 %q", tt.size, result, tt.expected)
			}
		})
	}
}

// TestDecodeBase64Image 测试共享的 base64 解码工具
func TestDecodeBase64Image(t *testing.T) {
	if _, err := decodeBase64Image(""); err == nil {
		t.Error("空字符串应该报错")
	}
	if _, err := decodeBase64Image("!!!invalid!!!"); err == nil {
		t.Error("无效 base64 应该报错")
	}
}

// TestMiniMaxProfileResolution 测试 MiniMax 配置解析
func TestMiniMaxProfileResolution(t *testing.T) {
	cfg := &config.Config{
		ImageAPIProfiles: []config.ImageAPIProfileSettings{
			{
				ID:            "minimax-test",
				Provider:      "minimax",
				OpenAIAPIKey:  "sk-test-key",
				OpenAIModel:   "image-01",
				OpenAIBaseURL: "https://api.minimaxi.com/v1",
			},
		},
	}
	profile, err := config.ResolveImageAPIProfile(cfg, "minimax-test")
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	if profile.Provider != "minimax" {
		t.Errorf("Provider 错误: %q", profile.Provider)
	}
}

// TestMiniMaxProfileDefaults 测试 MiniMax 默认值
func TestMiniMaxProfileDefaults(t *testing.T) {
	cfg := &config.Config{
		ImageAPIProfiles: []config.ImageAPIProfileSettings{
			{ID: "minimax-auto", Provider: "minimax", OpenAIAPIKey: "sk-test-key"},
		},
	}
	profile, err := config.ResolveImageAPIProfile(cfg, "minimax-auto")
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	if profile.OpenAIBaseURL != "https://api.minimaxi.com/v1" {
		t.Errorf("默认 BaseURL 错误: %q", profile.OpenAIBaseURL)
	}
	if profile.OpenAIModel != "image-01" {
		t.Errorf("默认模型错误: %q", profile.OpenAIModel)
	}
}

// TestMiniMaxProfileValidation 测试配置验证
func TestMiniMaxProfileValidation(t *testing.T) {
	cfg := &config.Config{
		ImageAPIProfiles: []config.ImageAPIProfileSettings{
			{ID: "minimax-nokey", Provider: "minimax"},
		},
	}
	if _, err := config.ResolveImageAPIProfile(cfg, "minimax-nokey"); err == nil {
		t.Error("缺少 API Key 应该报错")
	}
}

// ---------- Adapter 行为测试 ----------

// 一像素 PNG（8x8 透明）作为成功路径的图像数据。
const onePixelPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAgAAAAIAQMAAAD+wSzIAAAABlBMVEX///+/v7+jQ3Y5AAAADklEQVQI12P4//8/w38GIAXDAgcOwYWFAAAAAElFTkSuQmCC"

func newMiniMaxProfile(baseURL string) config.ResolvedImageAPIProfile {
	return config.ResolvedImageAPIProfile{
		ProfileID:     "minimax-test",
		Provider:      "minimax",
		OpenAIAPIKey:  "sk-test-key",
		OpenAIBaseURL: baseURL,
		OpenAIModel:   "image-01",
		OutputFormat:  "png",
	}
}

// TestMiniMaxAdapterGenerateSuccess 测试成功路径：返回 base64 时正确提取图像。
func TestMiniMaxAdapterGenerateSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/image_generation" {
			t.Errorf("非预期路径: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test-key" {
			t.Errorf("Authorization 头错误: %q", got)
		}
		body, err := json.Marshal(map[string]any{
			"base_resp": map[string]any{"status_code": 0, "status_msg": ""},
			"data":      map[string]any{"image_base64": onePixelPNGBase64},
		})
		if err != nil {
			t.Fatalf("编码响应失败: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	adapter := NewMiniMaxAdapter(nil)
	profile := newMiniMaxProfile(server.URL)
	result, err := adapter.Generate(context.Background(), profile, GenerateRequest{
		Prompt: "一只猫",
		Size:   "1024x1024",
	})
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	if len(result.Images) != 1 {
		t.Fatalf("期望 1 张图像, 得到 %d", len(result.Images))
	}
	if len(result.Images[0].Data) == 0 {
		t.Error("图像数据为空")
	}
	if result.OutputFormat != "png" {
		t.Errorf("OutputFormat 错误: %q", result.OutputFormat)
	}
}

// TestMiniMaxAdapterGenerateHTTPError 测试 HTTP 4xx/5xx 错误。
func TestMiniMaxAdapterGenerateHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer server.Close()

	adapter := NewMiniMaxAdapter(nil)
	profile := newMiniMaxProfile(server.URL)
	_, err := adapter.Generate(context.Background(), profile, GenerateRequest{Prompt: "x"})
	if err == nil {
		t.Fatal("HTTP 400 应当返回错误")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("错误消息应包含 HTTP 400, 实际: %v", err)
	}
}

// TestMiniMaxAdapterGenerateBusinessError 测试 base_resp.status_code != 0 的业务错误。
func TestMiniMaxAdapterGenerateBusinessError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]any{
			"base_resp": map[string]any{"status_code": 1004, "status_msg": "rate limit"},
		})
		_, _ = w.Write(body)
	}))
	defer server.Close()

	adapter := NewMiniMaxAdapter(nil)
	profile := newMiniMaxProfile(server.URL)
	_, err := adapter.Generate(context.Background(), profile, GenerateRequest{Prompt: "x"})
	if err == nil {
		t.Fatal("业务错误应被返回")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("错误消息应包含业务错误描述, 实际: %v", err)
	}
}

// TestMiniMaxAdapterGenerateEmptyResponse 测试 200 但响应里没有图像数据。
func TestMiniMaxAdapterGenerateEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]any{
			"base_resp": map[string]any{"status_code": 0, "status_msg": ""},
			"data":      map[string]any{},
		})
		_, _ = w.Write(body)
	}))
	defer server.Close()

	adapter := NewMiniMaxAdapter(nil)
	profile := newMiniMaxProfile(server.URL)
	_, err := adapter.Generate(context.Background(), profile, GenerateRequest{Prompt: "x"})
	if err == nil {
		t.Fatal("空响应应该返回错误")
	}
	if !errors.Is(err, ErrMiniMaxResponseInvalid) {
		t.Errorf("应该返回 ErrMiniMaxResponseInvalid, 实际: %v", err)
	}
}

// TestMiniMaxAdapterGenerateEmptyWithMsg 测试 200、空数据但 base_resp.status_msg 非空。
func TestMiniMaxAdapterGenerateEmptyWithMsg(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]any{
			"base_resp": map[string]any{"status_code": 0, "status_msg": "safety block"},
			"data":      map[string]any{},
		})
		_, _ = w.Write(body)
	}))
	defer server.Close()

	adapter := NewMiniMaxAdapter(nil)
	profile := newMiniMaxProfile(server.URL)
	_, err := adapter.Generate(context.Background(), profile, GenerateRequest{Prompt: "x"})
	if err == nil {
		t.Fatal("空响应 + status_msg 应返回错误")
	}
	if !strings.Contains(err.Error(), "safety block") {
		t.Errorf("错误消息应包含 status_msg, 实际: %v", err)
	}
}

// TestMiniMaxAdapterGenerateMissingInputs 测试前置校验。
func TestMiniMaxAdapterGenerateMissingInputs(t *testing.T) {
	adapter := NewMiniMaxAdapter(nil)
	profile := newMiniMaxProfile("http://unused")

	if _, err := adapter.Generate(context.Background(), profile, GenerateRequest{
		Prompt: "",
	}); !errors.Is(err, ErrMiniMaxPromptRequired) {
		t.Errorf("空 prompt 应返回 ErrMiniMaxPromptRequired, 实际: %v", err)
	}
	profile.OpenAIAPIKey = ""
	if _, err := adapter.Generate(context.Background(), profile, GenerateRequest{
		Prompt: "x",
	}); !errors.Is(err, ErrMiniMaxAPIKeyMissing) {
		t.Errorf("空 API Key 应返回 ErrMiniMaxAPIKeyMissing, 实际: %v", err)
	}
}

// TestMiniMaxAdapterGenerateInvalidBase64 测试响应里的 base64 不可解析。
func TestMiniMaxAdapterGenerateInvalidBase64(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]any{
			"base_resp": map[string]any{"status_code": 0, "status_msg": ""},
			"data":      map[string]any{"image_base64": "!!!not-base64!!!"},
		})
		_, _ = w.Write(body)
	}))
	defer server.Close()

	adapter := NewMiniMaxAdapter(nil)
	profile := newMiniMaxProfile(server.URL)
	_, err := adapter.Generate(context.Background(), profile, GenerateRequest{Prompt: "x"})
	if err == nil {
		t.Fatal("无法解析的 base64 应返回错误")
	}
}

// TestInferOutputFormat 测试根据图像数据推断输出格式。
func TestInferOutputFormat(t *testing.T) {
	if got := inferOutputFormat([]Image{{Extension: "png"}}, ""); got != "png" {
		t.Errorf("png 应推断为 png, 实际: %q", got)
	}
	if got := inferOutputFormat([]Image{{Extension: "jpeg"}}, ""); got != "jpeg" {
		t.Errorf("jpeg 应推断为 jpeg, 实际: %q", got)
	}
	if got := inferOutputFormat([]Image{{Extension: "unknown"}}, "jpeg"); got != "jpeg" {
		t.Errorf("未知格式应回退到 request.OutputFormat, 实际: %q", got)
	}
	if got := inferOutputFormat([]Image{}, ""); got != "png" {
		t.Errorf("空列表 + 空 fallback 应为 png, 实际: %q", got)
	}
}

// TestNormalizeSizeForProviderMinimax 测试 MiniMax 专用尺寸白名单。
func TestNormalizeSizeForProviderMinimax(t *testing.T) {
	if _, ok := NormalizeSizeForProvider("minimax", "1024x1024"); !ok {
		t.Error("1024x1024 应被 MiniMax 接受")
	}
	if _, ok := NormalizeSizeForProvider("minimax", "1280x720"); !ok {
		t.Error("1280x720 应被 MiniMax 接受")
	}
	if _, ok := NormalizeSizeForProvider("minimax", "9999x9999"); ok {
		t.Error("9999x9999 应被 MiniMax 拒绝")
	}
	// OpenAI 默认白名单上 MiniMax 接受的 1024 系列不应通过
	if _, ok := NormalizeSizeForProvider("", "1024x1024"); ok {
		t.Error("OpenAI provider 不应接受 1024x1024")
	}
	if _, ok := NormalizeSizeForProvider("", "2048x2048"); !ok {
		t.Error("OpenAI provider 应接受 2048x2048")
	}
}

// TestMiniMaxAdapterGenerateAspectRatioRequest 验证请求里的 aspect_ratio 字段。
func TestMiniMaxAdapterGenerateAspectRatioRequest(t *testing.T) {
	var capturedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8192)
		n, _ := r.Body.Read(buf)
		capturedBody = string(buf[:n])
		body, _ := json.Marshal(map[string]any{
			"base_resp": map[string]any{"status_code": 0, "status_msg": ""},
			"data":      map[string]any{"image_base64": onePixelPNGBase64},
		})
		_, _ = w.Write(body)
	}))
	defer server.Close()

	adapter := NewMiniMaxAdapter(nil)
	profile := newMiniMaxProfile(server.URL)
	_, err := adapter.Generate(context.Background(), profile, GenerateRequest{
		Prompt: "x",
		Size:   "1280x720",
	})
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	if !strings.Contains(capturedBody, `"aspect_ratio":"16:9"`) {
		t.Errorf("请求应包含 aspect_ratio=16:9, 实际 body: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, `"response_format":"base64"`) {
		t.Errorf("请求应强制要求 base64, 实际 body: %s", capturedBody)
	}
}

// 防止 import 未使用：base64 在 PNG 字节码在其它测试中可能用到
var _ = base64.StdEncoding
