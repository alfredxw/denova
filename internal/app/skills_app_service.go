package app

import (
	"context"
	"log"

	novaskills "nova/internal/skills"
)

// SkillsAppService exposes user and workspace skill management.
type SkillsAppService struct {
	app *App
}

func (a *App) SkillSnapshot(ctx context.Context) (novaskills.Snapshot, error) {
	return a.skills().Snapshot(ctx)
}

func (a *App) SkillDocument(ctx context.Context, scope novaskills.Scope, name string) (novaskills.Document, error) {
	return a.skills().Document(ctx, scope, name)
}

func (a *App) CreateSkillDocument(ctx context.Context, scope novaskills.Scope, name, description string, agents []string) (novaskills.Document, error) {
	return a.skills().Create(ctx, scope, name, description, agents)
}

func (a *App) SaveSkillDocument(ctx context.Context, scope novaskills.Scope, name, content string) (novaskills.Document, error) {
	return a.skills().Save(ctx, scope, name, content)
}

func (a *App) SaveSkillDocumentAs(ctx context.Context, scope novaskills.Scope, name string, targetScope novaskills.Scope, targetName, content string) (novaskills.Document, error) {
	return a.skills().SaveAs(ctx, scope, name, targetScope, targetName, content)
}

func (a *App) DeleteSkillDocument(ctx context.Context, scope novaskills.Scope, name string) error {
	return a.skills().Delete(ctx, scope, name)
}

func (s *SkillsAppService) Snapshot(ctx context.Context) (novaskills.Snapshot, error) {
	return novaskills.SnapshotFor(ctx, s.directories())
}

func (s *SkillsAppService) Document(ctx context.Context, scope novaskills.Scope, name string) (novaskills.Document, error) {
	return novaskills.ReadDocument(ctx, s.directories(), scope, name)
}

func (s *SkillsAppService) Create(ctx context.Context, scope novaskills.Scope, name, description string, agents []string) (novaskills.Document, error) {
	doc, err := novaskills.CreateDocument(ctx, s.directories(), scope, name, description, agents...)
	if err != nil {
		return novaskills.Document{}, err
	}
	log.Printf("[skills] Skill created scope=%s name=%s path=%s", scope, name, doc.Path)
	return doc, nil
}

func (s *SkillsAppService) Save(ctx context.Context, scope novaskills.Scope, name, content string) (novaskills.Document, error) {
	doc, err := novaskills.SaveDocument(ctx, s.directories(), scope, name, content)
	if err != nil {
		return novaskills.Document{}, err
	}
	log.Printf("[skills] Skill saved scope=%s name=%s path=%s", scope, name, doc.Path)
	return doc, nil
}

func (s *SkillsAppService) SaveAs(ctx context.Context, scope novaskills.Scope, name string, targetScope novaskills.Scope, targetName, content string) (novaskills.Document, error) {
	doc, err := novaskills.SaveDocumentAs(ctx, s.directories(), scope, name, targetScope, targetName, content)
	if err != nil {
		return novaskills.Document{}, err
	}
	log.Printf("[skills] Skill saved as source_scope=%s source_name=%s target_scope=%s target_name=%s path=%s", scope, name, targetScope, targetName, doc.Path)
	return doc, nil
}

func (s *SkillsAppService) Delete(ctx context.Context, scope novaskills.Scope, name string) error {
	if err := novaskills.DeleteDocument(ctx, s.directories(), scope, name); err != nil {
		return err
	}
	log.Printf("[skills] Skill deleted scope=%s name=%s", scope, name)
	return nil
}

func (s *SkillsAppService) directories() []novaskills.Directory {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.cfg == nil {
		return nil
	}
	return novaskills.NewDirectories(a.cfg.SkillsDir, a.cfg.NovaDir, a.workspace)
}
