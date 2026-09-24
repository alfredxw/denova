package platform

import (
	"io/fs"

	"denova/extensionassets"
)

// PreviewBundled freezes a trusted, embedded extension using normal package
// validation and size limits. The source is rooted at its manifest and contains
// only release files. The shared browser client is supplied when absent.
// Installation policy, package identity and grants belong to the caller.
func (m *Manager) PreviewBundled(kind Kind, source fs.FS) (Candidate, error) {
	if !kind.valid() {
		return Candidate{}, failure("INVALID_ARGUMENT", "Invalid package kind")
	}
	files, err := readPackageFiles(source, []string{"."})
	if err != nil {
		return Candidate{}, err
	}
	if _, exists := files["client.mjs"]; !exists {
		files["client.mjs"], err = extensionassets.Files().ReadFile("sdk/client.mjs")
		if err != nil {
			return Candidate{}, err
		}
	}
	total := 0
	for _, data := range files {
		total += len(data)
	}
	if len(files) > MaxPackageFiles || total > MaxPackageBytes {
		return Candidate{}, failure("LIMIT_EXCEEDED", "Bundled package exceeds file or byte limit")
	}
	return m.freeze(kind, files)
}
