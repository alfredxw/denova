package config

import (
	"errors"
	"net/url"
	"strings"
)

const DefaultMarketRegistryURL = "https://alfredxw.github.io/denova-index/index.json"

var ErrInvalidMarketRegistryURL = errors.New("market registry requires an absolute HTTPS URL without credentials or a fragment")

// MarketSettings selects the user-wide discovery index. Installed packages
// retain their own sources independently of this registry.
type MarketSettings struct {
	RegistryURL string `toml:"registry_url,omitempty" json:"registry_url,omitempty"`
}

func (settings MarketSettings) Validate() error {
	value := strings.TrimSpace(settings.RegistryURL)
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return ErrInvalidMarketRegistryURL
	}
	return nil
}
