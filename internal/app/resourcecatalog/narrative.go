package resourcecatalog

import (
	"denova/internal/interactive/teller"
	"denova/internal/style"
)

func (s *Service) Tellers() ([]teller.Definition, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.tellers.List()
}

func (s *Service) Teller(id string) (teller.Definition, error) {
	if err := s.ready(); err != nil {
		return teller.Definition{}, err
	}
	return s.tellers.Get(id)
}

func (s *Service) CreateTeller(definition teller.Definition) (teller.Definition, error) {
	if err := s.ready(); err != nil {
		return teller.Definition{}, err
	}
	return s.tellers.Create(definition)
}

func (s *Service) UpdateTeller(id string, definition teller.Definition, baseRevision string) (teller.Definition, error) {
	if err := s.ready(); err != nil {
		return teller.Definition{}, err
	}
	return s.tellers.Update(id, definition, baseRevision)
}

func (s *Service) DeleteTeller(id string) error {
	if err := s.ready(); err != nil {
		return err
	}
	return s.tellers.Delete(id)
}

func (s *Service) StyleReferences() ([]style.Reference, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.styleReferences.List()
}

func (s *Service) SaveStyleReference(request style.WriteRequest) (style.Reference, error) {
	if err := s.ready(); err != nil {
		return style.Reference{}, err
	}
	return s.styleReferences.Write(request)
}

func (s *Service) StyleReferenceFile(path string) (style.FileDocument, error) {
	if err := s.ready(); err != nil {
		return style.FileDocument{}, err
	}
	return s.styleReferences.Read(path)
}

func (s *Service) UpdateStyleReferenceFile(request style.UpdateRequest) (style.FileDocument, error) {
	if err := s.ready(); err != nil {
		return style.FileDocument{}, err
	}
	return s.styleReferences.Update(request)
}

func (s *Service) DeleteStyleReference(path string) error {
	if err := s.ready(); err != nil {
		return err
	}
	return s.styleReferences.Delete(path)
}
