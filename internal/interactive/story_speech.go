package interactive

import "errors"

var ErrSpeechContentMode = errors.New("invalid speech content mode")

// StorySpeechSettings contains playback preferences only. The journal owns
// persistence; credentials, voice selection, and audio never belong to a story.
type StorySpeechSettings struct {
	AutoRead        bool   `json:"auto_read"`
	Mode            string `json:"mode"`
	IgnoreAsterisks bool   `json:"ignore_asterisks"`
}

func normalizeStorySpeechSettings(settings StorySpeechSettings) StorySpeechSettings {
	if settings.Mode == "" {
		settings.Mode = "all"
	}
	return settings
}

func validateStorySpeechSettings(settings StorySpeechSettings) error {
	switch settings.Mode {
	case "", "all", "quoted":
		return nil
	default:
		return ErrSpeechContentMode
	}
}
