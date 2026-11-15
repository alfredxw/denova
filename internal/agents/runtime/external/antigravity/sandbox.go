package antigravity

import "fmt"

var verifyOSSandbox = detectOSSandbox

const (
	minWindows24H2Build     uint64 = 26100
	minWindows24H2AugustUBR uint64 = 9168
	minWindows26H2Build     uint64 = 26300
)

// validateWindowsSandboxBuild checks the documented Windows 11 24H2+ (August 2026
// update, KB5121003 build 26100.9168 / 26200.9168) AppContainer sandbox baseline.
func validateWindowsSandboxBuild(build, ubr uint64) error {
	if build >= minWindows26H2Build {
		return nil
	}
	if build >= minWindows24H2Build && ubr >= minWindows24H2AugustUBR {
		return nil
	}
	return fmt.Errorf("Windows build %d.%d does not support Antigravity AppContainer terminal sandbox (requires 26100.9168 or later)", build, ubr)
}
