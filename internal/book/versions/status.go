package versions

func (s *Service) Status(settings VersionAutoSettings) (VersionStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked(settings)
}

func (s *Service) statusLocked(settings VersionAutoSettings) (VersionStatus, error) {
	snapshot, err := s.collectWorkspaceSnapshot(nil)
	if err != nil {
		return VersionStatus{}, err
	}
	current, err := s.headVersion()
	if err != nil {
		return VersionStatus{}, err
	}
	changes := []VersionChange{}
	if current != nil {
		changes, err = s.diffChangesFromSnapshot(snapshot, current.ID)
		if err != nil {
			return VersionStatus{}, err
		}
	} else {
		changes = make([]VersionChange, 0, len(snapshot.files))
		for _, file := range snapshot.files {
			changes = append(changes, VersionChange{Path: file.Path, Status: "added"})
		}
	}
	lastAutoAt, _, err := s.latestVersionTimes()
	if err != nil {
		return VersionStatus{}, err
	}
	settings = normalizeVersionAutoSettings(settings)
	return VersionStatus{
		HasVersions: current != nil,
		Clean:       len(changes) == 0,
		Changes:     changes,
		Latest:      current,
		Auto: VersionAutoInfo{
			TimedEnabled:         settings.TimedEnabled,
			TimedIntervalMinutes: settings.TimedIntervalMinutes,
			Retention:            settings.Retention,
			LastAutoAt:           lastAutoAt,
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
	items, err := s.loadVersionHistory(limit)
	if err != nil {
		return nil, err
	}
	return items, nil
}
