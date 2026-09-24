package preset

// PreparePortable applies the native editor's bounds and normalization before
// a portable preset is given a content-addressed ID. It performs no writes.
func PreparePortable(value Preset) (Preset, error) {
	if err := validatePresetWriteBounds(value); err != nil {
		return Preset{}, err
	}
	value = normalizePreset(value)
	return value, validatePreset(value)
}
