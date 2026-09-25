// Package resourceexchange coordinates portable resource delivery. Domain
// libraries and the extension platform remain the owners of installed content.
package resourceexchange

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"denova/internal/portablepath"
	"denova/internal/revisionfile"
)

const CatalogURL = "https://alfredxw.github.io/denova-index/index.json"
const SubmissionURL = "https://github.com/alfredxw/denova-index/issues/new?template=package.yml"
const maxCatalogBytes = 4 << 20

type Source struct {
	Kind     string `json:"kind"`
	URL      string `json:"url,omitempty"`
	Ref      string `json:"ref,omitempty"`
	Path     string `json:"path,omitempty"`
	Commit   string `json:"commit,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type MarketEntry struct {
	ID            string            `json:"id"`
	Name          map[string]string `json:"name"`
	Description   map[string]string `json:"description"`
	Author        string            `json:"author"`
	Format        string            `json:"format"`
	Kinds         []string          `json:"kinds"`
	Tags          []string          `json:"tags"`
	Source        Source            `json:"source"`
	UpdatedAt     string            `json:"updated_at"`
	Featured      bool              `json:"featured,omitempty"`
	Cover         string            `json:"cover,omitempty"`
	Compatibility map[string]string `json:"compatibility,omitempty"`
	Usage         map[string]string `json:"usage,omitempty"`
}

type Catalog struct {
	SchemaVersion int           `json:"schema_version"`
	Entries       []MarketEntry `json:"entries"`
}

type MarketSnapshot struct {
	Catalog
	FetchedAt time.Time `json:"fetched_at"`
	Stale     bool      `json:"stale"`
	ErrorKey  string    `json:"error_key,omitempty"`
}

// Market fetches only after an explicit discovery-page request. Its cache is
// disposable: neither installation identity nor update authority comes from it.
type Market struct {
	path   string
	mu     sync.Mutex
	client *http.Client
}

func NewMarket(root string) *Market {
	return &Market{path: filepath.Join(root, "resource-exchange", "cache", "market.json"), client: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || req.URL.Scheme != "https" || req.URL.Host != "alfredxw.github.io" {
			return fmt.Errorf("unexpected catalog redirect")
		}
		return nil
	}}}
}

func (m *Market) Catalog(ctx context.Context, refresh bool) (MarketSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var cached MarketSnapshot
	if snapshot, err := revisionfile.Read(ctx, m.path); err == nil && snapshot.Exists && len(snapshot.Content) <= maxCatalogBytes+4096 {
		if json.Unmarshal(snapshot.Content, &cached) != nil || validateCatalog(cached.Catalog) != nil {
			cached = MarketSnapshot{}
		}
	}
	if !refresh && !cached.FetchedAt.IsZero() && time.Since(cached.FetchedAt) < time.Hour {
		return cached, nil
	}
	catalog, err := m.fetch(ctx)
	if err != nil {
		slog.WarnContext(ctx, "resource_market_refresh_failed", "reason", err)
		if cached.FetchedAt.IsZero() {
			return MarketSnapshot{}, err
		}
		cached.Stale, cached.ErrorKey = true, "market.errors.catalogUnavailable"
		return cached, nil
	}
	result := MarketSnapshot{Catalog: catalog, FetchedAt: time.Now().UTC()}
	raw, err := json.Marshal(result)
	if err != nil {
		return MarketSnapshot{}, err
	}
	if _, err := revisionfile.ReplaceIfRevision(ctx, m.path, "", raw, revisionfile.Options{}); err != nil {
		return MarketSnapshot{}, err
	}
	slog.InfoContext(ctx, "resource_market_refreshed", "entries", len(catalog.Entries))
	return result, nil
}

func (m *Market) fetch(ctx context.Context) (Catalog, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, CatalogURL, nil)
	if err != nil {
		return Catalog{}, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return Catalog{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Catalog{}, fmt.Errorf("catalog HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxCatalogBytes+1))
	if err != nil {
		return Catalog{}, err
	}
	if len(raw) > maxCatalogBytes {
		return Catalog{}, fmt.Errorf("catalog exceeds byte limit")
	}
	var catalog Catalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return Catalog{}, err
	}
	return catalog, validateCatalog(catalog)
}

var resourceID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
var repositoryPath = regexp.MustCompile(`^/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func validateSource(source Source) error {
	u, err := url.Parse(source.URL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("invalid HTTPS source")
	}
	switch source.Kind {
	case "github":
		if u.Host != "github.com" || u.RawQuery != "" || !repositoryPath.MatchString(u.Path) || len(source.Ref) > 200 {
			return fmt.Errorf("invalid GitHub source")
		}
		if source.Path != "" {
			return portablepath.Validate(source.Path)
		}
	case "https_zip":
		if source.Path != "" {
			if err := portablepath.Validate(source.Path); err != nil {
				return err
			}
		}
		if source.Ref != "" {
			return fmt.Errorf("ZIP source cannot contain Git ref")
		}
	default:
		return fmt.Errorf("unknown remote source kind %q", source.Kind)
	}
	return nil
}

func validKind(kind string) bool {
	switch kind {
	case "preset.narrative", "preset.image", "preset.game_planning", "preset.events", "preset.rules", "preset.actor_state", "style.reference", "skill", "lore.item", "game.opening", "project.cover", "extension.plugin", "extension.game":
		return true
	}
	return false
}

func validateCatalog(catalog Catalog) error {
	if catalog.SchemaVersion != 1 || catalog.Entries == nil || len(catalog.Entries) > 5000 {
		return fmt.Errorf("invalid catalog version or entry count")
	}
	seen := map[string]bool{}
	for _, entry := range catalog.Entries {
		if !resourceID.MatchString(entry.ID) || seen[entry.ID] || len(entry.Name) == 0 || len(entry.Description) == 0 || len(entry.Kinds) == 0 {
			return fmt.Errorf("invalid catalog entry %q", entry.ID)
		}
		seen[entry.ID] = true
		for _, value := range []map[string]string{entry.Name, entry.Description, entry.Compatibility, entry.Usage} {
			if len(value) > 8 {
				return fmt.Errorf("too many translations")
			}
			for _, text := range value {
				if strings.TrimSpace(text) == "" || len(text) > 8000 {
					return fmt.Errorf("invalid catalog text")
				}
			}
		}
		for _, kind := range entry.Kinds {
			if !validKind(kind) {
				return fmt.Errorf("unknown catalog resource kind %q", kind)
			}
		}
		switch entry.Format {
		case "skill", "extension.plugin", "extension.game", "denova.resource-pack", "character_card":
		default:
			return fmt.Errorf("unknown package format")
		}
		if _, err := time.Parse("2006-01-02", entry.UpdatedAt); err != nil {
			return err
		}
		if err := validateSource(entry.Source); err != nil {
			return err
		}
		if entry.Source.Commit != "" || entry.Source.Filename != "" {
			return fmt.Errorf("catalog cannot inject installation provenance")
		}
		if entry.Cover != "" {
			u, err := url.Parse(entry.Cover)
			if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
				return fmt.Errorf("invalid cover URL")
			}
			switch strings.ToLower(filepath.Ext(u.Path)) {
			case ".png", ".jpg", ".jpeg", ".webp":
			default:
				return fmt.Errorf("invalid cover type")
			}
		}
	}
	return nil
}
