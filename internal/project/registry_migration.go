package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	projectIDStateDirectoryRegistryVersion = 2
	readableStateDirectoryRegistryVersion  = 3
)

type legacyBookRecord struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	LastOpenedAt string `json:"last_opened_at"`
}

type legacyBookRegistry struct {
	Current  string             `json:"current"`
	Books    []legacyBookRecord `json:"books"`
	SortMode string             `json:"sort_mode"`
	Order    []string           `json:"order"`
	Hidden   []string           `json:"hidden"`
}

func (registry *Registry) loadOrMigrateLocked() (registryData, bool, error) {
	var data registryData
	raw, err := os.ReadFile(registry.path)
	if err == nil {
		if err := json.Unmarshal(raw, &data); err != nil {
			return registryData{}, false, fmt.Errorf("decode project registry: %w", err)
		}
		switch data.Version {
		case registryVersion:
			if err := registry.normalizeRegistryData(&data); err != nil {
				return registryData{}, false, err
			}
			if err := registry.migrateProjectStateStorageOnce(data.Projects); err != nil {
				return registryData{}, false, err
			}
			return data, false, nil
		case readableStateDirectoryRegistryVersion:
			if err := registry.assignLegacyProjectLocations(&data); err != nil {
				return registryData{}, false, err
			}
			if err := registry.normalizeRegistryData(&data); err != nil {
				return registryData{}, false, err
			}
			if err := registry.saveLocked(data); err != nil {
				return registryData{}, false, fmt.Errorf("persist portable Project locations: %w", err)
			}
			if err := registry.migrateProjectStateStorageOnce(data.Projects); err != nil {
				return registryData{}, false, err
			}
			return data, false, nil
		case projectIDStateDirectoryRegistryVersion:
			if err := registry.assignLegacyProjectLocations(&data); err != nil {
				return registryData{}, false, err
			}
			if err := registry.assignReadableStateDirNames(&data); err != nil {
				return registryData{}, false, err
			}
			if err := registry.normalizeRegistryData(&data); err != nil {
				return registryData{}, false, err
			}
			// Persist the mapping before moving state. If a move fails, the next
			// load reads the new mapping and safely resumes the remaining moves.
			if err := registry.saveLocked(data); err != nil {
				return registryData{}, false, fmt.Errorf("persist readable project state directories: %w", err)
			}
			if err := registry.migrateProjectStateStorageOnce(data.Projects); err != nil {
				return registryData{}, false, err
			}
			return data, false, nil
		default:
			return registryData{}, false, fmt.Errorf(
				"project registry version %d does not match supported version %d",
				data.Version,
				registryVersion,
			)
		}
	}
	if !errors.Is(err, os.ErrNotExist) {
		return registryData{}, false, err
	}
	data = registryData{Version: registryVersion, SortMode: SortRecent, Projects: []Record{}}
	migrated, err := registry.importLegacyBooksLocked(&data)
	if err != nil {
		return registryData{}, false, err
	}
	return data, migrated, nil
}

