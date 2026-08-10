package imagegen

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

// decodeBase64Image 解码 base64 图像数据，并在解码时检测格式。
// 这是 OpenAI 和 MiniMax 适配器共享的工具函数。
func decodeBase64Image(base64Data string) (Image, error) {
	base64Data = strings.TrimSpace(base64Data)
	if base64Data == "" {
		return Image{}, fmt.Errorf("base64 数据为空")
	}
	// 移除可能的数据 URL 前缀（如 data:image/png;base64,...）
	if idx := strings.Index(base64Data, ","); strings.HasPrefix(base64Data, "data:") && idx > 0 {
		base64Data = base64Data[idx+1:]
	}
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return Image{}, fmt.Errorf("base64 解码失败: %w", err)
	}
	format := imageFormatFromBytes(data)
	if format == "" {
		format = "png"
	}
	return Image{
		Data:      data,
		MIMEType:  mimeTypeForFormat(format),
		Extension: extensionForFormat(format),
	}, nil
}

// imageFormatFromBytes 通过 HTTP sniff 推断图像格式，供 OpenAI / MiniMax 适配器复用。
func imageFormatFromBytes(data []byte) string {
	contentType := http.DetectContentType(data)
	if format := imageFormatFromContentType(contentType); format != "" {
		return format
	}
	return ""
}

// mimeTypeForFormat 返回 image/png 或 image/jpeg；format 不可识别时返回空串。
func mimeTypeForFormat(format string) string {
	switch normalizeImageFormat(format) {
	case "png":
		return "image/png"
	case "jpeg":
		return "image/jpeg"
	default:
		return ""
	}
}
