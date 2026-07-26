package browser

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
)

const (
	rodMaxRequestBytes  = 1024 * 1024
	rodMaxResponseBytes = 16 * 1024 * 1024
)

type RodDriver struct {
	mu        sync.Mutex
	lifetime  context.Context
	binary    string
	client    *http.Client
	browser   *rod.Browser
	incognito *rod.Browser
	launcher  *launcher.Launcher
	denyProxy *http.Server
	userData  string
	closed    bool
}

// NewRodDriver discovers an installed Chrome, Chromium, or Edge executable.
// It never downloads or launches a browser; the process starts lazily on the
// first open call and inherits the supplied Agent lifetime.
func NewRodDriver(lifetime context.Context) (*RodDriver, error) {
	if lifetime == nil {
		lifetime = context.Background()
	}
	binary, found := launcher.LookPath()
	if !found {
		return nil, fmt.Errorf("%w: Chrome, Chromium, or Edge was not found", ErrUnavailable)
	}
	client := newPublicHTTPClient()
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		// Chromium follows redirects itself, causing every hop to cross the
		// public-address policy independently.
		return http.ErrUseLastResponse
	}
	return &RodDriver{lifetime: lifetime, binary: binary, client: client}, nil
}

func (driver *RodDriver) Available(ctx context.Context) error {
	if driver == nil || strings.TrimSpace(driver.binary) == "" {
		return ErrUnavailable
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	info, err := os.Stat(driver.binary)
	if err != nil || info.IsDir() {
		return fmt.Errorf("%w: installed browser executable cannot be used", ErrUnavailable)
	}
	return nil
}

func (driver *RodDriver) NewPage(ctx context.Context) (Page, error) {
	if err := driver.ensureBrowser(ctx); err != nil {
		return nil, err
	}
	driver.mu.Lock()
	isolated := driver.incognito
	driver.mu.Unlock()
	if isolated == nil {
		return nil, errors.New("isolated browser context is unavailable")
	}
	base, err := isolated.Page(proto.TargetCreateTarget{})
	if err != nil {
		return nil, fmt.Errorf("create isolated browser page: %w", err)
	}
	page := base.Context(driver.lifetime)
	removeHeaders, err := page.SetExtraHeaders([]string{"Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8"})
	if err != nil {
		_ = base.Close()
		return nil, fmt.Errorf("set browser page headers: %w", err)
	}
	router := page.HijackRequests()
	if err := router.Add("*", "", func(hijack *rod.Hijack) { driver.handleRequest(hijack) }); err != nil {
		removeHeaders()
		_ = base.Close()
		return nil, fmt.Errorf("configure browser request boundary: %w", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[browser] recovered request-router panic: %v", recovered)
			}
		}()
		router.Run()
	}()
	return &rodPage{page: page, router: router, routerDone: done, removeHeaders: removeHeaders}, nil
}

func (driver *RodDriver) Close(context.Context) error {
	if driver == nil {
		return nil
	}
	driver.mu.Lock()
	defer driver.mu.Unlock()
	if driver.closed {
		return nil
	}
	driver.closed = true
	var closeErrors []error
	if driver.incognito != nil {
		if err := driver.incognito.Close(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("close isolated browser context: %w", err))
		}
	}
	var browserCloseErr error
	if driver.browser != nil {
		browserCloseErr = driver.browser.Close()
		if browserCloseErr != nil {
			closeErrors = append(closeErrors, fmt.Errorf("close browser process: %w", browserCloseErr))
		}
	}
	if driver.launcher != nil {
		if browserCloseErr != nil {
			driver.launcher.Kill()
		}
		driver.launcher.Cleanup()
		driver.userData = ""
	}
	if driver.denyProxy != nil {
		if err := driver.denyProxy.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			closeErrors = append(closeErrors, fmt.Errorf("close browser deny proxy: %w", err))
		}
	}
	if driver.userData != "" {
		if err := os.RemoveAll(driver.userData); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("remove isolated browser profile: %w", err))
		}
	}
	return errors.Join(closeErrors...)
}

