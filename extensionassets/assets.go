// Package extensionassets owns the distributed extension scaffolds and SDK files.
// It contains no host runtime or extension installation policy.
package extensionassets

import "embed"

//go:embed starters sdk
var files embed.FS

// Files returns read-only distribution assets rooted at starters/ and sdk/.
// Callers select the project scaffold and copy SDK files to the extension root;
// these paths describe source assets, not paths persisted in user projects.
func Files() embed.FS { return files }
