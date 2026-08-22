package config

import "os"

const (
	AtlasCloudAPIKeyEnv     = "ATLASCLOUD_API_KEY"
	AtlasCloudAPIBaseEnv    = "ATLASCLOUD_API_BASE"
	AtlasCloudLegacyBaseEnv = "ATLASCLOUD_BASE_URL"
	AtlasCloudModelEnv      = "ATLASCLOUD_MODEL"
	AtlasCloudAPIBaseURL    = "https://api.atlascloud.ai/v1"
	AtlasCloudDefaultModel  = "qwen/qwen3.5-flash"
)

func atlasCloudEnvSettings() (apiKey, baseURL, model string, ok bool) {
	apiKey = os.Getenv(AtlasCloudAPIKeyEnv)
	baseURL = firstNonEmpty(os.Getenv(AtlasCloudAPIBaseEnv), os.Getenv(AtlasCloudLegacyBaseEnv))
	model = os.Getenv(AtlasCloudModelEnv)
	ok = apiKey != "" || baseURL != "" || model != ""
	return apiKey, baseURL, model, ok
}
