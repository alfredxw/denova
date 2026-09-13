package providers

import (
	"math"
	"path"
	"strconv"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// InputEstimator follows the image detail actually sent by the built-in
// adapters: OpenAI uses auto, and Anthropic uses its native resolution tier.
// Model names also work through compatible endpoints. Unidentified models use
// Agent's conservative visual reserve; encoded bytes never stand in for pixels.
func (config ModelConfig) InputEstimator() agent.InputEstimator {
	model := strings.ToLower(path.Base(strings.TrimSpace(config.Model)))
	var imageTokens func(int, int) int
	switch {
	case strings.HasPrefix(model, "claude-"):
		edge, patches := 1568, 1568
		// https://platform.claude.com/docs/en/build-with-claude/vision
		// Claude 4.7+ uses the high-resolution tier. Keep older named models
		// on their documented tier instead of inferring it from wire protocol.
		parts := strings.FieldsFunc(model, func(r rune) bool { return r == '-' || r == '.' })
		if len(parts) >= 3 {
			major, _ := strconv.Atoi(parts[2])
			minor := 0
			if len(parts) >= 4 {
				minor, _ = strconv.Atoi(parts[3])
			}
			if major >= 5 || major == 4 && minor >= 7 {
				edge, patches = 2576, 4784
			}
		}
		imageTokens = func(w, h int) int { return imagePatches(w, h, 28, edge, patches) }
	case hasModelPrefix(model, "gpt-6-astra", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"):
		imageTokens = patchImageTokens(65535, 0, 1.2)
	case hasModelPrefix(model, "gpt-5.5"):
		imageTokens = patchImageTokens(6000, 10000, 1.2)
	case hasModelPrefix(model, "gpt-5.4"):
		imageTokens = patchImageTokens(2048, 2500, 1.2)
	case hasModelPrefix(model, "gpt-5.2"):
		imageTokens = patchImageTokens(2048, 6144, 1.2)
	case hasModelPrefix(model, "gpt-4.1-mini"):
		imageTokens = patchImageTokens(2048, 6144, 1.62)
	case hasModelPrefix(model, "gpt-4.1-nano"):
		imageTokens = patchImageTokens(2048, 1536, 2.46)
	case hasModelPrefix(model, "o4-mini"):
		imageTokens = patchImageTokens(2048, 1536, 1.72)
	case hasModelPrefix(model, "gpt-5-mini"):
		imageTokens = patchImageTokens(2048, 1536, 1.2)
	case hasModelPrefix(model, "gpt-5-nano"):
		imageTokens = patchImageTokens(2048, 1536, 1.5)
	case hasModelPrefix(model, "gpt-4o-mini"):
		imageTokens = tileImageTokens(2833, 5667)
	case hasModelPrefix(model, "gpt-4o", "gpt-4.1"):
		imageTokens = tileImageTokens(85, 170)
	case hasModelPrefix(model, "gpt-5.1") || model == "gpt-5" || strings.HasPrefix(model, "gpt-5-20"):
		imageTokens = tileImageTokens(70, 140)
	case hasModelPrefix(model, "o1", "o3"):
		imageTokens = tileImageTokens(75, 150)
	}
	return agent.InputEstimator{ImageTokens: imageTokens}
}

// https://developers.openai.com/api/docs/guides/images-vision#calculating-costs
func patchImageTokens(edge, patches int, multiplier float64) func(int, int) int {
	return func(w, h int) int {
		return int(math.Ceil(float64(imagePatches(w, h, 32, edge, patches)) * multiplier))
	}
}

func tileImageTokens(base, tile int) func(int, int) int {
	return func(w, h int) int {
		width, height := float64(w), float64(h)
		scale := min(1, 2048/max(width, height), 768/min(width, height))
		columns := int(math.Ceil(max(1, math.Floor(width*scale)) / 512))
		rows := int(math.Ceil(max(1, math.Floor(height*scale)) / 512))
		return base + tile*columns*rows
	}
}

// Find the largest aspect-preserving image that fits whole visual patches.
// Working in decoded pixel dimensions makes this independent of file encoding.
// Integer resizing can differ slightly from provider billing; this is a
// context-budget estimate, not a billing calculator.
func imagePatches(w, h, patch, edge, limit int) int {
	width, height := float64(w), float64(h)
	scale := min(1, float64(edge)/max(width, height))
	count := func(scale float64) int {
		columns := math.Ceil(max(1, math.Floor(width*scale)) / float64(patch))
		rows := math.Ceil(max(1, math.Floor(height*scale)) / float64(patch))
		return int(columns * rows)
	}
	if limit == 0 || count(scale) <= limit {
		return count(scale)
	}
	low, high := 0.0, scale
	for range 48 {
		middle := (low + high) / 2
		if count(middle) <= limit {
			low = middle
		} else {
			high = middle
		}
	}
	return count(low)
}
