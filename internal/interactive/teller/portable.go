package teller

// PreparePortable validates a native narrative preset without saving it, so a
// host can assign a content digest after native normalization has completed.
func PreparePortable(value Definition) (Definition, error) {
	value = Normalize(value)
	return value, validateTeller(value)
}
