package app

import (
	"context"
	"fmt"
	"path/filepath"

	"denova/internal/book"
	"denova/internal/book/lore"
	"denova/internal/workspace/documentreview"
)

// documentReviewTargetResolver is the application-level adapter between the
// generic review domain and workspace-owned text resources. Keeping resource
// reads here prevents the review ledger from depending on book storage.
type documentReviewTargetResolver struct {
	files *book.Service
	lore  *lore.Store
}

func newDocumentReviewTargetResolver(workspace string, files *book.Service) documentreview.SnapshotResolver {
	return documentReviewTargetResolver{
		files: files,
		lore:  lore.NewStore(workspace),
	}
}

func (r documentReviewTargetResolver) ResolveReviewTarget(_ context.Context, target documentreview.Target) (documentreview.ResolvedTarget, error) {
	normalized, err := documentreview.NormalizeTarget(target)
	if err != nil {
		return documentreview.ResolvedTarget{}, err
	}
	switch normalized.Kind {
	case documentreview.TargetKindWorkspaceFile:
		if r.files == nil {
			return documentreview.ResolvedTarget{}, fmt.Errorf("resolve workspace file review target: book service is nil")
		}
		content, revision, err := r.files.ReadFileWithRevision(normalized.ID)
		if err != nil {
			return documentreview.ResolvedTarget{}, err
		}
		return documentreview.ResolvedTarget{
			Target: normalized,
			Name:   filepath.Base(filepath.FromSlash(normalized.ID)),
			Snapshot: documentreview.Snapshot{
				Content: content, Revision: revision,
			},
		}, nil
	case documentreview.TargetKindLoreItem:
		if r.lore == nil {
			return documentreview.ResolvedTarget{}, fmt.Errorf("resolve lore review target: lore store is nil")
		}
		item, err := r.lore.Get(normalized.ID)
		if err != nil {
			return documentreview.ResolvedTarget{}, err
		}
		resolved := documentreview.ResolvedTarget{
			Target: normalized,
			Name:   item.Name,
			Snapshot: documentreview.Snapshot{
				Content: item.Content, Revision: item.UpdatedAt,
			},
		}
		if !item.Enabled {
			snapshot := resolved.Snapshot
			resolved.ContextSnapshot = &snapshot
		}
		return resolved, nil
	default:
		return documentreview.ResolvedTarget{}, fmt.Errorf("unsupported document review target kind %q", normalized.Kind)
	}
}
