package app

import (
	"denova/internal/style"
)

func (a *App) StyleReferences() ([]style.Reference, error) {
	return a.interactiveService().StyleReferences()
}

func (s *InteractiveAppService) StyleReferences() ([]style.Reference, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return nil, ErrNoWorkspace
	}
	return style.NewLibrary(cfg.DataDir()).List()
}

func (a *App) SaveStyleReference(req style.WriteRequest) (style.Reference, error) {
	return a.interactiveService().SaveStyleReference(req)
}

func (s *InteractiveAppService) SaveStyleReference(req style.WriteRequest) (style.Reference, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return style.Reference{}, ErrNoWorkspace
	}
	return style.NewLibrary(cfg.DataDir()).Write(req)
}

func (a *App) StyleReferenceFile(path string) (style.FileDocument, error) {
	return a.interactiveService().StyleReferenceFile(path)
}

func (s *InteractiveAppService) StyleReferenceFile(path string) (style.FileDocument, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return style.FileDocument{}, ErrNoWorkspace
	}
	return style.NewLibrary(cfg.DataDir()).Read(path)
}

func (a *App) UpdateStyleReferenceFile(req style.UpdateRequest) (style.FileDocument, error) {
	return a.interactiveService().UpdateStyleReferenceFile(req)
}

func (s *InteractiveAppService) UpdateStyleReferenceFile(req style.UpdateRequest) (style.FileDocument, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return style.FileDocument{}, ErrNoWorkspace
	}
	return style.NewLibrary(cfg.DataDir()).Update(req)
}

func (a *App) DeleteStyleReference(path string) error {
	return a.interactiveService().DeleteStyleReference(path)
}

func (s *InteractiveAppService) DeleteStyleReference(path string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return ErrNoWorkspace
	}
	return style.NewLibrary(cfg.DataDir()).Delete(path)
}
