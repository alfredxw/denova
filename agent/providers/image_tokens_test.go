package providers

import "testing"

func TestImageTokensFollowNativeModelResolution(t *testing.T) {
	for _, test := range []struct {
		model         string
		width, height int
		want          int
	}{
		{"claude-sonnet-4-6", 768, 768, 784},
		{"claude-sonnet-4-6", 1000, 1000, 1296},
		{"claude-sonnet-4-6", 2000, 1500, 1564},
		{"claude-sonnet-4-6", 3840, 2160, 1560},
		{"claude-opus-4-7", 3840, 2160, 4784},
		{"claude-opus-5", 2000, 1500, 3888},
		{"proxy/claude-opus-4-7", 3840, 2160, 4784},
		{"gpt-4o", 768, 768, 765},
		{"gpt-4o", 31, 47, 255},
		{"gpt-4o-2024-08-06", 2048, 4096, 1105},
		{"gpt-4o-mini", 768, 768, 25501},
		{"gpt-5.1", 768, 768, 630},
		{"gpt-5.4", 1024, 1024, 1229},
		{"gpt-5.4-mini", 2048, 2048, 3000},
		{"gpt-5.5", 2048, 2048, 4916},
		{"gpt-5.6-terra", 3840, 2160, 9792},
		{"gpt-6-astra", 3840, 2160, 9792},
		{"gpt-4.1-mini", 1024, 1024, 1659},
	} {
		t.Run(test.model, func(t *testing.T) {
			estimator := (ModelConfig{Provider: ProviderOpenAICompatible, Model: test.model}).InputEstimator()
			if estimator.ImageTokens == nil {
				t.Fatal("known model has no visual policy")
			}
			if got := estimator.ImageTokens(test.width, test.height); got != test.want {
				t.Fatalf("%dx%d image: %d tokens, want %d", test.width, test.height, got, test.want)
			}
			if got := estimator.ImageTokens(test.height, test.width); got != test.want {
				t.Fatalf("rotated image: %d tokens, want %d", got, test.want)
			}
		})
	}
}
