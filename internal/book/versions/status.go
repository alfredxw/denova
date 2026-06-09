package versions

func (s *Service) Status(settings VersionAutoSettings) (VersionStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked(settings)
}

func (s *Service) statusLocked(settings VersionAutoSettings) (VersionStatus, error) {
	current, err := s.headVersion()
	if err != nil {
		return VersionStatus{}, err
	}
	changes := []VersionChange{}
	if current != nil {
		changes, err = s.diffChanges(*current)
		if err != nil {
			return VersionStatus{}, err
		}
	} else {
		files, err := s.collectVisibleFiles()
		if err != nil {
			return VersionStatus{}, err
		}
		changes = make([]VersionChange, 0, len(files))
		for _, file := range files {
			changes = append(changes, VersionChange{Path: file.Path, Status: "added"})
		}
	}
	items, err := s.loadVersions()
	if err != nil {
		return VersionStatus{}, err
	}
	settings = normalizeVersionAutoSettings(settings)
	return VersionStatus{
		HasVersions: len(items) > 0,
		Clean:       len(changes) == 0,
		Changes:     changes,
		Latest:      current,
		Auto: VersionAutoInfo{
			TimedEnabled:         settings.TimedEnabled,
			TimedIntervalMinutes: settings.TimedIntervalMinutes,
			AgentEnabled:         settings.AgentEnabled,
			AgentCharThreshold:   settings.AgentCharThreshold,
			Retention:            settings.Retention,
			LastAutoAt:           lastAutoVersionAt(items),
		},
	}, nil
}

func (s *Service) History(limit int) ([]VersionEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 30
	}
	if limit > 200 {
		limit = 200
	}
	items, err := s.loadVersions()
	if err != nil {
		return nil, err
	}
	items = append([]VersionEntry(nil), items...)
	sortVersionsDesc(items)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}
