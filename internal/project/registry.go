package project

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	workspacelayout "denova/internal/workspace"
)

const registryVersion = 2

type registryData struct {
	Version       int      `json:"version"`
	CurrentBookID string   `json:"current_book_id,omitempty"`
	SortMode      SortMode `json:"sort_mode,omitempty"`
	Order         []string `json:"order,omitempty"`
	Projects      []Record `json:"projects"`
}

// Registry is the sole authority for Project identity, aliases and order.
// Filesystem availability is projected at read time rather than persisted as
// an event, so moving a directory never destroys its user-owned state.
type Registry struct {
	mu              sync.Mutex
	path            string
	legacyBooksPath string
	denovaDir       string
}

func NewRegistry(denovaDir string) *Registry {
	denovaDir = strings.TrimSpace(denovaDir)
	return &Registry{
		path:            filepath.Join(denovaDir, "projects.json"),
		legacyBooksPath: filepath.Join(denovaDir, "books.json"),
		denovaDir:       denovaDir,
	}
}

func (registry *Registry) List(includeArchived bool) ([]Record, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	data, changed, err := registry.loadAndDiscoverLocked()
	if err != nil {
		return nil, err
	}
	if changed {
		if err := registry.saveLocked(data); err != nil {
			return nil, err
		}
	}
	return orderedRecords(data, includeArchived), nil
}

