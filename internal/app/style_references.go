package app

import (
	"denova/internal/styleref"
)

func (a *App) StyleReferences() ([]styleref.Reference, error) {
	return a.interactiveService().StyleReferences()
}

func (s *InteractiveAppService) StyleReferences() ([]styleref.Reference, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return nil, ErrNoWorkspace
	}
	return styleref.NewLibrary(cfg.NovaDir).List()
}

func (a *App) SaveStyleReference(req styleref.WriteRequest) (styleref.Reference, error) {
	return a.interactiveService().SaveStyleReference(req)
}

func (s *InteractiveAppService) SaveStyleReference(req styleref.WriteRequest) (styleref.Reference, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return styleref.Reference{}, ErrNoWorkspace
	}
	return styleref.NewLibrary(cfg.NovaDir).Write(req)
}

func (a *App) StyleReferenceFile(path string) (styleref.FileDocument, error) {
	return a.interactiveService().StyleReferenceFile(path)
}

func (s *InteractiveAppService) StyleReferenceFile(path string) (styleref.FileDocument, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return styleref.FileDocument{}, ErrNoWorkspace
	}
	return styleref.NewLibrary(cfg.NovaDir).Read(path)
}

func (a *App) UpdateStyleReferenceFile(req styleref.UpdateRequest) (styleref.FileDocument, error) {
	return a.interactiveService().UpdateStyleReferenceFile(req)
}

func (s *InteractiveAppService) UpdateStyleReferenceFile(req styleref.UpdateRequest) (styleref.FileDocument, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return styleref.FileDocument{}, ErrNoWorkspace
	}
	return styleref.NewLibrary(cfg.NovaDir).Update(req)
}

func (a *App) DeleteStyleReference(path string) error {
	return a.interactiveService().DeleteStyleReference(path)
}

func (s *InteractiveAppService) DeleteStyleReference(path string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return ErrNoWorkspace
	}
	return styleref.NewLibrary(cfg.NovaDir).Delete(path)
}
