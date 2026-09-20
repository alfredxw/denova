package config

// SpeechSettings is the independent user-scoped OpenAI-compatible speech
// endpoint. It is never included in Agent model settings or story context.
type SpeechSettings struct {
	Endpoint string `toml:"endpoint" json:"endpoint"`
	APIKey   string `toml:"api_key" json:"api_key"`
	Model    string `toml:"model" json:"model"`
	Voice    string `toml:"voice" json:"voice"`
}
