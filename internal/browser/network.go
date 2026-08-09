package browser

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"denova/internal/publicnet"
)

const maxBrowserURLRunes = 4096

// ValidatePublicURL rejects credentials, non-HTTP schemes, and destinations
// that currently resolve outside the public Internet. RodDriver repeats this
// policy at dial time for every top-level and subresource request.
func ValidatePublicURL(ctx context.Context, raw string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("browser URL is required")
	}
	if utf8.RuneCountInString(raw) > maxBrowserURLRunes {
		return "", fmt.Errorf("browser URL exceeds %d characters", maxBrowserURLRunes)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse browser URL: %w", err)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("browser URL must use http or https")
	}
	if parsed.Hostname() == "" {
		return "", errors.New("browser URL must include a host")
	}
	if parsed.User != nil {
		return "", errors.New("browser URL must not include user credentials")
	}
	if err := publicnet.ValidateHost(ctx, parsed.Hostname()); err != nil {
		return "", fmt.Errorf("validate browser destination: %w", err)
	}
	return parsed.String(), nil
}

func newPublicHTTPClient() *http.Client {
	return publicnet.NewHTTPClient()
}
