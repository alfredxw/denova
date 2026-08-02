package resourcecatalog

import (
	"context"
	"fmt"
	"log/slog"

	novaskills "denova/internal/agents/skills"
)

func (s *Service) SkillSnapshot(ctx context.Context) (novaskills.Snapshot, error) {
	return novaskills.SnapshotFor(ctx, s.skillDirectories())
}

func (s *Service) SkillDocument(ctx context.Context, scope novaskills.Scope, name string) (novaskills.Document, error) {
	return novaskills.ReadDocument(ctx, s.skillDirectories(), scope, name)
}

func (s *Service) SkillFileDocument(ctx context.Context, scope novaskills.Scope, name, path string) (novaskills.FileDocument, error) {
	return novaskills.ReadSkillFile(ctx, s.skillDirectories(), scope, name, path)
}

func (s *Service) CreateSkill(ctx context.Context, scope novaskills.Scope, name string, metadata novaskills.CreateMetadata) (novaskills.Document, error) {
	document, err := novaskills.CreateDocumentWithMetadata(ctx, s.skillDirectories(), scope, name, metadata)
	if err != nil {
		return novaskills.Document{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[app/resourcecatalog] Skill created scope=%s name=%s path=%s", scope, name, document.Path))
	return document, nil
}

func (s *Service) SaveSkillFile(ctx context.Context, scope novaskills.Scope, name, path, content, baseRevision string) (novaskills.FileDocument, error) {
	document, err := novaskills.SaveSkillFileIfRevision(ctx, s.skillDirectories(), scope, name, path, content, baseRevision)
	if err != nil {
		return novaskills.FileDocument{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[app/resourcecatalog] Skill file saved scope=%s name=%s file=%s", scope, name, path))
	return document, nil
}

func (s *Service) SaveSkillAs(ctx context.Context, scope novaskills.Scope, name string, targetScope novaskills.Scope, targetName, content, baseRevision string) (novaskills.Document, error) {
	document, err := novaskills.SaveDocumentAsIfRevision(ctx, s.skillDirectories(), scope, name, targetScope, targetName, content, baseRevision)
	if err != nil {
		return novaskills.Document{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[app/resourcecatalog] Skill saved as source_scope=%s source_name=%s target_scope=%s target_name=%s path=%s", scope, name, targetScope, targetName, document.Path))
	return document, nil
}

func (s *Service) DeleteSkill(ctx context.Context, scope novaskills.Scope, name string) error {
	if err := novaskills.DeleteDocument(ctx, s.skillDirectories(), scope, name); err != nil {
		return err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[app/resourcecatalog] Skill deleted scope=%s name=%s", scope, name))
	return nil
}

func (s *Service) PreviewSkillZip(ctx context.Context, scope novaskills.Scope, data []byte) (novaskills.InstallPreview, error) {
	return novaskills.PreviewZip(ctx, s.skillDirectories(), scope, data)
}

func (s *Service) InstallSkillZip(ctx context.Context, scope novaskills.Scope, data []byte, candidateIDs []string) (novaskills.InstallResult, error) {
	result, err := novaskills.InstallZip(ctx, s.skillDirectories(), scope, data, candidateIDs)
	if err != nil {
		return novaskills.InstallResult{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[app/resourcecatalog] Skills installed from zip scope=%s count=%d", scope, len(result.Installed)))
	return result, nil
}

func (s *Service) PreviewSkillRemoteArchive(ctx context.Context, scope novaskills.Scope, source novaskills.RemoteArchiveSource) (novaskills.InstallPreview, error) {
	return novaskills.PreviewRemoteArchive(ctx, s.skillDirectories(), scope, source)
}

func (s *Service) InstallSkillRemoteArchive(ctx context.Context, scope novaskills.Scope, source novaskills.RemoteArchiveSource, candidateIDs []string) (novaskills.InstallResult, error) {
	result, err := novaskills.InstallRemoteArchive(ctx, s.skillDirectories(), scope, source, candidateIDs)
	if err != nil {
		return novaskills.InstallResult{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[app/resourcecatalog] Skills installed from remote archive scope=%s count=%d", scope, len(result.Installed)))
	return result, nil
}

func (s *Service) PreviewSkillGitHub(ctx context.Context, scope novaskills.Scope, source novaskills.GitHubSource) (novaskills.InstallPreview, error) {
	return novaskills.PreviewGitHub(ctx, s.skillDirectories(), scope, source)
}

func (s *Service) InstallSkillGitHub(ctx context.Context, scope novaskills.Scope, source novaskills.GitHubSource, candidateIDs []string) (novaskills.InstallResult, error) {
	result, err := novaskills.InstallGitHub(ctx, s.skillDirectories(), scope, source, candidateIDs)
	if err != nil {
		return novaskills.InstallResult{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[app/resourcecatalog] Skills installed from GitHub scope=%s url=%q count=%d", scope, source.URL, len(result.Installed)))
	return result, nil
}

func (s *Service) skillDirectories() []novaskills.Directory {
	if s == nil || s.skillSource == nil {
		return nil
	}
	return s.skillSource.SkillDirectories()
}