func (driver *RodDriver) ensureBrowser(ctx context.Context) error {
	if driver == nil {
		return ErrUnavailable
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	driver.mu.Lock()
	defer driver.mu.Unlock()
	if driver.closed {
		return errors.New("browser driver is closed")
	}
	if driver.incognito != nil {
		return nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("start browser deny proxy: %w", err)
	}
	denyProxy := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "browser network request was not admitted", http.StatusForbidden)
	})}
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[browser] recovered deny-proxy panic: %v", recovered)
			}
		}()
		if serveErr := denyProxy.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Printf("[browser] deny proxy stopped unexpectedly: %v", serveErr)
		}
	}()
	launch := launcher.New().Context(driver.lifetime).Bin(driver.binary).
		Proxy("http://"+listener.Addr().String()).
		Set("proxy-bypass-list", "<-loopback>").
		Set("disable-quic").
		Set("force-webrtc-ip-handling-policy", "disable_non_proxied_udp")
	controlURL, err := launch.Launch()
	if err != nil {
		_ = denyProxy.Close()
		_ = os.RemoveAll(launch.Get(flags.UserDataDir))
		return fmt.Errorf("launch installed browser: %w", err)
	}
	root := rod.New().ControlURL(controlURL).Context(driver.lifetime)
	if err := root.Connect(); err != nil {
		launch.Kill()
		launch.Cleanup()
		_ = denyProxy.Close()
		return fmt.Errorf("connect to installed browser: %w", err)
	}
	isolated, err := root.Incognito()
	if err != nil {
		if closeErr := root.Close(); closeErr != nil {
			launch.Kill()
		}
		launch.Cleanup()
		_ = denyProxy.Close()
		return fmt.Errorf("create isolated browser context: %w", err)
	}
	driver.browser = root
	driver.incognito = isolated
	driver.launcher = launch
	driver.denyProxy = denyProxy
	driver.userData = launch.Get(flags.UserDataDir)
	log.Printf("[browser] started isolated browser runtime binary=%s", filepathBase(driver.binary))
	return nil
}

func (driver *RodDriver) handleRequest(hijack *rod.Hijack) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[browser] recovered request handler panic: %v", recovered)
			hijack.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
		}
	}()
	resourceType := hijack.Request.Type()
	if resourceType == proto.NetworkResourceTypeMedia || resourceType == proto.NetworkResourceTypeFont {
		hijack.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
		return
	}
	method := strings.ToUpper(strings.TrimSpace(hijack.Request.Method()))
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPost,
		http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		hijack.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
		return
	}
	if len(hijack.Request.Body()) > rodMaxRequestBytes {
		hijack.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
		return
	}
	request := hijack.Request.Req().Clone(hijack.Request.Req().Context())
	request.RequestURI = ""
	removeHopByHopHeaders(request.Header)
	response, err := driver.client.Do(request)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Printf("[browser] blocked or failed subrequest method=%s url=%s error=%v", method, safeURL(request.URL), err)
		}
		hijack.Response.Fail(proto.NetworkErrorReasonConnectionFailed)
		return
	}
	defer response.Body.Close()
	body, err := readBoundedBody(response, rodMaxResponseBytes)
	if err != nil {
		hijack.Response.Fail(proto.NetworkErrorReasonBlockedByResponse)
		return
	}
	payload := hijack.Response.Payload()
	payload.ResponseCode = response.StatusCode
	payload.ResponsePhrase = http.StatusText(response.StatusCode)
	for name, values := range response.Header {
		if isHopByHopHeader(name) {
			continue
		}
		for _, value := range values {
			hijack.Response.SetHeader(name, value)
		}
	}
	hijack.Response.SetBody(body)
}

func readBoundedBody(response *http.Response, maximum int64) ([]byte, error) {
	if response.ContentLength > maximum {
		return nil, fmt.Errorf("browser response exceeds %d bytes", maximum)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maximum {
		return nil, fmt.Errorf("browser response exceeds %d bytes", maximum)
	}
	return content, nil
}

func removeHopByHopHeaders(header http.Header) {
	for _, name := range []string{"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Proxy-Connection", "TE", "Trailer", "Transfer-Encoding", "Upgrade"} {
		header.Del(name)
	}
}

func isHopByHopHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Proxy-Connection", "Te", "Trailer", "Transfer-Encoding", "Upgrade":
		return true
	default:
		return false
	}
}

func safeURL(value *url.URL) string {
	if value == nil {
		return ""
	}
	return (&url.URL{Scheme: value.Scheme, Host: value.Host, Path: value.Path}).String()
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func filepathBase(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	if index := strings.LastIndex(path, "/"); index >= 0 {
		return path[index+1:]
	}
	return path
}
