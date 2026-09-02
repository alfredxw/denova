package project

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"denova/internal/portablepath"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	agentsStoreDirName         = "Agents"
	defaultStoreDirName        = "Project"
	maxStoreDirectoryNameBytes = 128
)

func (registry *Registry) assignReadableStoreDirNames(data *registryData) error {
	for index := range data.Projects {
		data.Projects[index].StoreDirName = ""
	}
	used, err := registry.usedStoreDirNames(nil)
	if err != nil {
		return err
	}
	for index := range data.Projects {
		record := &data.Projects[index]
		if record.Type == TypeAgents || record.ID == AgentsProjectID {
			record.StoreDirName = agentsStoreDirName
			continue
		}
		record.StoreDirName = availableStoreDirName(record.Name, used)
		used[storeDirNameKey(record.StoreDirName)] = true
	}
	return validateStoreDirNames(data.Projects)
}

func (registry *Registry) nextStoreDirName(name string, projects []Record) (string, error) {
	used, err := registry.usedStoreDirNames(projects)
	if err != nil {
		return "", err
	}
	return availableStoreDirName(name, used), nil
}

func (registry *Registry) usedStoreDirNames(projects []Record) (map[string]bool, error) {
	used := map[string]bool{storeDirNameKey(agentsStoreDirName): true}
	for _, record := range projects {
		if record.StoreDirName != "" {
			used[storeDirNameKey(record.StoreDirName)] = true
		}
	}
	if registry != nil && strings.TrimSpace(registry.denovaDir) != "" {
		entries, err := os.ReadDir(filepath.Join(registry.denovaDir, storeDirectoryName))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		for _, entry := range entries {
			used[storeDirNameKey(entry.Name())] = true
		}
	}
	return used, nil
}

func availableStoreDirName(name string, used map[string]bool) string {
	base := storeDirNameBase(name)
	if !used[storeDirNameKey(base)] {
		return base
	}
	for suffix := 2; ; suffix++ {
		tail := fmt.Sprintf("-%d", suffix)
		prefix := truncateStoreDirName(base, maxStoreDirectoryNameBytes-len(tail))
		candidate := strings.TrimRight(prefix, "-") + tail
		if !used[storeDirNameKey(candidate)] {
			return candidate
		}
	}
}

func storeDirNameBase(name string) string {
	name = norm.NFC.String(strings.TrimSpace(name))
	var builder strings.Builder
	pendingDash := false
	for _, char := range name {
		if unicode.IsLetter(char) || unicode.IsNumber(char) {
			if pendingDash && builder.Len() > 0 {
				builder.WriteByte('-')
			}
			builder.WriteRune(char)
			pendingDash = false
			continue
		}
		if builder.Len() > 0 {
			pendingDash = true
		}
	}
	result := truncateStoreDirName(strings.Trim(builder.String(), "-"), maxStoreDirectoryNameBytes)
	if result == "" {
		result = defaultStoreDirName
	}
	if portablepath.ValidateComponent(result) != nil {
		result = defaultStoreDirName + "-" + result
	}
	return result
}

