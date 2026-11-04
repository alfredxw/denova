package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"denova/internal/buildinfo"
	"denova/internal/desktop/locales"
	"denova/internal/update"
	"github.com/egoist/mygo"
	fetchplugin "github.com/egoist/mygo/plugins/fetch"
	"github.com/egoist/mygo/plugins/updater"
)

// Shell preferences describe this host, and never enter portable .denova data.
type preferences struct {
	DataDir             string                  `json:"dataDir"`
	Locale              string                  `json:"locale"`
	Theme               string                  `json:"theme"`
	Version             string                  `json:"version"`
	CompletedUpdate     *update.CompletedUpdate `json:"completedUpdate,omitempty"`
	BackgroundExplained bool                    `json:"backgroundExplained,omitempty"`
}

type shell struct {
	window              atomic.Pointer[mygo.Window]
	backend             atomic.Pointer[backend]
	tray                *mygo.Tray
	userData, resources string
	preferencesMu       sync.Mutex
	preferences         preferences
	operation           sync.Mutex
	allowQuit           atomic.Bool
	recovering          atomic.Bool
}

func Run() error {
	mygo.App.SetName("Denova")
	if mygo.App.Version() == "" {
		mygo.App.SetVersion(buildinfo.Version)
	}
	userDataOverride := os.Getenv("DENOVA_DESKTOP_USER_DATA")
	for _, arg := range os.Args[1:] {
		if value, ok := strings.CutPrefix(arg, "--desktop-user-data="); ok && userDataOverride == "" {
			userDataOverride = value
		}
	}
	if dir := userDataOverride; dir != "" {
		dir, err := filepath.Abs(dir)
		if err != nil {
			return err
		}
		mygo.App.SetPath(mygo.PathUserData, dir)
		// macOS relaunches through Launch Services, which drops environment
		// overrides. MyGo preserves arguments; keep the selected host store.
		arg := "--desktop-user-data=" + dir
		if !hasArg(arg) {
			os.Args = append(os.Args, arg)
		}
	}
	userData, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Join(userData, "log"), 0700); err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(userData, "log", "desktop.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	// Finder and packaged Windows applications do not own a terminal.
	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{AddSource: true})))
	resources, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		return err
	}
	if override := os.Getenv("DENOVA_DESKTOP_RESOURCES"); override != "" {
		resources = override
	}
	s := &shell{userData: userData, resources: resources}
	if data, err := os.ReadFile(filepath.Join(userData, "desktop.json")); err == nil {
		if err = json.Unmarshal(data, &s.preferences); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if s.preferences.Locale == "" {
		s.preferences.Locale = "en-US"
		if strings.HasPrefix(mygo.App.Locale(), "zh") {
			s.preferences.Locale = "zh-CN"
		}
	}
	if s.preferences.Theme == "" {
		s.preferences.Theme = "dark"
	}
	if dir := os.Getenv("DENOVA_DIR"); dir != "" {
		s.preferences.DataDir, err = filepath.Abs(dir)
		if err != nil {
			return err
		}
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	mygo.Theme.SetSource(mygo.ThemeSource(s.preferences.Theme))
	mygo.Use(updater.New(updater.Options{Interval: 6 * time.Hour, DisableAutomaticChecks: true}), fetchplugin.New(fetchplugin.Options{Client: &http.Client{Transport: s}, Allow: s.allowFetch}))
	mygo.BindAs("Desktop", &Bridge{shell: s})
	if err := mygo.Protocol.Handle("mygo", http.HandlerFunc(s.serve)); err != nil {
		return err
	}
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		slog.Info("Desktop quit requested", "allowed", s.allowQuit.Load())
		if s.allowQuit.Load() {
			return
		}
		if !s.operation.TryLock() {
			slog.Info("Desktop quit deferred", "reason", "another shell operation is running")
			e.PreventDefault()
			return
		}
		defer s.operation.Unlock()
		ok, err := s.confirmStop(context.Background(), "quit")
		if err != nil {
			s.showError(err)
		}
		if err != nil || !ok {
			e.PreventDefault()
			return
		}
		// Do not cancel and replay the native quit: the official updater's
		// Relaunch intent belongs to this event. Drain the child in OnQuit.
		s.allowQuit.Store(true)
		if b := s.backend.Load(); b != nil {
			b.stopping.Store(true)
			_ = b.in.Close()
		}
	})
	mygo.App.OnQuit(func() {
		s.backend.Load().stop()
		slog.Info("Desktop shutdown completed")
	})
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnActivate(func(bool) { s.focus() })
	mygo.App.OnSecondInstance(func([]string, string) { s.focus() })
	mygo.App.WhenReady(func() {
		s.async("startup", func() error {
			if s.preferences.DataDir == "" {
				response, err := mygo.Dialog.Message(mygo.MessageOptions{Title: s.t("dataTitle"), Message: s.t("dataMessage"), Detail: s.t("dataDetail"), Buttons: []string{s.t("useDefault"), s.t("choose"), s.t("quit")}, CancelButton: 2})
				if err != nil {
					return err
				}
				switch response.Button {
				case 0:
					home, _ := os.UserHomeDir()
					s.preferences.DataDir = filepath.Join(home, ".denova")
				case 1:
					ok, err := s.chooseData()
					if err != nil {
						return err
					}
					if !ok {
						s.allowQuit.Store(true)
						mygo.App.Quit()
						return nil
					}
				default:
					s.allowQuit.Store(true)
					mygo.App.Quit()
					return nil
				}
			}
			if err := s.savePreferences(); err != nil {
				return err
			}
			if runtime.GOOS == "windows" {
				icon, err := os.ReadFile(filepath.Join(resources, "tray.png"))
				if err != nil {
					return err
				}
				s.tray, err = mygo.NewTray(mygo.TrayOptions{Icon: icon, ToolTip: "Denova"})
				if err != nil {
					return err
				}
				s.tray.OnClick(s.focus)
			}
			s.operation.Lock()
			if s.allowQuit.Load() {
				s.operation.Unlock()
				return nil
			}
			startErr := s.start()
			s.operation.Unlock()
			if startErr != nil {
				s.recover(startErr)
			}
			return nil
		})
	})
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	done := make(chan struct{})
	defer close(done)
	s.async("signals", func() error {
		select {
		case <-signals:
			mygo.App.Quit()
		case <-done:
		}
		return nil
	})
	return mygo.App.Run()
}

