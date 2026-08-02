package resourcecatalog

import imagepreset "denova/internal/image/preset"

func (s *Service) ImagePresets() ([]imagepreset.Preset, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.imagePresets.List()
}

func (s *Service) ImagePreset(id string) (imagepreset.Preset, error) {
	if err := s.ready(); err != nil {
		return imagepreset.Preset{}, err
	}
	return s.imagePresets.Get(id)
}

func (s *Service) CreateImagePreset(preset imagepreset.Preset) (imagepreset.Preset, error) {
	if err := s.ready(); err != nil {
		return imagepreset.Preset{}, err
	}
	return s.imagePresets.Create(preset)
}

func (s *Service) UpdateImagePreset(id string, preset imagepreset.Preset, baseRevision string) (imagepreset.Preset, error) {
	if err := s.ready(); err != nil {
		return imagepreset.Preset{}, err
	}
	return s.imagePresets.Update(id, preset, baseRevision)
}

func (s *Service) DeleteImagePreset(id string) error {
	if err := s.ready(); err != nil {
		return err
	}
	return s.imagePresets.Delete(id)
}
