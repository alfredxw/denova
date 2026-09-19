package providers

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/url"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	"github.com/disintegration/imaging"
)

// ImageInput is an ephemeral native image part. It never replaces the durable
// attachment: tools and previews continue to read the SHA-bound original.
type ImageInput struct {
	MediaType string
	Base64    string
}

func (input ImageInput) DataURL() string {
	return "data:" + input.MediaType + ";base64," + input.Base64
}

// NativeImageCount counts occurrences, including repeated images in history.
// Adapters pass the complete request count to PrepareImage for API admission.
func NativeImageCount(messages []*agent.Message) int {
	count := 0
	for _, message := range messages {
		if message != nil && (message.Role == agent.User || message.Role == agent.ToolRole) {
			for _, attachment := range message.Attachments {
				if agent.IsNativeImageMediaType(attachment.MediaType) {
					count++
				}
			}
		}
	}
	return count
}

// PrepareImage uses the model's documented resolution tier, preserving bytes
// when no conversion is needed. Unknown models retain their original pixels.
// Official endpoint constraints apply only to those endpoints, not to proxies
// that happen to speak the same protocol. Their own errors remain authoritative.
func (config ModelConfig) PrepareImage(attachment agent.Attachment, requestImageCount int) (ImageInput, error) {
	fail := func(detail string) (ImageInput, error) {
		return ImageInput{}, &APIError{StatusCode: http.StatusBadRequest, Kind: "image_input_error", Message: fmt.Sprintf("attached image %q: %s", attachment.Name, detail)}
	}
	data, err := agent.ReadAttachmentImage(attachment)
	if err != nil {
		return fail(err.Error())
	}
	dimensions, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || dimensions.Width <= 0 || dimensions.Height <= 0 {
		return fail("invalid image dimensions or encoding")
	}
	mediaType := "image/" + format
	if !agent.IsNativeImageMediaType(mediaType) {
		return fail("unsupported native image encoding")
	}
	width, height := dimensions.Width, dimensions.Height
	policy := config.imagePolicy()
	if policy.dimensions != nil {
		width, height = policy.dimensions(width, height)
	}
	endpoint, _ := url.Parse(config.BaseURL)
	host := ""
	if endpoint != nil {
		host = strings.ToLower(endpoint.Hostname())
	}
	if host == "" {
		switch config.Protocol {
		case ProtocolAnthropicMessages:
			host = "api.anthropic.com"
		case ProtocolOpenAIResponses, ProtocolOpenAIChatCompletions:
			host = "api.openai.com"
		}
	}
	maxEncodedBytes := 0
	switch host {
	case "api.anthropic.com":
		// https://platform.claude.com/docs/en/build-with-claude/vision
		if requestImageCount > 600 {
			return fail("request exceeds 600 images")
		}
		edge := 8000
		if requestImageCount > 20 {
			edge = 2000
		}
		width, height = imageDimensions(width, height, 1, edge, 0)
		maxEncodedBytes = 10 << 20
	case "api.openai.com":
		// https://developers.openai.com/api/docs/guides/images-vision
		if requestImageCount > 1500 || policy.patchSize == 32 && ((int64(width)+31)/32)*((int64(height)+31)/32) > 30000 {
			return fail("request exceeds the image count or native image patch limit")
		}
	}
	// GIF input is its first frame. Flattening also admits animated uploads on
	// APIs that accept only non-animated GIFs; the original remains available.
	convert := width != dimensions.Width || height != dimensions.Height || format == "gif"
	if convert {
		// Bound decoded working memory before allocation. 64 MP covers 8000²
		// input; larger images can still pass through unchanged to capable APIs.
		if int64(dimensions.Width)*int64(dimensions.Height) > 64_000_000 {
			return fail("image exceeds the 64 megapixel local conversion limit")
		}
		picture, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
		if err != nil {
			return fail(fmt.Sprintf("decode image: %v", err))
		}
		if format == "gif" && picture.Bounds() != image.Rect(0, 0, dimensions.Width, dimensions.Height) {
			// A GIF frame may occupy only part of its logical canvas. Keep
			// that placement before resizing rather than stretching the frame.
			canvas := image.NewNRGBA(image.Rect(0, 0, dimensions.Width, dimensions.Height))
			draw.Draw(canvas, picture.Bounds(), picture, picture.Bounds().Min, draw.Src)
			picture = canvas
		}
		if picture.Bounds().Dx() == dimensions.Height && picture.Bounds().Dy() == dimensions.Width {
			width, height = height, width
		}
		picture = imaging.Resize(picture, width, height, imaging.Lanczos)
		var encoded bytes.Buffer
		if format == "jpeg" {
			err = jpeg.Encode(&encoded, picture, &jpeg.Options{Quality: 90})
		} else {
			mediaType = "image/png"
			err = png.Encode(&encoded, picture)
		}
		if err != nil {
			return fail(fmt.Sprintf("encode sending image: %v", err))
		}
		data = encoded.Bytes()
	}
	if maxEncodedBytes > 0 && base64.StdEncoding.EncodedLen(len(data)) > maxEncodedBytes {
		return fail(fmt.Sprintf("encoded image exceeds %d bytes", maxEncodedBytes))
	}
	return ImageInput{MediaType: mediaType, Base64: base64.StdEncoding.EncodeToString(data)}, nil
}
