package webaccess

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	rodstealth "github.com/go-rod/stealth"
)

const (
	browserStableWindow        = time.Second
	browserStableDOMDifference = 0.05
	browserMaxRequestBytes     = 1024 * 1024
)

type rodBrowserRenderer struct {
	maxResponseBytes int64
	publicClient     *http.Client

	initializeMu sync.Mutex
	browser      *rod.Browser
	launcher     *launcher.Launcher
	denyProxy    *http.Server
}

type browserNavigationState struct {
	mu         sync.Mutex
	statusCode int
	finalURL   *url.URL
}

func newRodBrowserRenderer(maxResponseBytes int64) browserRenderer {
	publicClient := newPublicHTTPClient()
	publicClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		// Let Chromium observe and follow each redirect so every hop is checked
		// independently by the public-only dialer.
		return http.ErrUseLastResponse
	}
	return &rodBrowserRenderer{maxResponseBytes: maxResponseBytes, publicClient: publicClient}
}

func (renderer *rodBrowserRenderer) Render(ctx context.Context, target *url.URL) (renderedPage, error) {
	browser, err := renderer.ensureBrowser(ctx)
	if err != nil {
		return renderedPage{}, err
	}
	standardPage, standardErr := renderer.renderAttempt(ctx, browser, target, browserRenderModeStandard)
	if standardErr == nil && !browserPageNeedsStealth(standardPage) {
		return standardPage, nil
	}
	if err := ctx.Err(); err != nil {
		return renderedPage{}, err
	}
	if standardErr != nil {
		log.Printf("[webaccess] standard Rod render failed; trying stealth url=%s error=%v", safeURLForLog(target), standardErr)
	} else {
		log.Printf("[webaccess] standard Rod render encountered an access challenge; trying stealth url=%s status=%d", safeURLForLog(target), standardPage.HTTPStatus)
	}
	stealthPage, stealthErr := renderer.renderAttempt(ctx, browser, target, browserRenderModeStealth)
	if stealthErr == nil {
		return stealthPage, nil
	}
	if standardErr != nil {
		return renderedPage{}, fmt.Errorf("standard Rod render failed: %v; go-rod/stealth fallback failed: %w", standardErr, stealthErr)
	}
	standardPage.StealthFallbackError = stealthErr
	return standardPage, nil
}

func (renderer *rodBrowserRenderer) renderAttempt(ctx context.Context, browser *rod.Browser, target *url.URL, mode browserRenderMode) (renderedPage, error) {
	incognito, err := browser.Incognito()
	if err != nil {
		return renderedPage{}, fmt.Errorf("create isolated browser context: %w", err)
	}
	defer func() {
		if closeErr := incognito.Close(); closeErr != nil {
			log.Printf("[webaccess] close isolated browser context: %v", closeErr)
		}
	}()

	var basePage *rod.Page
	if mode == browserRenderModeStealth {
		basePage, err = rodstealth.Page(incognito)
	} else {
		basePage, err = incognito.Page(proto.TargetCreateTarget{})
	}
	if err != nil {
		return renderedPage{}, fmt.Errorf("create %s browser page: %w", mode, err)
	}
	defer func() {
		if closeErr := basePage.Close(); closeErr != nil {
			log.Printf("[webaccess] close browser page: %v", closeErr)
		}
	}()
	page := basePage.Context(ctx)
	cleanupHeaders, err := page.SetExtraHeaders([]string{
		"Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8",
	})
	if err != nil {
		return renderedPage{}, fmt.Errorf("set browser page headers: %w", err)
	}
	defer cleanupHeaders()

	state := &browserNavigationState{}
	router := page.HijackRequests()
	if err := router.Add("*", "", func(hijack *rod.Hijack) {
		renderer.handleBrowserRequest(state, hijack)
	}); err != nil {
		return renderedPage{}, fmt.Errorf("configure browser request boundary: %w", err)
	}
	routerDone := make(chan struct{})
	go func() {
		defer close(routerDone)
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[webaccess] browser request router recovered panic: %v", recovered)
			}
		}()
		router.Run()
	}()
	defer func() {
		if stopErr := router.Stop(); stopErr != nil && ctx.Err() == nil {
			log.Printf("[webaccess] stop browser request router: %v", stopErr)
		}
		<-routerDone
	}()

	log.Printf("[webaccess] rendering public page with Rod mode=%s url=%s", mode, safeURLForLog(target))
	waitForNavigation := page.WaitNavigation(proto.PageLifecycleEventNameDOMContentLoaded)
	if err := page.Navigate(target.String()); err != nil {
		return renderedPage{}, fmt.Errorf("navigate browser page: %w", err)
	}
	waitForNavigation()
	if err := ctx.Err(); err != nil {
		return renderedPage{}, err
	}
	// Network-idle waits never finish on many modern pages because analytics,
	// long polling, and ads stay active after useful content is rendered. DOM
	// stability captures the readable snapshot without imposing a max runtime;
	// cancellation remains owned by the Agent operation context.
	if err := page.WaitDOMStable(browserStableWindow, browserStableDOMDifference); err != nil {
		return renderedPage{}, fmt.Errorf("wait for browser page DOM stability: %w", err)
	}
	html, err := page.HTML()
	if err != nil {
		return renderedPage{}, fmt.Errorf("read browser page HTML: %w", err)
	}
	info, err := page.Info()
	if err != nil {
		return renderedPage{}, fmt.Errorf("read browser page metadata: %w", err)
	}
	finalURL := target
	if parsed, parseErr := validateFetchURL(info.URL); parseErr == nil {
		finalURL = parsed
	}
	statusCode, observedURL := state.snapshot()
	if observedURL != nil {
		finalURL = observedURL
	}
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	return renderedPage{FinalURL: finalURL, HTML: html, HTTPStatus: statusCode, RenderMode: mode}, nil
}

