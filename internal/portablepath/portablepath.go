// Package portablepath defines the filename subset Denova may create inside a
// movable data directory. The rules intentionally use the strictest common
// behavior of Windows, WSL, Linux, and macOS.
package portablepath

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	MaxComponentBytes    = 240
	MaxRelativePathBytes = 2048
)

var fold = cases.Fold()

// NormalizeComponent prepares a user-visible name for a newly created path.
// Existing durable paths are validated, never silently normalized.
func NormalizeComponent(value string) (string, error) {
	value = norm.NFC.String(strings.TrimSpace(value))
	if err := ValidateComponent(value); err != nil {
		return "", err
	}
	return value, nil
}

func Validate(relative string) error {
	if relative == "" || relative == "." || len(relative) > MaxRelativePathBytes ||
		strings.Contains(relative, `\`) || !utf8.ValidString(relative) || !fs.ValidPath(relative) {
		return fmt.Errorf("path is not a portable slash-relative path: %q", relative)
	}
	if norm.NFC.String(relative) != relative {
		return fmt.Errorf("path is not Unicode NFC normalized: %q", relative)
	}
	for _, component := range strings.Split(relative, "/") {
		if err := ValidateComponent(component); err != nil {
			return fmt.Errorf("path component %q: %w", component, err)
		}
	}
	return nil
}

func ValidateComponent(component string) error {
	if component == "" || component == "." || component == ".." || !utf8.ValidString(component) {
		return fmt.Errorf("filename is empty, invalid UTF-8, or reserved")
	}
	if len(component) > MaxComponentBytes {
		return fmt.Errorf("filename exceeds %d bytes", MaxComponentBytes)
	}
	if norm.NFC.String(component) != component {
		return fmt.Errorf("filename is not Unicode NFC normalized")
	}
	if strings.ContainsAny(component, `<>:"/\|?*`) {
		return fmt.Errorf("filename contains a cross-platform reserved character")
	}
	for _, char := range component {
		if char == 0 || unicode.IsControl(char) {
			return fmt.Errorf("filename contains a control character")
		}
	}
	if strings.HasSuffix(component, " ") || strings.HasSuffix(component, ".") {
		return fmt.Errorf("filename ends with a space or period")
	}
	base := component
	if dot := strings.IndexByte(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	if isWindowsDeviceName(base) {
		return fmt.Errorf("filename uses the reserved device name %q", base)
	}
	return nil
}

func FoldKey(component string) string {
	return fold.String(norm.NFC.String(component))
}

// CheckNoCollision rejects a new relative path when a case-sensitive host
// already contains a sibling that would alias it on another target platform.
func CheckNoCollision(root, relative string) error {
	if err := Validate(relative); err != nil {
		return err
	}
	current := filepath.Clean(root)
	for _, component := range strings.Split(relative, "/") {
		entries, err := os.ReadDir(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		key := FoldKey(component)
		matched := ""
		for _, entry := range entries {
			if FoldKey(entry.Name()) != key {
				continue
			}
			if matched != "" && matched != entry.Name() {
				return fmt.Errorf("portable filename collision between %q and %q", matched, entry.Name())
			}
			matched = entry.Name()
		}
		if matched == "" {
			return nil
		}
		if matched != component {
			return fmt.Errorf("portable filename collision between %q and %q", matched, component)
		}
		current = filepath.Join(current, component)
	}
	return nil
}

// PreflightTree validates an existing movable tree without changing it.
func PreflightTree(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("portable tree root must be a regular directory: %s", root)
	}
	return preflightDirectory(root, "")
}

func preflightDirectory(current, relative string) error {
	entries, err := os.ReadDir(current)
	if err != nil {
		return err
	}
	seen := make(map[string]string, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if err := ValidateComponent(name); err != nil {
			return fmt.Errorf("portable preflight %s: %w", filepath.Join(current, name), err)
		}
		key := FoldKey(name)
		if previous := seen[key]; previous != "" && previous != name {
			return fmt.Errorf("portable preflight filename collision in %s: %q and %q", current, previous, name)
		}
		seen[key] = name
		relativePath := name
		if relative != "" {
			relativePath = path.Join(relative, name)
		}
		if err := Validate(relativePath); err != nil {
			return fmt.Errorf("portable preflight %s: %w", filepath.Join(current, name), err)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("portable preflight rejects symbolic links: %s", filepath.Join(current, name))
		}
		if entry.IsDir() {
			if err := preflightDirectory(filepath.Join(current, name), relativePath); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("portable preflight rejects non-regular entries: %s", filepath.Join(current, name))
		}
	}
	return nil
}

func isWindowsDeviceName(base string) bool {
	switch strings.ToUpper(base) {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"COM¹", "COM²", "COM³", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
		"LPT¹", "LPT²", "LPT³":
		return true
	default:
		return false
	}
}
