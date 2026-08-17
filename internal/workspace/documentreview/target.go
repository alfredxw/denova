package documentreview

import (
	"path"
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
		hasWindowsDrivePrefix := len(target.ID) >= 2 && target.ID[1] == ':' &&
			((target.ID[0] >= 'a' && target.ID[0] <= 'z') || (target.ID[0] >= 'A' && target.ID[0] <= 'Z'))
		cleanPath := path.Clean(target.ID)
		if cleanPath == "." || path.IsAbs(target.ID) || hasWindowsDrivePrefix || strings.ContainsRune(target.ID, '\\') ||
			cleanPath == ".." || strings.HasPrefix(cleanPath, "../") || strings.ContainsRune(target.ID, '\x00') {
			return Target{}, newError(ErrorCodeInvalid, "review comment target is invalid", nil)
		}
		target.ID = cleanPath
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