func (registry *Registry) Get(id string) (Record, error) {
	if err := ValidateID(id); err != nil {
		return Record{}, err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	data, changed, err := registry.loadAndDiscoverLocked()
	if err != nil {
		return Record{}, err
	}
	if changed {
		if err := registry.saveLocked(data); err != nil {
			return Record{}, err
		}
	}
	for _, record := range data.Projects {
		if record.ID == id {
			return projectStatus(record), nil
		}
	}
	return Record{}, fmt.Errorf("project %s not found", id)
}

// Resolve returns one non-archived project and its durable layout. When
// requireAvailable is true, a missing content directory is rejected before a
// runtime can bind to it.
func (registry *Registry) Resolve(id string, requireAvailable bool) (Record, Layout, error) {
	if registry == nil {
		return Record{}, Layout{}, fmt.Errorf("project registry is unavailable")
	}
	id = strings.TrimSpace(id)
	record, err := registry.Get(id)
	if err != nil {
		return Record{}, Layout{}, err
	}
	if record.Status == StatusArchived {
		return Record{}, Layout{}, fmt.Errorf("project %s is archived", id)
	}
	if requireAvailable && record.Status != StatusAvailable {
		return Record{}, Layout{}, fmt.Errorf("project directory is unavailable: %s", record.WorkspacePath)
	}
	layout, err := registry.EnsureState(record)
	if err != nil {
		return Record{}, Layout{}, err
	}
	return record, layout, nil
}

// ResolveByPath upgrades a compatibility directory binding to the stable
// project identity used by all application services.
func (registry *Registry) ResolveByPath(path string, requireAvailable bool) (Record, Layout, error) {
	if registry == nil {
		return Record{}, Layout{}, fmt.Errorf("project registry is unavailable")
	}
	record, found, err := registry.FindByPath(path, false)
	if err != nil {
		return Record{}, Layout{}, err
	}
	if !found {
		return Record{}, Layout{}, fmt.Errorf("directory is not a registered project")
	}
	return registry.Resolve(record.ID, requireAvailable)
}

func (registry *Registry) FindByPath(path string, includeArchived bool) (Record, bool, error) {
	canonical, err := canonicalDirectory(path, false)
	if err != nil {
		return Record{}, false, err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	data, changed, err := registry.loadAndDiscoverLocked()
	if err != nil {
		return Record{}, false, err
	}
	if changed {
		if err := registry.saveLocked(data); err != nil {
			return Record{}, false, err
		}
	}
	for _, record := range data.Projects {
		if samePath(record.WorkspacePath, canonical) && (includeArchived || record.ArchivedAt == nil) {
			return projectStatus(record), true, nil
		}
	}
	return Record{}, false, nil
}

func (registry *Registry) Add(path string, kind Type, name string) (Record, error) {
	if !kind.Valid() {
		return Record{}, fmt.Errorf("invalid project type %q", kind)
	}
	canonical, err := canonicalDirectory(path, true)
	if err != nil {
		return Record{}, err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	data, _, err := registry.loadAndDiscoverLocked()
	if err != nil {
		return Record{}, err
	}
	now := time.Now().UTC()
	for index := range data.Projects {
		if !samePath(data.Projects[index].WorkspacePath, canonical) {
			continue
		}
		record := &data.Projects[index]
		if record.Type != kind {
			return Record{}, fmt.Errorf(
				"directory is already registered as project type %s",
				record.Type,
			)
		}
		if alias := strings.TrimSpace(name); alias != "" {
			record.Name = alias
		}
		record.ArchivedAt = nil
		record.UpdatedAt = now
		if err := registry.saveLocked(data); err != nil {
			return Record{}, err
		}
		return projectStatus(*record), nil
	}
	record, err := newRecord(canonical, kind, name, now)
	if err != nil {
		return Record{}, err
	}
	data.Projects = append(data.Projects, record)
	data.Order = append(data.Order, record.ID)
	if data.SortMode == "" {
		data.SortMode = SortManual
	}
	if err := registry.saveLocked(data); err != nil {
		return Record{}, err
	}
	return projectStatus(record), nil
}

func (registry *Registry) EnsureBook(path string) (Record, error) {
	canonical, err := canonicalDirectory(path, true)
	if err != nil {
		return Record{}, err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	data, changed, err := registry.loadAndDiscoverLocked()
	if err != nil {
		return Record{}, err
	}
	for _, record := range data.Projects {
		if samePath(record.WorkspacePath, canonical) {
			if record.ArchivedAt != nil {
				return Record{}, fmt.Errorf("project %s is archived", record.ID)
			}
			if record.Type != TypeBook {
				return Record{}, fmt.Errorf("project %s is not a book", record.ID)
			}
			if changed {
				if err := registry.saveLocked(data); err != nil {
					return Record{}, err
				}
			}
			return projectStatus(record), nil
		}
	}
	now := time.Now().UTC()
	record, err := newRecord(canonical, TypeBook, "", now)
	if err != nil {
		return Record{}, err
	}
	data.Projects = append(data.Projects, record)
	data.Order = append(data.Order, record.ID)
	if err := registry.saveLocked(data); err != nil {
		return Record{}, err
	}
	return projectStatus(record), nil
}

func (registry *Registry) TouchBook(path string) (Record, error) {
	record, err := registry.EnsureBook(path)
	if err != nil {
		return Record{}, err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	data, err := registry.loadLocked()
	if err != nil {
		return Record{}, err
	}
	now := time.Now().UTC()
	for index := range data.Projects {
		if data.Projects[index].ID != record.ID {
			continue
		}
		data.Projects[index].LastOpenedAt = now
		data.Projects[index].UpdatedAt = now
		data.CurrentBookID = record.ID
		record = data.Projects[index]
		break
	}
	if err := registry.saveLocked(data); err != nil {
		return Record{}, err
	}
	return projectStatus(record), nil
}

func (registry *Registry) CurrentBookPath() string {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	data, changed, err := registry.loadAndDiscoverLocked()
	if err != nil {
		return ""
	}
	if changed {
		_ = registry.saveLocked(data)
	}
	for _, record := range data.Projects {
		if record.ID == data.CurrentBookID && record.Type == TypeBook && record.ArchivedAt == nil && projectStatus(record).Status == StatusAvailable {
			return record.WorkspacePath
		}
	}
	return ""
}

func (registry *Registry) Rename(id, name string) (Record, error) {
	name = strings.TrimSpace(name)
	if err := ValidateID(id); err != nil || name == "" {
		return Record{}, fmt.Errorf("project ID and name are required")
	}
	return registry.updateRecord(id, func(record *Record, now time.Time) error {
		record.Name = name
		record.UpdatedAt = now
		return nil
	})
}

func (registry *Registry) Archive(id string) (Record, error) {
	return registry.updateRecord(id, func(record *Record, now time.Time) error {
		if record.ArchivedAt == nil {
			record.ArchivedAt = &now
			record.UpdatedAt = now
		}
		return nil
	})
}

func (registry *Registry) Relink(id, path string) (Record, error) {
	canonical, err := canonicalDirectory(path, true)
	if err != nil {
		return Record{}, err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	data, _, err := registry.loadAndDiscoverLocked()
	if err != nil {
		return Record{}, err
	}
	for _, record := range data.Projects {
		// Archived Projects still own their stable identity and user state. Allowing a
		// second Project to relink onto the same directory would make path-based legacy
		// resolution ambiguous and normalizeRegistryData would have to discard one record.
		if record.ID != id && samePath(record.WorkspacePath, canonical) {
			return Record{}, fmt.Errorf("directory is already registered by project %s", record.ID)
		}
	}
	now := time.Now().UTC()
	for index := range data.Projects {
		if data.Projects[index].ID != id {
			continue
		}
		data.Projects[index].WorkspacePath = canonical
		data.Projects[index].ArchivedAt = nil
		data.Projects[index].UpdatedAt = now
		if err := registry.saveLocked(data); err != nil {
			return Record{}, err
		}
		return projectStatus(data.Projects[index]), nil
	}
	return Record{}, fmt.Errorf("project %s not found", id)
}

func (registry *Registry) Reorder(ids []string) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	data, _, err := registry.loadAndDiscoverLocked()
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(data.Projects))
	for _, record := range data.Projects {
		if record.ArchivedAt == nil {
			known[record.ID] = true
		}
	}
	seen := make(map[string]bool, len(ids))
	order := make([]string, 0, len(known))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if known[id] && !seen[id] {
			seen[id] = true
			order = append(order, id)
		}
	}
	for _, record := range orderedRecords(data, false) {
		if !seen[record.ID] {
			order = append(order, record.ID)
		}
	}
	data.Order = order
	data.SortMode = SortManual
	return registry.saveLocked(data)
}

func (registry *Registry) SortMode() SortMode {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	data, changed, err := registry.loadAndDiscoverLocked()
	if err != nil {
		return SortRecent
	}
	if changed {
		if err := registry.saveLocked(data); err != nil {
			return SortRecent
		}
	}
	return normalizedSortMode(data.SortMode, len(data.Order) > 0)
}

func (registry *Registry) SetSortMode(mode SortMode) error {
	if mode != SortRecent && mode != SortManual {
		return fmt.Errorf("invalid project sort mode %q", mode)
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	data, _, err := registry.loadAndDiscoverLocked()
	if err != nil {
		return err
	}
	if mode == SortManual && len(data.Order) == 0 {
		for _, record := range orderedRecords(data, false) {
			data.Order = append(data.Order, record.ID)
		}
	}
	data.SortMode = mode
	return registry.saveLocked(data)
}

func (registry *Registry) updateRecord(id string, mutate func(*Record, time.Time) error) (Record, error) {
	if err := ValidateID(id); err != nil {
		return Record{}, err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	data, _, err := registry.loadAndDiscoverLocked()
	if err != nil {
		return Record{}, err
	}
	now := time.Now().UTC()
	for index := range data.Projects {
		if data.Projects[index].ID != id {
			continue
		}
		if err := mutate(&data.Projects[index], now); err != nil {
			return Record{}, err
		}
		if data.CurrentBookID == id && data.Projects[index].ArchivedAt != nil {
			data.CurrentBookID = ""
		}
		if err := registry.saveLocked(data); err != nil {
			return Record{}, err
		}
		return projectStatus(data.Projects[index]), nil
	}
	return Record{}, fmt.Errorf("project %s not found", id)
}

func (registry *Registry) loadAndDiscoverLocked() (registryData, bool, error) {
	data, migrated, err := registry.loadOrMigrateLocked()
	if err != nil {
		return registryData{}, false, err
	}
	discovered, err := registry.discoverBooksLocked(&data)
	return data, migrated || discovered, err
}

func (registry *Registry) loadLocked() (registryData, error) {
	data, _, err := registry.loadOrMigrateLocked()
	return data, err
}

func (registry *Registry) discoverBooksLocked(data *registryData) (bool, error) {
	if strings.TrimSpace(registry.denovaDir) == "" {
		return false, nil
	}
	root, err := filepath.Abs(registry.denovaDir)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	known := make(map[string]bool, len(data.Projects))
	for _, record := range data.Projects {
		known[filepath.Clean(record.WorkspacePath)] = true
	}
	candidates := []string{filepath.Join(root, ContentDirectoryName), root}
	changed := false
	for index, parent := range candidates {
		entries, readErr := os.ReadDir(parent)
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return false, readErr
		}
		for _, entry := range entries {
			if !entry.IsDir() || (index == 1 && isUserDataDirectory(entry.Name())) {
				continue
			}
			path, canonicalErr := canonicalDirectory(filepath.Join(parent, entry.Name()), true)
			if canonicalErr != nil || known[path] || !isBookWorkspace(path) {
				continue
			}
			record, recordErr := newRecord(path, TypeBook, entry.Name(), time.Now().UTC())
			if recordErr != nil {
				return false, recordErr
			}
			data.Projects = append(data.Projects, record)
			data.Order = append(data.Order, record.ID)
			known[path] = true
			changed = true
		}
	}
	return changed, nil
}

func (registry *Registry) saveLocked(data registryData) error {
	normalizeRegistryData(&data)
	if err := os.MkdirAll(filepath.Dir(registry.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(registry.path), ".projects-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	removeTemp := true
	defer func() {
		_ = temp.Close()
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(append(raw, '\n')); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, registry.path); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func normalizeRegistryData(data *registryData) {
	data.Version = registryVersion
	data.SortMode = normalizedSortMode(data.SortMode, len(data.Order) > 0)
	seenIDs := make(map[string]bool, len(data.Projects))
	seenPaths := make(map[string]bool, len(data.Projects))
	projects := make([]Record, 0, len(data.Projects))
	for _, record := range data.Projects {
		canonical, err := canonicalDirectory(record.WorkspacePath, false)
		if err != nil || canonical == "" || seenPaths[canonical] {
			continue
		}
		record.WorkspacePath = canonical
		if !record.Type.Valid() {
			record.Type = TypeBook
		}
		if ValidateID(record.ID) != nil || seenIDs[record.ID] {
			record.ID = uniqueStableID(canonical, seenIDs)
		}
		if record.Name = strings.TrimSpace(record.Name); record.Name == "" {
			record.Name = filepath.Base(canonical)
		}
		if record.CreatedAt.IsZero() {
			record.CreatedAt = time.Now().UTC()
		}
		if record.UpdatedAt.IsZero() {
			record.UpdatedAt = record.CreatedAt
		}
		record.Status = ""
		seenIDs[record.ID] = true
		seenPaths[canonical] = true
		projects = append(projects, record)
	}
	data.Projects = projects
}

func orderedRecords(data registryData, includeArchived bool) []Record {
	records := make([]Record, 0, len(data.Projects))
	for _, record := range data.Projects {
		if !includeArchived && record.ArchivedAt != nil {
			continue
		}
		records = append(records, projectStatus(record))
	}
	if normalizedSortMode(data.SortMode, len(data.Order) > 0) == SortManual {
		rank := make(map[string]int, len(data.Order))
		for index, id := range data.Order {
			if _, exists := rank[id]; !exists {
				rank[id] = index
			}
		}
		sort.SliceStable(records, func(i, j int) bool {
			left, leftOK := rank[records[i].ID]
			right, rightOK := rank[records[j].ID]
			if leftOK != rightOK {
				return leftOK
			}
			if leftOK && left != right {
				return left < right
			}
			return recentLess(records[i], records[j])
		})
		return records
	}
	sort.SliceStable(records, func(i, j int) bool { return recentLess(records[i], records[j]) })
	return records
}

func recentLess(left, right Record) bool {
	if !left.LastOpenedAt.Equal(right.LastOpenedAt) {
		return left.LastOpenedAt.After(right.LastOpenedAt)
	}
	if strings.ToLower(left.Name) != strings.ToLower(right.Name) {
		return strings.ToLower(left.Name) < strings.ToLower(right.Name)
	}
	return left.ID < right.ID
}

func newRecord(path string, kind Type, name string, now time.Time) (Record, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = filepath.Base(path)
	}
	id, err := randomID()
	if err != nil {
		return Record{}, fmt.Errorf("generate project ID: %w", err)
	}
	return Record{
		ID: id, Type: kind, Name: name, WorkspacePath: filepath.Clean(path),
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func randomID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "project-" + hex.EncodeToString(value[:]), nil
}

func stableID(path string) string {
	digest := sha256.Sum256([]byte(filepath.Clean(path)))
	return "project-" + hex.EncodeToString(digest[:16])
}

func uniqueStableID(path string, existing map[string]bool) string {
	for suffix := 0; ; suffix++ {
		candidatePath := path
		if suffix > 0 {
			candidatePath = fmt.Sprintf("%s\x00%d", path, suffix)
		}
		candidate := stableID(candidatePath)
		if !existing[candidate] {
			return candidate
		}
	}
}

func projectStatus(record Record) Record {
	if record.ArchivedAt != nil {
		record.Status = StatusArchived
		return record
	}
	info, err := os.Stat(record.WorkspacePath)
	if err == nil && info.IsDir() {
		record.Status = StatusAvailable
	} else {
		record.Status = StatusMissing
	}
	return record
}

func canonicalDirectory(path string, requireExists bool) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("project directory is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if info, statErr := os.Stat(abs); statErr == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("project path is not a directory: %s", abs)
		}
		if canonical, evalErr := filepath.EvalSymlinks(abs); evalErr == nil {
			abs = filepath.Clean(canonical)
		}
	} else if requireExists {
		return "", fmt.Errorf("project directory does not exist: %s", abs)
	}
	return abs, nil
}

func samePath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}

func normalizedSortMode(mode SortMode, hasOrder bool) SortMode {
	if mode == SortRecent || mode == SortManual {
		return mode
	}
	if hasOrder {
		return SortManual
	}
	return SortRecent
}

func isUserDataDirectory(name string) bool {
	switch name {
	case "book_meta", "styles", ContentDirectoryName, StateDirectoryName, "automations":
		return true
	default:
		return strings.HasPrefix(name, ".")
	}
}

func isBookWorkspace(path string) bool {
	markers := []string{
		filepath.Join(path, workspacelayout.DataDirName), filepath.Join(path, workspacelayout.LegacyDataDirName),
		filepath.Join(path, "book.json"), filepath.Join(path, "ideas.md"), filepath.Join(path, "brainstorm.md"),
		filepath.Join(path, "chapters"), filepath.Join(path, "setting"),
	}
	for _, marker := range markers {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	return false
}
