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

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	harnessStateDirName        = "harness"
	defaultStateDirName        = "Project"
	maxStateDirectoryNameBytes = 128
)

func (registry *Registry) assignReadableStateDirNames(data *registryData) error {
	for index := range data.Projects {
		data.Projects[index].StateDirName = ""
	}
	used, err := registry.usedStateDirNames(nil)
	if err != nil {
		return err
	}
	for index := range data.Projects {
		record := &data.Projects[index]
		if record.Type == TypeHarness || record.ID == HarnessProjectID {
			record.StateDirName = harnessStateDirName
			continue
		}
		record.StateDirName = availableStateDirName(record.Name, used)
		used[stateDirNameKey(record.StateDirName)] = true
	}
	return validateStateDirNames(data.Projects)
}

func (registry *Registry) nextStateDirName(name string, projects []Record) (string, error) {
	used, err := registry.usedStateDirNames(projects)
	if err != nil {
		return "", err
	}
	return availableStateDirName(name, used), nil
}

func (registry *Registry) usedStateDirNames(projects []Record) (map[string]bool, error) {
	used := map[string]bool{stateDirNameKey(harnessStateDirName): true}
	for _, record := range projects {
		if record.StateDirName != "" {
			used[stateDirNameKey(record.StateDirName)] = true
		}
	}
	if registry != nil && strings.TrimSpace(registry.denovaDir) != "" {
		entries, err := os.ReadDir(filepath.Join(registry.denovaDir, StateDirectoryName))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		for _, entry := range entries {
			used[stateDirNameKey(entry.Name())] = true
		}
	}
	return used, nil
}

func availableStateDirName(name string, used map[string]bool) string {
	base := stateDirNameBase(name)
	if !used[stateDirNameKey(base)] {
		return base
	}
	for suffix := 2; ; suffix++ {
		tail := fmt.Sprintf("-%d", suffix)
		prefix := truncateStateDirName(base, maxStateDirectoryNameBytes-len(tail))
		candidate := strings.TrimRight(prefix, "-") + tail
		if !used[stateDirNameKey(candidate)] {
			return candidate
		}
	}
}

func stateDirNameBase(name string) string {
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
	result := truncateStateDirName(strings.Trim(builder.String(), "-"), maxStateDirectoryNameBytes)
	if result == "" {
		result = defaultStateDirName
	}
	if isWindowsReservedStateDirName(result) {
		result = defaultStateDirName + "-" + result
	}
	return result
}

func truncateStateDirName(name string, maxBytes int) string {
	if len(name) <= maxBytes {
		return name
	}
	for len(name) > maxBytes {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	return strings.TrimRight(name, "-")
}

func isWindowsReservedStateDirName(name string) bool {
	switch strings.ToUpper(name) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}

func stateDirNameKey(name string) string {
	return norm.NFC.String(cases.Fold().String(name))
}

func validateStateDirName(name string) error {
	if name == "" || len(name) > maxStateDirectoryNameBytes || stateDirNameBase(name) != name {
		return fmt.Errorf("invalid project state directory %q", name)
	}
	return nil
}

func validateStateDirNames(projects []Record) error {
	seen := make(map[string]string, len(projects))
	for _, record := range projects {
		if err := validateStateDirName(record.StateDirName); err != nil {
			return fmt.Errorf("project %s: %w", record.ID, err)
		}
		if record.Type == TypeHarness && record.StateDirName != harnessStateDirName {
			return fmt.Errorf("Harness project must use state directory %q", harnessStateDirName)
		}
		key := stateDirNameKey(record.StateDirName)
		if existing := seen[key]; existing != "" {
			return fmt.Errorf(
				"projects %s and %s share state directory %q",
				existing,
				record.ID,
				record.StateDirName,
			)
		}
		seen[key] = record.ID
	}
	return nil
}

func (registry *Registry) migrateProjectIDStateDirectoriesOnce(projects []Record) error {
	if registry.stateDirectoryMigrationComplete {
		return nil
	}
	if err := registry.migrateProjectIDStateDirectories(projects); err != nil {
		return err
	}
	registry.stateDirectoryMigrationComplete = true
	return nil
}

// migrateProjectIDStateDirectories moves the unreleased ID-named layout to
// the persisted readable directory. It never merges or overwrites state.
func (registry *Registry) migrateProjectIDStateDirectories(projects []Record) error {
	stateRoot := filepath.Join(registry.denovaDir, StateDirectoryName)
	for _, record := range projects {
		source := filepath.Join(stateRoot, record.ID)
		destination := filepath.Join(stateRoot, record.StateDirName)
		if filepath.Clean(source) == filepath.Clean(destination) {
			continue
		}

		sourceInfo, sourceErr := os.Lstat(source)
		destinationInfo, destinationErr := os.Lstat(destination)
		if sourceErr != nil && !errors.Is(sourceErr, os.ErrNotExist) {
			return fmt.Errorf("inspect legacy project state %s: %w", source, sourceErr)
		}
		if destinationErr != nil && !errors.Is(destinationErr, os.ErrNotExist) {
			return fmt.Errorf("inspect readable project state %s: %w", destination, destinationErr)
		}
		if sourceErr != nil {
			if destinationErr == nil && (destinationInfo.Mode()&os.ModeSymlink != 0 || !destinationInfo.IsDir()) {
				return fmt.Errorf("project state destination is not a directory: %s", destination)
			}
			continue
		}
		if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.IsDir() {
			return fmt.Errorf("legacy project state is not a directory: %s", source)
		}
		if destinationErr == nil {
			if os.SameFile(sourceInfo, destinationInfo) {
				continue
			}
			return fmt.Errorf("project state migration destination already exists: %s", destination)
		}
		if err := os.Rename(source, destination); err != nil {
			return fmt.Errorf("move project state to %s: %w", destination, err)
		}
		slog.Info(
			"[internal/project/state_directory.go] migrated Project state directory",
			"project_id", record.ID,
			"source", source,
			"destination", destination,
		)
	}
	return nil
}