func (registry *Registry) assignLegacyProjectLocations(data *registryData) error {
	type assignment struct {
		location      ProjectLocation
		managedKey    string
		directManaged bool
	}
	assignments := make([]assignment, len(data.Projects))
	managedOwners := make(map[string]int, len(data.Projects))
	preferOwner := func(candidateIndex, ownerIndex int) bool {
		candidate := data.Projects[candidateIndex]
		owner := data.Projects[ownerIndex]
		candidateHarness := candidate.ID == HarnessProjectID || candidate.Type == TypeHarness
		ownerHarness := owner.ID == HarnessProjectID || owner.Type == TypeHarness
		if candidateHarness != ownerHarness {
			return candidateHarness
		}
		candidateCurrent := candidate.ID == data.CurrentBookID
		ownerCurrent := owner.ID == data.CurrentBookID
		if candidateCurrent != ownerCurrent {
			return candidateCurrent
		}
		if assignments[candidateIndex].directManaged != assignments[ownerIndex].directManaged {
			return assignments[candidateIndex].directManaged
		}
		candidateActive := candidate.ArchivedAt == nil
		ownerActive := owner.ArchivedAt == nil
		if candidateActive != ownerActive {
			return candidateActive
		}
		if !candidate.LastOpenedAt.Equal(owner.LastOpenedAt) {
			return candidate.LastOpenedAt.After(owner.LastOpenedAt)
		}
		return candidate.UpdatedAt.After(owner.UpdatedAt)
	}

	for index := range data.Projects {
		record := &data.Projects[index]
		legacyPath := strings.TrimSpace(record.WorkspacePath)
		location, err := registry.legacyProjectLocation(legacyPath)
		if err != nil {
			return fmt.Errorf("project %s: %w", record.ID, err)
		}
		directManaged := false
		if filepath.IsAbs(legacyPath) {
			directLocation, directErr := registry.locationForWorkspace(legacyPath)
			if directErr != nil {
				return fmt.Errorf("project %s: %w", record.ID, directErr)
			}
			directManaged = directLocation.Kind == LocationManaged
		}
		assignments[index] = assignment{location: location, directManaged: directManaged}
		if location.Kind != LocationManaged {
			continue
		}
		key, keyErr := projectLocationKey(location)
		if keyErr != nil {
			return fmt.Errorf("project %s: %w", record.ID, keyErr)
		}
		assignments[index].managedKey = key
		ownerIndex, exists := managedOwners[key]
		if !exists || preferOwner(index, ownerIndex) {
			managedOwners[key] = index
		}
	}

	for index := range data.Projects {
		selected := assignments[index]
		location := selected.location
		if location.Kind == LocationManaged {
			if managedOwners[selected.managedKey] != index {
				// Repeated host-path prefixing in older releases could create several
				// Project identities for one managed directory. Keep every identity and
				// its state, but only let the authoritative record claim live content.
				location = legacyExternalProjectLocation(data.Projects[index].WorkspacePath)
			}
		}
		data.Projects[index].Location = location
		data.Projects[index].WorkspacePath = ""
		data.Projects[index].Status = ""
	}
	return nil
}