func (s *shell) t(key string) string {
	s.preferencesMu.Lock()
	defer s.preferencesMu.Unlock()
	return locales.Catalog.Format(s.preferences.Locale, key, nil)
}
func (s *shell) savePreferences() error {
	s.preferencesMu.Lock()
	defer s.preferencesMu.Unlock()
	return s.savePreferencesLocked()
}
func (s *shell) savePreferencesLocked() error {
	data, err := json.Marshal(s.preferences)
	if err != nil {
		return err
	}
	file := filepath.Join(s.userData, "desktop.json")
	if err = os.WriteFile(file+".tmp", data, 0600); err != nil {
		return err
	}
	return os.Rename(file+".tmp", file)
}
func (s *shell) async(name string, fn func() error) {
	go func() {
		defer func() {
			if p := recover(); p != nil {
				s.showError(fmt.Errorf("%s panic: %v", name, p))
				if name == "startup" {
					s.allowQuit.Store(true)
					mygo.App.Quit()
				}
			}
		}()
		if err := fn(); err != nil {
			s.showError(fmt.Errorf("%s: %w", name, err))
			if name == "startup" {
				s.allowQuit.Store(true)
				mygo.App.Quit()
			}
		}
	}()
}
func (s *shell) showError(err error) {
	slog.Error("Desktop operation failed", "error", err)
	_, _ = mygo.Dialog.Message(mygo.MessageOptions{Type: mygo.MessageError, Message: s.t("operationFailed"), Buttons: []string{s.t("ok")}})
}
func (s *shell) focus() {
	if w := s.window.Load(); w != nil && !w.IsDestroyed() {
		w.Restore()
		w.Show()
		w.Focus()
	}
}
func (s *shell) chooseDirectory(initial string) (DirectorySelection, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: s.window.Load(), Title: s.t("dataTitle"), DefaultPath: initial, Directory: true, CreateDirectories: true})
	if err != nil {
		return DirectorySelection{}, err
	}
	if len(paths) == 0 {
		return DirectorySelection{Canceled: true}, nil
	}
	return DirectorySelection{Path: paths[0]}, nil
}
func (s *shell) chooseData() (bool, error) {
	s.preferencesMu.Lock()
	dir := s.preferences.DataDir
	s.preferencesMu.Unlock()
	selection, err := s.chooseDirectory(dir)
	if err != nil || selection.Canceled {
		return false, err
	}
	s.preferencesMu.Lock()
	s.preferences.DataDir = selection.Path
	err = s.savePreferencesLocked()
	s.preferencesMu.Unlock()
	return err == nil, err
}
func (s *shell) confirmStop(ctx context.Context, reason string) (bool, error) {
	b := s.backend.Load()
	if b == nil || b.stopping.Load() {
		return true, nil
	}
	var activity activity
	if err := b.request(ctx, "inspect", &activity); err != nil {
		return false, err
	}
	slog.Info("Desktop activity inspected", "reason", reason, "active", activity.Active, "scheduled", activity.Scheduled)
	if !activity.Active && !activity.Scheduled {
		return true, nil
	}
	message, action := "quitMessage", "quit"
	switch reason {
	case "data":
		message, action = "changeDataMessage", "changeDataConfirm"
	}
	result, err := mygo.Dialog.Message(mygo.MessageOptions{Message: s.t(message), Detail: s.t("quitDetail"), Buttons: []string{s.t("cancel"), s.t(action)}, CancelButton: 0})
	return result.Button == 1, err
}
func (s *shell) quit() error { mygo.App.Quit(); return nil }

