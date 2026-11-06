//go:build desktop_e2e

package desktop

import (
	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/plugins/automation"
)

// CI and local automation can run behind another window. Keep frame callbacks
// active for those tests; production retains WebView's power-saving behavior.
const backgroundThrottling = mygo.ThrottlingOff