// legacyProjectLocation recognizes a Project previously stored below the
// managed projects directory without interpreting a foreign absolute path as
// a path on the current host. External paths are otherwise kept opaque.
func (registry *Registry) legacyProjectLocation(value string) (ProjectLocation, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ProjectLocation{}, fmt.Errorf("legacy Project path is required")
	}
	if filepath.IsAbs(value) {
		location, err := registry.locationForWorkspace(value)
		if err != nil {
			return ProjectLocation{}, err
		}
		if location.Kind == LocationManaged {
			return location, nil
		}
	}

	portable := strings.ReplaceAll(value, `\`, "/")
	marker := "/" + ContentDirectoryName + "/"
	if markerIndex := strings.LastIndex(strings.ToLower(portable), marker); markerIndex >= 0 {
		relative := ContentDirectoryName + "/" + strings.TrimPrefix(portable[markerIndex+len(marker):], "/")
		managed, found, err := registry.existingManagedLegacyLocation(relative)
		if err != nil {
			return ProjectLocation{}, err
		}
		if found {
			return ProjectLocation{Kind: LocationManaged, Path: managed}, nil
		}
	}
	// v0.3.3 also allowed managed Books as immediate DataRoot children. Once
	// the root moves, only the portable basename remains comparable. Reuse the
	// uniquely case-folded current directory when it is still a Book workspace.
	rootRelative := path.Base(strings.TrimSuffix(portable, "/"))
	managed, found, err := registry.existingManagedLegacyLocation(rootRelative)
	if err != nil {
		return ProjectLocation{}, err
	}
	if found && !isUserDataDirectory(managed) && isBookWorkspace(filepath.Join(registry.denovaDir, filepath.FromSlash(managed))) {
		return ProjectLocation{Kind: LocationManaged, Path: managed}, nil
	}

	return legacyExternalProjectLocation(value), nil
}

func legacyExternalProjectLocation(value string) ProjectLocation {
	value = strings.TrimSpace(value)
	if filepath.IsAbs(value) {
		value = filepath.Clean(value)
	}
	// filepath.IsAbs intentionally returns false for absolute paths using a
	// foreign operating system's syntax. Keep those bytes unchanged.
	return ProjectLocation{Kind: LocationExternal, Path: value}
}

func (registry *Registry) existingManagedLegacyLocation(relative string) (string, bool, error) {
	normalized, err := normalizeManagedLocationPath(relative)
	if err != nil || normalized == "." {
		return "", false, nil
	}
	current := registry.denovaDir
	resolved := make([]string, 0, strings.Count(normalized, "/")+1)
	for _, component := range strings.Split(normalized, "/") {
		entries, readErr := os.ReadDir(current)
		if errors.Is(readErr, os.ErrNotExist) {
			return "", false, nil
		}
		if readErr != nil {
			return "", false, readErr
		}
		match := ""
		for _, entry := range entries {
			if stateDirNameKey(entry.Name()) != stateDirNameKey(component) {
				continue
			}
			if match != "" && match != entry.Name() {
				return "", false, fmt.Errorf("multiple managed Project paths match legacy component %q", component)
			}
			match = entry.Name()
		}
		if match == "" {
			return "", false, nil
		}
		resolved = append(resolved, match)
		current = filepath.Join(current, match)
	}
	if !projectDirectoryExists(current) {
		return "", false, nil
	}
	managed := strings.Join(resolved, "/")
	if _, err := normalizeManagedLocationPath(managed); err != nil {
		return "", false, err
	}
	return managed, true, nil
}

func (registry *Registry) importLegacyBooksLocked(data *registryData) (bool, error) {
	legacy, found, err := registry.readLegacyBookRegistryLocked()
	if err != nil || !found {
		return false, err
	}
	now := time.Now().UTC()
	hiddenLocations := registry.legacyProjectLocationKeys(legacy.Hidden)
	locationToID := make(map[string]string, len(legacy.Books)+len(legacy.Hidden))
	activeIDs := make(map[string]bool, len(legacy.Books))
	for _, book := range legacy.Books {
		location, locationErr := registry.legacyProjectLocation(book.Path)
		if locationErr != nil {
			continue
		}
		key, keyErr := projectLocationKey(location)
		if keyErr != nil || locationToID[key] != "" {
			continue
		}
		record, recordErr := registry.newRecordAtLocation(data, location, TypeBook, book.Name, now)
		if recordErr != nil {
			return false, recordErr
		}
		if opened, parseErr := time.Parse(time.RFC3339Nano, book.LastOpenedAt); parseErr == nil {
			record.LastOpenedAt = opened.UTC()
		}
		_, hidden := hiddenLocations[key]
		if hidden || projectStatus(record).Status != StatusAvailable {
			record.ArchivedAt = timePointer(now)
		} else {
			activeIDs[record.ID] = true
		}
		data.Projects = append(data.Projects, record)
		locationToID[key] = record.ID
	}

	// Hidden books were not always present in legacy.Books. Persist an archived
	// tombstone so automatic Book discovery cannot resurrect them.
	for _, legacyPath := range legacy.Hidden {
		location, locationErr := registry.legacyProjectLocation(legacyPath)
		if locationErr != nil {
			continue
		}
		key, keyErr := projectLocationKey(location)
		if keyErr != nil || locationToID[key] != "" {
			continue
		}
		record, recordErr := registry.newRecordAtLocation(data, location, TypeBook, projectLocationBase(location), now)
		if recordErr != nil {
			return false, recordErr
		}
		record.ArchivedAt = timePointer(now)
		data.Projects = append(data.Projects, record)
		locationToID[key] = record.ID
	}

	for _, legacyPath := range legacy.Order {
		if location, locationErr := registry.legacyProjectLocation(legacyPath); locationErr == nil {
			if key, keyErr := projectLocationKey(location); keyErr == nil && locationToID[key] != "" {
				data.Order = append(data.Order, locationToID[key])
			}
		}
	}
	if location, locationErr := registry.legacyProjectLocation(legacy.Current); locationErr == nil {
		if key, keyErr := projectLocationKey(location); keyErr == nil {
			if id := locationToID[key]; activeIDs[id] {
				data.CurrentBookID = id
			}
		}
	}
	data.SortMode = normalizedSortMode(SortMode(legacy.SortMode), len(data.Order) > 0)
	return len(data.Projects) > 0 || len(legacy.Hidden) > 0, nil
}

func (registry *Registry) readLegacyBookRegistryLocked() (legacyBookRegistry, bool, error) {
	raw, err := os.ReadFile(registry.legacyBooksPath)
	if errors.Is(err, os.ErrNotExist) {
		return legacyBookRegistry{}, false, nil
	}
	if err != nil {
		return legacyBookRegistry{}, false, err
	}
	var legacy legacyBookRegistry
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return legacyBookRegistry{}, false, fmt.Errorf("decode legacy book registry: %w", err)
	}
	return legacy, true, nil
}

func (registry *Registry) legacyProjectLocationKeys(paths []string) map[string]struct{} {
	locations := make(map[string]struct{}, len(paths))
	for _, legacyPath := range paths {
		if location, err := registry.legacyProjectLocation(legacyPath); err == nil {
			if key, keyErr := projectLocationKey(location); keyErr == nil {
				locations[key] = struct{}{}
			}
		}
	}
	return locations
}

func projectDirectoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func timePointer(value time.Time) *time.Time {
	return &value
}
