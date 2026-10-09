package resourceexchange

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMarketUsesConfiguredProxy(t *testing.T) {
	var connects atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "alfredxw.github.io:443" {
			t.Errorf("unexpected proxy request: %s %s", r.Method, r.Host)
		}
		connects.Add(1)
		http.Error(w, "fixture proxy unavailable", http.StatusBadGateway)
	}))
	t.Cleanup(proxy.Close)
	for _, key := range []string{"HTTPS_PROXY", "https_proxy"} {
		t.Setenv(key, proxy.URL)
	}
	for _, key := range []string{"NO_PROXY", "no_proxy"} {
		t.Setenv(key, "")
	}
	market := NewMarket(t.TempDir())
	market.client.Timeout = time.Second
	t.Cleanup(market.client.CloseIdleConnections)
	_, err := market.Catalog(context.Background(), CatalogURL, true)
	if err == nil || connects.Load() != 1 {
		t.Fatalf("market bypassed the proxy: connects=%d error=%v", connects.Load(), err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testCatalog() Catalog {
	return Catalog{SchemaVersion: 1, Entries: []MarketEntry{{ID: "writing", Name: map[string]string{"en-US": "Writing"}, Description: map[string]string{"en-US": "A writing skill"}, Author: "Author", Format: "skill", Kinds: []string{"skill"}, Tags: []string{"writing"}, UpdatedAt: "2026-09-24", Source: Source{Kind: "github", URL: "https://github.com/author/skills", Ref: "main"}}}}
}

func TestMarketExplicitFetchCacheAndOfflineRecovery(t *testing.T) {
	ctx := context.Background()
	m := NewMarket(t.TempDir())
	calls := 0
	fail := false
	m.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.String() != CatalogURL {
			t.Fatalf("unexpected URL %s", req.URL)
		}
		if fail {
			return nil, errors.New("offline")
		}
		raw, _ := json.Marshal(testCatalog())
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})
	if calls != 0 {
		t.Fatal("construction fetched catalog")
	}
	first, err := m.Catalog(ctx, CatalogURL, false)
	if err != nil || len(first.Entries) != 1 || calls != 1 {
		t.Fatalf("first fetch: %+v %v calls=%d", first, err, calls)
	}
	if _, err := m.Catalog(ctx, CatalogURL, false); err != nil || calls != 1 {
		t.Fatal("fresh cache fetched again")
	}
	fail = true
	stale, err := m.Catalog(ctx, CatalogURL, true)
	if err != nil || !stale.Stale || len(stale.Entries) != 1 || stale.FetchedAt != first.FetchedAt {
		t.Fatalf("cache was lost: %+v %v", stale, err)
	}
	if err := os.Remove(m.path); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Catalog(ctx, CatalogURL, false); err == nil {
		t.Fatal("first offline request appeared empty")
	}
}

func TestMarketRejectsInvalidRefreshWithoutReplacingCache(t *testing.T) {
	for _, invalid := range []string{`{"schema_version":2,"entries":[]}`, `{"schema_version":1,"entries":null}`, strings.Repeat("x", maxCatalogBytes+1)} {
		t.Run(invalid[:20], func(t *testing.T) {
			m := NewMarket(t.TempDir())
			raw, _ := json.Marshal(testCatalog())
			body := string(raw)
			m.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			if _, err := m.Catalog(context.Background(), CatalogURL, false); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(m.path)
			body = invalid
			result, err := m.Catalog(context.Background(), CatalogURL, true)
			after, _ := os.ReadFile(m.path)
			if err != nil || !result.Stale || string(before) != string(after) {
				t.Fatalf("bad refresh replaced cache: %+v %v", result, err)
			}
		})
	}
}

func TestCatalogIdentityAndSourceValidation(t *testing.T) {
	catalog := testCatalog()
	catalog.Entries = append(catalog.Entries, catalog.Entries[0])
	if validateCatalog(catalog) == nil {
		t.Fatal("duplicate entry accepted")
	}
	for _, source := range []Source{{Kind: "github", URL: "https://user:pass@github.com/a/b"}, {Kind: "github", URL: "https://github.com/a/b", Path: "../outside"}, {Kind: "https_zip", URL: "http://example.com/pack.zip"}, {Kind: "command", URL: "https://example.com"}} {
		if validateSource(source) == nil {
			t.Fatalf("invalid source accepted: %+v", source)
		}
	}
}
