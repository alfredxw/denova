package agent

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"

	_ "golang.org/x/image/webp"
)

// InputEstimateVersion changes when the local token-counting units change. A stored
// response may calibrate a later request only when both use this version.
const InputEstimateVersion uint16 = 2

// InputSize measures context capacity. Tokens includes text and native vision.
// Bytes measures the provider-neutral JSON envelope (including attachment
// descriptors), never encoded image data. Adapters own actual wire limits.
type InputSize struct {
	Tokens int
	Bytes  int
}

// InputEstimator supplies a model's visual token policy to the shared request
// accounting. ImageTokens receives decoded dimensions, never compressed bytes.
// A zero value reserves 32K tokens per image for unidentified custom models;
// known adapters should provide their documented visual policy instead.
// Estimators are immutable and must be safe for concurrent use.
type InputEstimator struct {
	ImageTokens func(width, height int) int
}

// ModelInputEstimator is an optional model capability. Middleware that replaces
// a model must preserve its estimator or expose the replacement's estimator.
// Agent captures it before equivalent provider wrappers hide the concrete model.
type ModelInputEstimator interface {
	InputEstimator() InputEstimator
}

// Estimate reads native image headers from the existing immutable attachments.
// It neither loads full pixel buffers nor writes metadata back to the journal.
// Missing/invalid images fail explicitly rather than masquerading as oversized
// context. Protocol adapters still verify accepted SHA256 bytes before sending.
func (estimator InputEstimator) Estimate(messages []*Message, tools []*ToolInfo) (InputSize, error) {
	encoded, err := json.Marshal(struct {
		Messages []*Message  `json:"messages"`
		Tools    []*ToolInfo `json:"tools,omitempty"`
	}{messages, tools})
	if err != nil {
		return InputSize{}, fmt.Errorf("serialize model input for estimation: %w", err)
	}
	size := InputSize{Tokens: EstimateRequestTextTokens(messages, tools), Bytes: len(encoded)}
	for _, message := range messages {
		if message == nil || (message.Role != User && message.Role != ToolRole) {
			continue
		}
		for _, attachment := range message.Attachments {
			if !IsNativeImageMediaType(attachment.MediaType) {
				continue
			}
			config, err := attachmentImageSize(attachment)
			if err != nil {
				return InputSize{}, err
			}
			tokens := 32 * 1024
			if estimator.ImageTokens != nil {
				tokens = estimator.ImageTokens(config.Width, config.Height)
			}
			if tokens <= 0 {
				return InputSize{}, fmt.Errorf("image token estimator returned %d for %q", tokens, attachment.Name)
			}
			size.Tokens += tokens
		}
	}
	return size, nil
}

func attachmentImageSize(attachment Attachment) (image.Config, error) {
	file, err := os.Open(attachmentFilePath(attachment))
	if err != nil {
		return image.Config{}, fmt.Errorf("inspect attached image %q: %w", attachment.Name, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return image.Config{}, fmt.Errorf("stat attached image %q: %w", attachment.Name, err)
	}
	if !info.Mode().IsRegular() {
		return image.Config{}, fmt.Errorf("attached image %q is not a regular file", attachment.Name)
	}
	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return image.Config{}, fmt.Errorf("decode attached image %q dimensions: %w", attachment.Name, err)
	}
	if config.Width <= 0 || config.Height <= 0 {
		return image.Config{}, fmt.Errorf("attached image %q has invalid dimensions", attachment.Name)
	}
	return config, nil
}

func inputEstimatorForModel(model BaseChatModel) InputEstimator {
	if capability, ok := model.(ModelInputEstimator); ok {
		return capability.InputEstimator()
	}
	return InputEstimator{}
}
