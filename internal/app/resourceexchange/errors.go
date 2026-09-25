package resourceexchange

import "errors"

// Sentinel errors identify decisions the user can resolve without exposing host paths.
var (
	ErrLocalModified    = errors.New("local resource changed; explicit replacement required")
	ErrResourceOwned    = errors.New("resource belongs to another tracked installation")
	ErrSkillExists      = errors.New("Skill already exists; choose another name")
	ErrSourceChanged    = errors.New("source identity changed; stop tracking before adopting another source")
	ErrBundleOwned      = errors.New("extension belongs to a resource bundle; update that installation")
	ErrResourcesBusy    = errors.New("resources are in use by a running task")
	ErrReferenceChanged = errors.New("referenced extension changed or is unavailable")
)
