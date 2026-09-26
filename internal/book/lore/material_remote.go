package lore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"denova/internal/publicnet"
)

var (
	ErrMaterialURL         = errors.New("material URL must be an absolute HTTPS URL without credentials")
	ErrMaterialDownload    = errors.New("could not download remote material")
	ErrMaterialRemoteImage = errors.New("remote material must contain a valid PNG or JPEG image")
)

func parseMaterialURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 8192 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || strings.ContainsAny(raw, "\r\n\\") {
		return nil, ErrMaterialURL
	}
	return u, nil
}

// RemoteMaterial applies an explicit remote add, association replacement, or
// local copy. Online references never fetch bytes on the backend. Downloads
// use the public-network policy and the same atomic file commit as uploads.
func (s *Store) RemoteMaterial(ctx context.Context, id string, m MaterialMutation) (Item, error) {
	item, err := s.ReadAny(id)
	if err != nil {
		return Item{}, err
	}
	if m.Op == "localize" {
		m.URL = ""
		for _, material := range item.ResolvedMaterials {
			if material.ID == m.AssetID {
				m.URL = material.URL
				break
			}
		}
		if m.URL == "" {
			return Item{}, fmt.Errorf("remote material no longer linked: %s", m.AssetID)
		}
		m.SaveLocally = true
	} else if m.Op != "remote" {
		return Item{}, fmt.Errorf("unknown remote material operation: %s", m.Op)
	}
	raw := strings.TrimSpace(m.URL)
	u, err := parseMaterialURL(raw)
	if err != nil {
		return Item{}, err
	}
	filename := path.Base(u.Path)
	if filename == "." || filename == "/" || filename == "" {
		filename = u.Hostname()
	}
	entry := MaterialEntry{Name: strings.TrimSpace(m.Name), Description: strings.TrimSpace(m.Description)}
	source := AssetSource{Kind: "web", URL: raw}
	if m.SaveLocally {
		data, err := downloadMaterial(ctx, materialHTTPClient(), raw)
		if err != nil {
			return Item{}, err
		}
		return s.SaveMaterial(ctx, id, MaterialFile{Filename: filename, Data: data, Source: source, Entry: entry, ReplaceAssetID: m.AssetID})
	}
	if err := ctx.Err(); err != nil {
		return Item{}, err
	}
	// Exact MIME and byte size are unknown until explicitly downloaded.
	asset := Asset{URL: raw, OriginalName: filename, MIMEType: "image/*", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Source: source}
	updated, err := s.attachAsset(id, asset, entry, m.AssetID)
	if err == nil {
		slog.InfoContext(ctx, "[lore-material] linked remote image", "item_id", id, "host", u.Hostname())
	}
	return updated, err
}

func materialHTTPClient() *http.Client {
	client := publicnet.NewHTTPClient()
	client.Timeout = 45 * time.Second
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("material redirect limit exceeded")
		}
		_, err := parseMaterialURL(req.URL.String())
		return err
	}
	return client
}

func downloadMaterial(ctx context.Context, client *http.Client, raw string) ([]byte, error) {
	defer client.CloseIdleConnections()
	if _, err := parseMaterialURL(raw); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, ErrMaterialURL
	}
	req.Header.Set("Accept", "image/png, image/jpeg")
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMaterialDownload, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrMaterialDownload, response.StatusCode)
	}
	if response.ContentLength > MaxMaterialUploadBytes {
		return nil, ErrMaterialTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxMaterialUploadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMaterialDownload, err)
	}
	if len(data) > MaxMaterialUploadBytes {
		return nil, ErrMaterialTooLarge
	}
	mime, _, err := MaterialFormat(data)
	if err != nil || !strings.HasPrefix(mime, "image/") {
		return nil, ErrMaterialRemoteImage
	}
	return data, nil
}
