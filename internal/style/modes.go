// Package style owns narrative-style identity, mode availability, and the
// user-managed reference library. Prompt content remains with the interactive
// Teller library.
package style

const (
	DefaultID   = "rhythm"
	ModeWriting = "writing"
	ModeGame    = "game"
)

// AllModes returns a fresh slice so callers cannot mutate shared defaults.
func AllModes() []string {
	return []string{ModeWriting, ModeGame}
}

// NormalizeModes keeps supported values in their canonical order. A missing
// or unusable field is treated as both modes for legacy custom styles.
func NormalizeModes(modes []string) []string {
	writing, game := false, false
	for _, mode := range modes {
		switch mode {
		case ModeWriting:
			writing = true
		case ModeGame:
			game = true
		}
	}
	if !writing && !game {
		return AllModes()
	}
	result := make([]string, 0, 2)
	if writing {
		result = append(result, ModeWriting)
	}
	if game {
		result = append(result, ModeGame)
	}
	return result
}

// Supports reports whether a normalized or legacy mode list can be used by
// the requested runtime.
func Supports(modes []string, mode string) bool {
	for _, candidate := range NormalizeModes(modes) {
		if candidate == mode {
			return true
		}
	}
	return false
}
