package resourceexchange

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"

	"denova/internal/portablepath"
)

type portableImage struct {
	AssetPath string `json:"asset_path"`
	AltText   string `json:"alt_text,omitempty"`
}

func resourceAsset(dir string, resource PreviewResource, name string) ([]byte, error) {
	if err := portablepath.Validate(name); err != nil {
		return nil, err
	}
	full := path.Join(resource.Root, name)
	if !slices.Contains(resource.Assets, full) {
		return nil, fmt.Errorf("image reference is not a declared asset")
	}
	return os.ReadFile(filepath.Join(dir, "files", filepath.FromSlash(full)))
}