func browserPageNeedsStealth(page renderedPage) bool {
	return browserStatusIndicatesAccessChallenge(page.HTTPStatus) || isLikelyBrowserChallengeDocument(page.HTML)
}

func (renderer *rodBrowserRenderer) ensureBrowser(ctx context.Context) (*rod.Browser, error) {
	renderer.initializeMu.Lock()
	defer renderer.initializeMu.Unlock()
	if renderer.browser != nil {
		return renderer.browser, nil
	}
	binary, found := launcher.LookPath()
	if !found {
		return nil, fmt.Errorf("Chrome, Chromium, or Edge executable was not found; browser fallback does not download a browser automatically")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start browser deny proxy: %w", err)
	}
	denyProxy := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "browser network request was not admitted", http.StatusForbidden)
	})}
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[webaccess] browser deny proxy recovered panic: %v", recovered)
			}
		}()
		if serveErr := denyProxy.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Printf("[webaccess] browser deny proxy stopped unexpectedly: %v", serveErr)
		}
	}()
	proxyURL := "http://" + listener.Addr().String()

	launch := launcher.New().Context(ctx).Bin(binary).
		Proxy(proxyURL).
		Set("proxy-bypass-list", "<-loopback>").
		Set("disable-quic").
		Set("force-webrtc-ip-handling-policy", "disable_non_proxied_udp")
	controlURL, err := launch.Launch()
	if err != nil {
		_ = denyProxy.Close()
		return nil, fmt.Errorf("launch installed browser %q: %w", binary, err)
	}
	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		launch.Kill()
		_ = denyProxy.Close()
		return nil, fmt.Errorf("connect to installed browser: %w", err)
	}
	log.Printf("[webaccess] started shared Rod browser binary=%s", binary)
	renderer.browser = browser
	renderer.launcher = launch
	renderer.denyProxy = denyProxy
	return browser, nil
}

func (renderer *rodBrowserRenderer) handleBrowserRequest(state *browserNavigationState, hijack *rod.Hijack) {
	requestType := hijack.Request.Type()
	if requestType == proto.NetworkResourceTypeImage || requestType == proto.NetworkResourceTypeMedia || requestType == proto.NetworkResourceTypeFont {
		hijack.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
		return
	}
	method := strings.ToUpper(strings.TrimSpace(hijack.Request.Method()))
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions && method != http.MethodPost {
		log.Printf("[webaccess] blocked browser request with unsupported method method=%s url=%s", method, safeURLForLog(hijack.Request.URL()))
		hijack.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
		return
	}
	// The browser context is incognito and receives no ambient user cookies or
	// authentication. A bounded POST is still needed for common anti-bot/session
	// bootstraps used while rendering otherwise public pages.
	if method == http.MethodPost && len(hijack.Request.Body()) > browserMaxRequestBytes {
		log.Printf("[webaccess] blocked oversized browser POST url=%s request_bytes=%d", safeURLForLog(hijack.Request.URL()), len(hijack.Request.Body()))
		hijack.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
		return
	}
	request := hijack.Request.Req().Clone(hijack.Request.Req().Context())
	request.RequestURI = ""
	removeHopByHopHeaders(request.Header)
	response, err := renderer.publicClient.Do(request)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Printf("[webaccess] browser subrequest failed method=%s url=%s error=%v", method, safeURLForLog(request.URL), err)
		}
		hijack.Response.Fail(proto.NetworkErrorReasonConnectionFailed)
		return
	}
	defer response.Body.Close()
	body, err := readBoundedResponse(response, renderer.maxResponseBytes)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Printf("[webaccess] browser subresponse rejected method=%s url=%s error=%v", method, safeURLForLog(request.URL), err)
		}
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
	if hijack.Request.IsNavigation() {
		state.record(response.StatusCode, request.URL)
	}
}

func (state *browserNavigationState) record(statusCode int, finalURL *url.URL) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.statusCode = statusCode
	if finalURL != nil {
		copy := *finalURL
		state.finalURL = &copy
	}
}

func (state *browserNavigationState) snapshot() (int, *url.URL) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.finalURL == nil {
		return state.statusCode, nil
	}
	copy := *state.finalURL
	return state.statusCode, &copy
}

func removeHopByHopHeaders(header http.Header) {
	for _, name := range []string{
		"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
		"Proxy-Connection", "TE", "Trailer", "Transfer-Encoding", "Upgrade",
	} {
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
