package platform

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// ExportInstalled exports exactly the selected installed release, including
// disabled releases. Source edits and local settings never enter the archive.
func (m *Manager) ExportInstalled(ref ReleaseRef, writer io.Writer) error {
	if strings.HasPrefix(ref.ReleaseID, "preview-") {
		return failure("INVALID_ARGUMENT", "Only installed releases can be exported")
	}
	release, installed, err := m.release(ref)
	if err != nil {
		return err
	}
	if installed.Removed {
		return failure("NOT_FOUND", "Package is not installed")
	}
	root, err := os.OpenRoot(m.releasePath(release.Ref))
	if err != nil {
		return err
	}
	defer root.Close()
	files, err := readPackageFiles(root.FS(), []string{"."})
	if err != nil {
		return err
	}
	checked, err := checkPackage(ref.Package.Kind, files)
	if err != nil {
		return err
	}
	if checked.Digest != release.Digest {
		return failure("DOCUMENT_CONFLICT", "Installed package bytes changed")
	}
	if err := writePackageArchive(checked, writer); err != nil {
		return err
	}
	slog.Info("platform_package_exported", "kind", ref.Package.Kind, "package", ref.Package.ID, "release", ref.ReleaseID)
	return nil
}
