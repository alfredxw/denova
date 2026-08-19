package generation

import (
	"math"
	"strconv"
	"strings"
)

func parseImageDimensions(value string) (int, int, bool) {
	left, right, ok := strings.Cut(strings.ToLower(strings.TrimSpace(value)), "x")
	if !ok {
		return 0, 0, false
	}
	width, widthErr := strconv.Atoi(strings.TrimSpace(left))
	height, heightErr := strconv.Atoi(strings.TrimSpace(right))
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func aspectRatioValue(value string) float64 {
	left, right, ok := strings.Cut(strings.TrimSpace(value), ":")
	if !ok {
		return 0
	}
	width, widthErr := strconv.ParseFloat(strings.TrimSpace(left), 64)
	height, heightErr := strconv.ParseFloat(strings.TrimSpace(right), 64)
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return 0
	}
	return width / height
}

func closestAspectRatio(size, requested string, supported []string) string {
	requested = strings.TrimSpace(requested)
	for _, candidate := range supported {
		if candidate == requested {
			return requested
		}
	}
	target := aspectRatioValue(requested)
	if width, height, ok := parseImageDimensions(size); ok {
		target = float64(width) / float64(height)
	}
	if target <= 0 || len(supported) == 0 {
		return ""
	}
	best := supported[0]
	bestDistance := math.MaxFloat64
	for _, candidate := range supported {
		ratio := aspectRatioValue(candidate)
		if ratio <= 0 {
			continue
		}
		distance := math.Abs(math.Log(target / ratio))
		if distance < bestDistance {
			best = candidate
			bestDistance = distance
		}
	}
	return best
}
