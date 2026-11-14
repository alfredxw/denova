package antigravity

import (
	"fmt"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

func detectOSSandbox() error {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("read Windows version for terminal sandbox verification: %w", err)
	}
	defer key.Close()
	buildText, _, err := key.GetStringValue("CurrentBuildNumber")
	if err != nil {
		return fmt.Errorf("read Windows build number for terminal sandbox verification: %w", err)
	}
	build, err := strconv.ParseUint(buildText, 10, 64)
	if err != nil {
		return fmt.Errorf("parse Windows build number %q: %w", buildText, err)
	}
	if build >= minWindows26H2Build {
		return nil
	}
	ubr, _, err := key.GetIntegerValue("UBR")
	if err != nil {
		return fmt.Errorf("read Windows UBR for terminal sandbox verification: %w", err)
	}
	return validateWindowsSandboxBuild(build, ubr)
}
