//go:build !windows

package platform

import "os"

func publishReleaseDirectory(staging, destination string) error {
	return os.Rename(staging, destination)
}
