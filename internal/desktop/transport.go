package desktop

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
)

//go:embed preload.js
var preload string

func desktopOrigin() string {
	if runtime.GOOS == "windows" {
		return "http://mygo.localhost"
	}
	return "mygo://localhost"
}

func ownPage(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.User == nil && (u.Scheme+"://"+u.Host == desktopOrigin() || u.Scheme+"://"+u.Host == "mygo://localhost")
}

func (s *shell) serve(w http.ResponseWriter, r *http.Request) {
	page := mygo.CallerPage(r.Context())
	if page == nil || page.Window() != s.window.Load() || !ownPage(r.URL.String()) {
		http.Error(w, "Untrusted desktop request", http.StatusForbidden)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != desktopOrigin() {
		http.Error(w, "Untrusted origin", http.StatusForbidden)
		return
	}
	b := s.backend.Load()
	if b == nil || b.stopping.Load() {
		http.Error(w, "Backend unavailable", http.StatusServiceUnavailable)
		return
	}
	upstream := b.url
	if renderer := os.Getenv("DENOVA_DESKTOP_RENDERER_URL"); renderer != "" && r.URL.Path != "/api" && !strings.HasPrefix(r.URL.Path, "/api/") {
		upstream = renderer
	}
	target, err := url.Parse(upstream)
	if err != nil {
		http.Error(w, "Invalid backend URL", http.StatusBadGateway)
		return
	}
	proxy := httputil.ReverseProxy{
		FlushInterval: -1,
		Rewrite: func(p *httputil.ProxyRequest) {
			p.SetURL(target)
			for _, key := range []string{"Cookie", "Referer", "Authorization", "X-Denova-Desktop"} {
				p.Out.Header.Del(key)
			}
			p.Out.Header.Set("Origin", desktopOrigin())
			if upstream == b.url {
				p.Out.Header.Set("X-Denova-Desktop", b.token)
			}
		},
		ModifyResponse: func(response *http.Response) error { response.Header.Del("Set-Cookie"); return nil },
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if r.Context().Err() == nil {
				slog.Error("Desktop proxy failed", "path", r.URL.Path, "error", err)
			}
			http.Error(w, "Backend unavailable", http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(w, r)
}

// Bridge exposes only shell operations. Go's IPC identifies the owning page;
// business requests continue through the same HTTP API as the Web distribution.
type Bridge struct{ shell *shell }

func (b *Bridge) authorize(ctx context.Context) error {
	p := mygo.CallerPage(ctx)
	if p == nil || p.Window() != b.shell.window.Load() || !ownPage(p.URL()) {
		return errors.New("untrusted desktop caller")
	}
	return nil
}

type DirectorySelection struct {
	Path     string `json:"path"`
	Canceled bool   `json:"canceled"`
}
type Appearance struct {
	Locale string `json:"locale"`
	Theme  string `json:"theme"`
}

func (b *Bridge) ChooseDirectory(ctx context.Context, initial string) (DirectorySelection, error) {
	if err := b.authorize(ctx); err != nil {
		return DirectorySelection{}, err
	}
	return b.shell.chooseDirectory(initial)
}

func (b *Bridge) ChangeDataDirectory(ctx context.Context) error {
	if err := b.authorize(ctx); err != nil {
		return err
	}
	return b.shell.changeData(ctx)
}

func (b *Bridge) SetAppearance(ctx context.Context, appearance Appearance) error {
	if err := b.authorize(ctx); err != nil {
		return err
	}
	if (appearance.Locale != "en-US" && appearance.Locale != "zh-CN") || (appearance.Theme != "dark" && appearance.Theme != "light" && appearance.Theme != "system") {
		return errors.New("invalid appearance")
	}
	b.shell.preferencesMu.Lock()
	changed := b.shell.preferences.Locale != appearance.Locale || b.shell.preferences.Theme != appearance.Theme
	b.shell.preferences.Locale, b.shell.preferences.Theme = appearance.Locale, appearance.Theme
	err := b.shell.savePreferencesLocked()
	b.shell.preferencesMu.Unlock()
	if changed {
		// Build plugin labels after the locale is applied, on the same thread.
		mygo.RunOnMain(func() {
			mygo.Theme.SetSource(mygo.ThemeSource(appearance.Theme))
			mygo.App.SetLocale(appearance.Locale)
			b.shell.menus()
		})
	}
	return err
}

func (s *shell) preloadScript(b *backend) string {
	platform := runtime.GOOS
	if platform == "windows" {
		platform = "win32"
	}
	info, _ := json.Marshal(map[string]string{"platform": platform, "backendOrigin": b.url})
	return "const denovaShell=" + string(info) + ";\n" + preload
}

// Only the owning page can ask the official fetch plugin for private APIs.
// The public placeholder is resolved here; the private backend token never
// enters JavaScript and cannot be forwarded to a renderer-selected host.
func (s *shell) allowFetch(r *http.Request) bool {
	p := mygo.CallerPage(r.Context())
	return p != nil && p.Window() == s.window.Load() && ownPage(p.URL()) && r.URL.Scheme == "http" && r.URL.Host == "mygo.localhost" && r.URL.User == nil && strings.HasPrefix(r.URL.Path, "/api/")
}
func (s *shell) RoundTrip(r *http.Request) (*http.Response, error) {
	if !s.allowFetch(r) {
		return nil, errors.New("untrusted desktop API request")
	}
	b := s.backend.Load()
	if b == nil || b.stopping.Load() {
		return nil, errors.New("backend unavailable")
	}
	target, err := url.Parse(b.url)
	if err != nil {
		return nil, err
	}
	request := r.Clone(r.Context())
	request.URL.Scheme, request.URL.Host = target.Scheme, target.Host
	request.Host = target.Host
	for _, key := range []string{"Cookie", "Referer", "Authorization", "X-Denova-Desktop"} {
		request.Header.Del(key)
	}
	request.Header.Set("Origin", desktopOrigin())
	request.Header.Set("X-Denova-Desktop", b.token)
	response, err := http.DefaultTransport.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	response.Header.Del("Set-Cookie")
	return response, nil
}
