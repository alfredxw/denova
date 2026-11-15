//go:build !windows && !linux && !darwin

package antigravity

import "errors"

func detectOSSandbox() error {
	return errors.New("Antigravity terminal sandbox is unsupported on this operating system")
}
