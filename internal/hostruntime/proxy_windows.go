package hostruntime

import (
	"context"
	"errors"

	"golang.org/x/sys/windows/registry"
)

func systemProxyEnvironment(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer key.Close()
	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if enabled == 0 {
		return nil, nil
	}
	server, _, err := key.GetStringValue("ProxyServer")
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	bypass, _, err := key.GetStringValue("ProxyOverride")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return nil, err
	}
	return windowsProxyEnvironment(enabled, server, bypass), nil
}
