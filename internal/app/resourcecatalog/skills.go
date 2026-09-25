package resourcecatalog

import (
	"context"
	"fmt"
	"log/slog"

	novaskills "denova/internal/agents/skills"
)

func (s *Service) SkillSnapshot(ctx context.Context, target SkillTarget) (novaskills.Snapshot, error) {
	directories, err := s.skillDirectories(target)
	if err != nil {
		return novaskills.Snapshot{}, err
	}
	return novaskills.SnapshotFor(ctx, directories)
}

func (s *Service) SkillDocument(ctx context.Context, target SkillTarget, scope novaskills.Scope, name string) (novaskills.Document, error) {
	directories, err := s.skillDirectories(target)
	if err != nil {
		return novaskills.Document{}, err
	}
	return novaskills.ReadDocument(ctx, directories, scope, name)
}

func (s *Service) SkillFileDocument(ctx context.Context, target SkillTarget, scope novaskills.Scope, name, path string) (novaskills.FileDocument, error) {
	directories, err := s.skillDirectories(target)
	if err != nil {
		return novaskills.FileDocument{}, err
	}
	return novaskills.ReadSkillFile(ctx, directories, scope, name, path)
}

func (s *Service) CreateSkill(ctx context.Context, target SkillTarget, scope novaskills.Scope, name string, metadata novaskills.CreateMetadata) (novaskills.Document, error) {
	directories, err := s.skillDirectories(target)
	if err != nil {
		return novaskills.Document{}, err
	}
	document, err := novaskills.CreateDocumentWithMetadata(ctx, directories, scope, name, metadata)
	if err != nil {
		return novaskills.Document{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[app/resourcecatalog] Skill created scope=%s name=%s path=%s", scope, name, document.Path))
	return document, nil
}

func (s *Service) SaveSkillFile(ctx context.Context, target SkillTarget, scope novaskills.Scope, name, path, content, baseRevision string) (novaskills.FileDocument, error) {
	directories, err := s.skillDirectories(target)
	if err != nil {
		return novaskills.FileDocument{}, err
	}
	document, err := novaskills.SaveSkillFileIfRevision(ctx, directories, scope, name, path, content, baseRevision)
	if err != nil {
		return novaskills.FileDocument{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[app/resourcecatalog] Skill file saved scope=%s name=%s file=%s", scope, name, path))
	return document, nil
}

func (s *Service) SaveSkillAs(ctx context.Context, target SkillTarget, scope novaskills.Scope, name string, targetScope novaskills.Scope, targetName, content, baseRevision string) (novaskills.Document, error) {
	directories, err := s.skillDirectories(target)
	if err != nil {
		return novaskills.Document{}, err
	}
	document, err := novaskills.SaveDocumentAsIfRevision(ctx, directories, scope, name, targetScope, targetName, content, baseRevision)
	if err != nil {
		return novaskills.Document{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[app/resourcecatalog] Skill saved as source_scope=%s source_name=%s target_scope=%s target_name=%s path=%s", scope, name, targetScope, targetName, document.Path))
	return document, nil
}

func (s *Service) DeleteSkill(ctx context.Context, target SkillTarget, scope novaskills.Scope, name string) error {
	directories, err := s.skillDirectories(target)
	if err != nil {
		return err
	}
	if err := novaskills.DeleteDocument(ctx, directories, scope, name); err != nil {
		return err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[app/resourcecatalog] Skill deleted scope=%s name=%s", scope, name))
	return nil
}
func (s *Service) skillDirectories(target SkillTarget) ([]novaskills.Directory, error) {
	if s == nil || s.skillSource == nil {
		return nil, fmt.Errorf("Skill directory source is not configured")
	}
	return s.skillSource.SkillDirectories(target)
}