func (s *shell) changeData(ctx context.Context) error {
	if !s.operation.TryLock() {
		return nil
	}
	defer s.operation.Unlock()
	ok, err := s.confirmStop(ctx, "data")
	if err != nil || !ok {
		return err
	}
	ok, err = s.chooseData()
	if err != nil || !ok {
		return err
	}
	s.backend.Load().stop()
	_ = os.Unsetenv("DENOVA_DIR")
	s.allowQuit.Store(true)
	mygo.App.Relaunch()
	return nil
}

func (s *shell) start() error {
	s.preferencesMu.Lock()
	dir := s.preferences.DataDir
	s.preferencesMu.Unlock()
	b, err := startBackend(s.resources, s.userData, dir, desktopOrigin(), mygo.IsDev() || hasArg("--dev-mode"))
	if err != nil {
		return err
	}
	s.backend.Store(b)
	s.preferencesMu.Lock()
	if b.locale == "en-US" || b.locale == "zh-CN" {
		s.preferences.Locale = b.locale
	}
	version := mygo.App.Version()
	if s.preferences.Version != "" && s.preferences.Version != version {
		s.preferences.CompletedUpdate = &update.CompletedUpdate{ID: "desktop-" + version, PreviousVersion: s.preferences.Version, Version: version, CompletedAt: time.Now().UTC()}
	}
	s.preferences.Version = version
	err = s.savePreferencesLocked()
	s.preferencesMu.Unlock()
	if err != nil {
		b.stop()
		return err
	}
	// Seed the official plugin once from the existing user configuration.
	if _, statErr := os.Stat(filepath.Join(s.userData, "updater.json")); errors.Is(statErr, os.ErrNotExist) {
		var a activity
		if err := b.request(context.Background(), "inspect", &a); err != nil {
			return err
		}
		updater.SetAutomaticDownloads(a.Policy.Download)
		updater.SetAutomaticChecks(a.Policy.Check)
	}
	options := mygo.WindowOptions{Title: "Denova", Width: 1280, Height: 850, MinWidth: 480, MinHeight: 480, Hidden: true, BackgroundColor: "#09090b", Page: mygo.PageOptions{BackgroundThrottling: backgroundThrottling, PreloadScript: s.preloadScript(b)}}
	if runtime.GOOS == "darwin" {
		options.TitleBarStyle = mygo.TitleBarHidden
		options.TrafficLightPosition = &mygo.Point{X: 12, Y: 17}
		options.TitleBarHeight = 48
	}
	w := mygo.NewWindow(options)
	if old := s.window.Swap(w); old != nil {
		old.Destroy()
	}
	w.Page().SetPermissionHandler(func(mygo.PermissionRequest) bool { return false })
	w.Page().SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions { openExternal(req.URL); return nil })
	w.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
		if !ownPage(e.URL) {
			e.PreventDefault()
			openExternal(e.URL)
		}
	})
	w.Page().OnRenderProcessGone(func(reason string) {
		s.async("renderer recovery", func() error { s.recover(fmt.Errorf("renderer exited: %s", reason)); return nil })
	})
	w.OnClose(func(e *mygo.CloseEvent) {
		if s.allowQuit.Load() {
			return
		}
		e.PreventDefault()
		w.Hide()
		if runtime.GOOS == "windows" {
			s.async("background notice", func() error {
				s.preferencesMu.Lock()
				show := !s.preferences.BackgroundExplained
				s.preferences.BackgroundExplained = true
				err := s.savePreferencesLocked()
				s.preferencesMu.Unlock()
				if show {
					_ = mygo.NewNotification(mygo.NotificationOptions{Title: s.t("background"), Body: s.t("backgroundDetail")}).Show()
				}
				return err
			})
		}
	})
	w.OnReadyToShow(s.focus)
	w.Page().OnDidFinishLoad(func() { slog.Info("Desktop page loaded", "url", w.Page().URL(), "webview_pid", w.Page().ProcessID()) })
	mygo.RunOnMain(func() {
		mygo.App.SetLocale(b.locale)
		s.menus()
	})
	if err = w.Page().LoadURL("mygo://localhost/"); err != nil {
		b.stop()
		return err
	}
	s.async("backend monitor", func() error {
		<-b.done
		if !b.stopping.Load() && !s.allowQuit.Load() {
			s.recover(fmt.Errorf("backend exited: %v", b.err))
		}
		return nil
	})
	slog.Info("Desktop ready", "backend_pid", b.cmd.Process.Pid, "version", version)
	return nil
}
func hasArg(value string) bool {
	for _, a := range os.Args[1:] {
		if a == value {
			return true
		}
	}
	return false
}
func openExternal(raw string) {
	u, err := url.Parse(raw)
	if err == nil && (u.Scheme == "https" || u.Scheme == "http" || u.Scheme == "mailto") {
		if err = mygo.Shell.OpenExternal(raw); err != nil {
			slog.Error("Open external link failed", "error", err)
		}
	}
}
func (s *shell) recover(cause error) {
	if s.allowQuit.Load() || !s.recovering.CompareAndSwap(false, true) {
		return
	}
	defer s.recovering.Store(false)
	s.operation.Lock()
	defer s.operation.Unlock()
	slog.Error("Desktop backend unavailable", "error", cause)
	s.backend.Load().stop()
	for !s.allowQuit.Load() {
		result, err := mygo.Dialog.Message(mygo.MessageOptions{Type: mygo.MessageError, Message: s.t("failure"), Detail: s.t("failureDetail"), Buttons: []string{s.t("retry"), s.t("logs"), s.t("choose"), s.t("quit")}, CancelButton: 3})
		if err != nil || result.Button == 3 {
			s.allowQuit.Store(true)
			mygo.App.Quit()
			return
		}
		if result.Button == 1 {
			_ = mygo.Shell.OpenPath(filepath.Join(s.userData, "log"))
			continue
		}
		if result.Button == 2 {
			ok, err := s.chooseData()
			if err != nil {
				slog.Error("Select data directory failed", "error", err)
			}
			if !ok {
				continue
			}
		}
		if err = s.start(); err == nil {
			return
		}
		slog.Error("Desktop recovery failed", "error", err)
	}
}
