package documentreview

import (
	"path/filepath"
	"strings"
)

const maxTargetIDBytes = 16 * 1024

// NormalizeTarget validates and canonicalizes the stable identity stored in
// the review ledger. Resource-specific existence checks belong to the snapshot
// resolver; this function only owns the cross-resource identity contract.
func NormalizeTarget(target Target) (Target, error) {
	target.Kind = strings.TrimSpace(target.Kind)
	target.ID = strings.TrimSpace(target.ID)
	target.Field = strings.TrimSpace(target.Field)
	if target.ID == "" || len(target.ID) > maxTargetIDBytes {
		return Target{}, newError(ErrorCodeInvalid, "review comment target is invalid", nil)
	}
	switch target.Kind {
	case TargetKindWorkspaceFile:
		if target.Field != "" {
			return Target{}, newError(ErrorCodeInvalid, "workspace file review target does not accept a field", map[string]any{"field": target.Field})
		}
		nativePath := filepath.Clean(filepath.FromSlash(target.ID))
		if nativePath == "." || filepath.IsAbs(nativePath) || nativePath == ".." || strings.HasPrefix(nativePath, ".."+string(filepath.Separator)) || strings.ContainsRune(nativePath, '\x00') {
			return Target{}, newError(ErrorCodeInvalid, "review comment target is invalid", nil)
		}
		target.ID = filepath.ToSlash(nativePath)
	case TargetKindLoreItem:
		if target.Field == "" {
			target.Field = TargetFieldLoreContent
		}
		if target.Field != TargetFieldLoreContent {
			return Target{}, newError(ErrorCodeInvalid, "unsupported lore review field", map[string]any{"field": target.Field})
		}
	default:
		return Target{}, newError(ErrorCodeInvalid, "unsupported review target kind", map[string]any{"kind": target.Kind})
	}
	return target, nil
}