func truncateStoreDirName(name string, maxBytes int) string {
	if len(name) <= maxBytes {
		return name
	}
	for len(name) > maxBytes {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	return strings.TrimRight(name, "-")
}

func storeDirNameKey(name string) string {
	return norm.NFC.String(cases.Fold().String(name))
}

func validateStoreDirName(name string) error {
	if name == "" || len(name) > maxStoreDirectoryNameBytes || storeDirNameBase(name) != name {
		return fmt.Errorf("invalid Project Store directory %q", name)
	}
	return nil
}

func validateStoreDirNames(projects []Record) error {
	seen := make(map[string]string, len(projects))
	for _, record := range projects {
		if err := validateStoreDirName(record.StoreDirName); err != nil {
			return fmt.Errorf("project %s: %w", record.ID, err)
		}
		if record.Type == TypeAgents && record.StoreDirName != agentsStoreDirName {
			return fmt.Errorf("Agents project must use Store directory %q", agentsStoreDirName)
		}
		key := storeDirNameKey(record.StoreDirName)
		if existing := seen[key]; existing != "" {
			return fmt.Errorf(
				"projects %s and %s share Store directory %q",
				existing,
				record.ID,
				record.StoreDirName,
			)
		}
		seen[key] = record.ID
	}
	return nil
}

// migrateLegacyProjectStoreRoot atomically adopts the unreleased project-state
// root. A conflicting stores root is never merged or overwritten.
func (registry *Registry) migrateLegacyProjectStoreRoot() error {
	source := filepath.Join(registry.denovaDir, legacyProjectStateDirectoryName)
	destination := filepath.Join(registry.denovaDir, storeDirectoryName)
	sourceInfo, sourceErr := os.Lstat(source)
	destinationInfo, destinationErr := os.Lstat(destination)
	if sourceErr != nil && !errors.Is(sourceErr, os.ErrNotExist) {
		return fmt.Errorf("inspect legacy Project Store root %s: %w", source, sourceErr)
	}
	if destinationErr != nil && !errors.Is(destinationErr, os.ErrNotExist) {
		return fmt.Errorf("inspect Project Store root %s: %w", destination, destinationErr)
	}
	if sourceErr != nil {
		if destinationErr == nil && (destinationInfo.Mode()&os.ModeSymlink != 0 || !destinationInfo.IsDir()) {
			return fmt.Errorf("Project Store root is not a directory: %s", destination)
		}
		return nil
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.IsDir() {
		return fmt.Errorf("legacy Project Store root is not a directory: %s", source)
	}
	if destinationErr == nil {
		return fmt.Errorf("Project Store migration destination already exists: %s", destination)
	}
	if err := os.Rename(source, destination); err != nil {
		return fmt.Errorf("move Project Store root to %s: %w", destination, err)
	}
	slog.Info(
		"[internal/project/store_directory.go] migrated Project Store root",
		"source", source,
		"destination", destination,
	)
	return nil
}

// migrateProjectStoresOnce completes the coupled directory and receipt
// migrations before the Registry exposes a runtime Project layout.
func (registry *Registry) migrateProjectStoresOnce(projects []Record) error {
	if registry.storeMigrationComplete {
		return nil
	}
	if err := registry.migrateProjectIDStoreDirectories(projects); err != nil {
		return err
	}
	if err := registry.migrateReleasedStoreReceipts(projects); err != nil {
		return err
	}
	registry.storeMigrationComplete = true
	return nil
}

// migrateProjectIDStoreDirectories moves the unreleased ID-named layout to
// the persisted readable directory. It never merges or overwrites Store data.
func (registry *Registry) migrateProjectIDStoreDirectories(projects []Record) error {
	storesRoot := filepath.Join(registry.denovaDir, storeDirectoryName)
	for _, record := range projects {
		source := filepath.Join(storesRoot, record.ID)
		destination := filepath.Join(storesRoot, record.StoreDirName)
		if filepath.Clean(source) == filepath.Clean(destination) {
			continue
		}

		sourceInfo, sourceErr := os.Lstat(source)
		destinationInfo, destinationErr := os.Lstat(destination)
		if sourceErr != nil && !errors.Is(sourceErr, os.ErrNotExist) {
			return fmt.Errorf("inspect legacy Project Store %s: %w", source, sourceErr)
		}
		if destinationErr != nil && !errors.Is(destinationErr, os.ErrNotExist) {
			return fmt.Errorf("inspect readable Project Store %s: %w", destination, destinationErr)
		}
		if sourceErr != nil {
			if destinationErr == nil && (destinationInfo.Mode()&os.ModeSymlink != 0 || !destinationInfo.IsDir()) {
				return fmt.Errorf("Project Store destination is not a directory: %s", destination)
			}
			continue
		}
		if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.IsDir() {
			return fmt.Errorf("legacy Project Store is not a directory: %s", source)
		}
		if destinationErr == nil {
			if os.SameFile(sourceInfo, destinationInfo) {
				continue
			}
			return fmt.Errorf("Project Store migration destination already exists: %s", destination)
		}
		if err := os.Rename(source, destination); err != nil {
			return fmt.Errorf("move Project Store to %s: %w", destination, err)
		}
		slog.Info(
			"[internal/project/store_directory.go] migrated Project Store directory",
			"project_id", record.ID,
			"source", source,
			"destination", destination,
		)
	}
	return nil
}
