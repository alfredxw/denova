package browser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	defaultMaxTabs          = 8
	absoluteMaxTabs         = 16
	maxTabNameRunes         = 64
	maxSelectorRunes        = 2048
	maxInputRunes           = 64 * 1024
	maxWaitTextRunes        = 4096
	maxExpressionRunes      = 64 * 1024
	maxEvaluateResultBytes  = 1024 * 1024
	maxScreenshotBytes      = 16 * 1024 * 1024
	maxObservationTextRunes = 128 * 1024
	maxObservedElements     = 256
	maxElementNameRunes     = 512
)

type tabState struct {
	page      Page
	artifacts []string
}

// Session serializes operations over named tabs. Tool descriptors also mark
// browser calls session-exclusive, but this lock keeps direct callers and
// future hosts safe without relying on one scheduler implementation.
type Session struct {
	mu            sync.Mutex
	driver        Driver
	tabs          map[string]*tabState
	maxTabs       int
	artifactRoot  string
	artifactOwned bool
	validateURL   func(context.Context, string) (string, error)
	shutdown      bool
	shutdownErr   error
}

// NewSession validates the driver with setupCtx but does not retain that
// Context as a second cleanup owner. The creator must call Shutdown and handle
// its result.
func NewSession(setupCtx context.Context, driver Driver, options Options) (*Session, error) {
	if driver == nil {
		return nil, errors.New("browser driver is required")
	}
	if setupCtx == nil {
		setupCtx = context.Background()
	}
	if err := driver.Available(setupCtx); err != nil {
		return nil, err
	}
	maxTabs := options.MaxTabs
	if maxTabs <= 0 {
		maxTabs = defaultMaxTabs
	}
	if maxTabs > absoluteMaxTabs {
		return nil, fmt.Errorf("browser tab limit exceeds %d", absoluteMaxTabs)
	}
	validateURL := options.ValidateURL
	if validateURL == nil {
		validateURL = ValidatePublicURL
	}
	session := &Session{
		driver: driver, tabs: make(map[string]*tabState), maxTabs: maxTabs,
		artifactRoot: strings.TrimSpace(options.ArtifactRoot), validateURL: validateURL,
	}
	return session, nil
}

func (session *Session) Open(ctx context.Context, request OpenRequest) (Result, error) {
	if err := contextError(ctx); err != nil {
		return Result{}, err
	}
	tab, err := normalizeTabName(request.Tab)
	if err != nil {
		return Result{}, err
	}
	urlValue := strings.TrimSpace(request.URL)
	if urlValue != "" {
		urlValue, err = session.validateURL(ctx, urlValue)
		if err != nil {
			return Result{}, err
		}
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	if err := session.ensureOpen(); err != nil {
		return Result{}, err
	}
	state := session.tabs[tab]
	created := state == nil
	if created {
		if len(session.tabs) >= session.maxTabs {
			return Result{}, fmt.Errorf("browser session may keep at most %d named tabs", session.maxTabs)
		}
		page, pageErr := session.driver.NewPage(ctx)
		if pageErr != nil {
			return Result{}, fmt.Errorf("create browser tab %q: %w", tab, pageErr)
		}
		state = &tabState{page: page}
	}
	if urlValue != "" {
		if err := state.page.Navigate(ctx, urlValue); err != nil {
			if created {
				_ = state.page.Close(context.Background())
			}
			return Result{}, fmt.Errorf("navigate browser tab %q: %w", tab, err)
		}
	}
	if created {
		session.tabs[tab] = state
	}
	observation, err := session.observe(ctx, state)
	if err != nil {
		observeErr := fmt.Errorf("observe browser tab %q: %w", tab, err)
		if created {
			delete(session.tabs, tab)
			if closeErr := state.page.Close(context.Background()); closeErr != nil {
				observeErr = errors.Join(observeErr, fmt.Errorf("close failed browser tab %q: %w", tab, closeErr))
			}
		}
		return Result{}, observeErr
	}
	return session.result("open", tab, "", &observation, nil, nil), nil
}

func (session *Session) Run(ctx context.Context, request RunRequest) (Result, error) {
	if err := contextError(ctx); err != nil {
		return Result{}, err
	}
	tab, err := normalizeTabName(request.Tab)
	if err != nil {
		return Result{}, err
	}
	command := strings.ToLower(strings.TrimSpace(request.Command))
	if request.TimeoutSeconds != 0 && command != CommandWait {
		return Result{}, errors.New("browser timeout_seconds is supported only by wait")
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	if err := session.ensureOpen(); err != nil {
		return Result{}, err
	}
	state := session.tabs[tab]
	if state == nil {
		return Result{}, fmt.Errorf("browser tab %q is not open", tab)
	}

	var value json.RawMessage
	var artifact *ScreenshotArtifact
	switch command {
	case CommandObserve:
	case CommandGoto:
		target, validateErr := session.validateURL(ctx, request.URL)
		if validateErr != nil {
			return Result{}, validateErr
		}
		if err := state.page.Navigate(ctx, target); err != nil {
			return Result{}, fmt.Errorf("navigate browser tab %q: %w", tab, err)
		}
	case CommandWait:
		selector := strings.TrimSpace(request.Selector)
		if utf8.RuneCountInString(selector) > maxSelectorRunes {
			return Result{}, fmt.Errorf("browser selector exceeds %d characters", maxSelectorRunes)
		}
		text := strings.TrimSpace(request.Text)
		if utf8.RuneCountInString(text) > maxWaitTextRunes {
			return Result{}, fmt.Errorf("browser wait text exceeds %d characters", maxWaitTextRunes)
		}
		if selector == "" && text == "" {
			return Result{}, errors.New("browser wait requires selector or text")
		}
		if request.TimeoutSeconds < 0 {
			return Result{}, errors.New("browser wait timeout_seconds cannot be negative")
		}
		if int64(request.TimeoutSeconds) > math.MaxInt64/int64(time.Second) {
			return Result{}, errors.New("browser wait timeout_seconds is too large")
		}
		waitCtx := ctx
		if waitCtx == nil {
			waitCtx = context.Background()
		}
		cancel := func() {}
		if request.TimeoutSeconds > 0 {
			waitCtx, cancel = context.WithTimeout(waitCtx, time.Duration(request.TimeoutSeconds)*time.Second)
		}
		waitErr := state.page.Wait(waitCtx, WaitCondition{Selector: selector, Text: text})
		cancel()
		if waitErr != nil {
			return Result{}, fmt.Errorf("wait in browser tab %q: %w", tab, waitErr)
		}
	case CommandClick:
		selector, validateErr := boundedRequired("selector", request.Selector, maxSelectorRunes)
		if validateErr != nil {
			return Result{}, validateErr
		}
		if err := state.page.Click(ctx, selector); err != nil {
			return Result{}, fmt.Errorf("click %q in browser tab %q: %w", selector, tab, err)
		}
	case CommandFill, CommandType:
		selector, validateErr := boundedRequired("selector", request.Selector, maxSelectorRunes)
		if validateErr != nil {
			return Result{}, validateErr
		}
		if utf8.RuneCountInString(request.Text) > maxInputRunes {
			return Result{}, fmt.Errorf("browser text exceeds %d characters", maxInputRunes)
		}
		if command == CommandFill {
			err = state.page.Fill(ctx, selector, request.Text)
		} else {
			err = state.page.Type(ctx, selector, request.Text)
		}
		if err != nil {
			return Result{}, fmt.Errorf("%s %q in browser tab %q: %w", command, selector, tab, err)
		}
	case CommandPress:
		key, validateErr := boundedRequired("key", request.Key, 64)
		if validateErr != nil {
			return Result{}, validateErr
		}
		selector := strings.TrimSpace(request.Selector)
		if utf8.RuneCountInString(selector) > maxSelectorRunes {
			return Result{}, fmt.Errorf("browser selector exceeds %d characters", maxSelectorRunes)
		}
		if err := state.page.Press(ctx, selector, key); err != nil {
			return Result{}, fmt.Errorf("press %q in browser tab %q: %w", key, tab, err)
		}
	case CommandSelect:
		selector, validateErr := boundedRequired("selector", request.Selector, maxSelectorRunes)
		if validateErr != nil {
			return Result{}, validateErr
		}
		values, validateErr := normalizeSelectValues(request.Values)
		if validateErr != nil {
			return Result{}, validateErr
		}
		if err := state.page.Select(ctx, selector, values); err != nil {
			return Result{}, fmt.Errorf("select values in browser tab %q: %w", tab, err)
		}
	case CommandEvaluate:
		expression, validateErr := boundedRequired("expression", request.Expression, maxExpressionRunes)
		if validateErr != nil {
			return Result{}, validateErr
		}
		value, err = state.page.Evaluate(ctx, expression)
		if err != nil {
			return Result{}, fmt.Errorf("evaluate in browser tab %q: %w", tab, err)
		}
		if !json.Valid(value) {
			return Result{}, errors.New("browser evaluation returned invalid JSON")
		}
		if len(value) > maxEvaluateResultBytes {
			return Result{}, fmt.Errorf("browser evaluation result exceeds %d bytes", maxEvaluateResultBytes)
		}
	case CommandScreenshot:
		image, screenshotErr := state.page.Screenshot(ctx, request.FullPage)
		if screenshotErr != nil {
			return Result{}, fmt.Errorf("screenshot browser tab %q: %w", tab, screenshotErr)
		}
		artifact, err = session.writeScreenshot(tab, image)
		if err != nil {
			return Result{}, err
		}
		state.artifacts = append(state.artifacts, artifact.Path)
	default:
		return Result{}, fmt.Errorf("unsupported browser command %q", command)
	}

	observation, err := session.observe(ctx, state)
	if err != nil {
		return Result{}, fmt.Errorf("observe browser tab %q after %s: %w", tab, command, err)
	}
	return session.result("run", tab, command, &observation, value, artifact), nil
}

func (session *Session) Close(ctx context.Context, request CloseRequest) (Result, error) {
	if err := contextError(ctx); err != nil {
		return Result{}, err
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if err := session.ensureOpen(); err != nil {
		return Result{}, err
	}
	if request.All {
		if strings.TrimSpace(request.Tab) != "" {
			return Result{}, errors.New("browser close accepts tab or all=true, not both")
		}
		if err := session.closeTabs(ctx); err != nil {
			return Result{}, err
		}
		return session.result("close", "", "", nil, nil, nil), nil
	}
	tab, err := normalizeTabName(request.Tab)
	if err != nil {
		return Result{}, errors.New("browser close requires tab or all=true")
	}
	state := session.tabs[tab]
	if state == nil {
		return Result{}, fmt.Errorf("browser tab %q is not open", tab)
	}
	if err := session.closeTab(ctx, tab, state); err != nil {
		return Result{}, err
	}
	return session.result("close", tab, "", nil, nil, nil), nil
}

// Shutdown closes every page, removes generated artifacts, and releases the
// isolated browser process. It is idempotent, but ownership is explicit: the
// caller that created the Session must invoke Shutdown and observe its error.
// In the Agent host that owner is InvocationResource. Keeping cancellation and
// cleanup separate prevents a cancellation watcher from winning the shutdown
// race and hiding the real cleanup error from the invocation finisher.
func (session *Session) Shutdown(ctx context.Context) error {
	if session == nil {
		return nil
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.shutdown {
		return session.shutdownErr
	}
	session.shutdown = true
	closeErr := session.closeTabs(ctx)
	driverErr := session.driver.Close(ctx)
	var artifactErr error
	if session.artifactOwned && session.artifactRoot != "" {
		artifactErr = os.RemoveAll(session.artifactRoot)
	}
	session.shutdownErr = errors.Join(closeErr, driverErr, artifactErr)
	return session.shutdownErr
}

func (session *Session) ensureOpen() error {
	if session == nil || session.driver == nil {
		return errors.New("browser session is not configured")
	}
	if session.shutdown {
		return errors.New("browser session is closed")
	}
	return nil
}

func (session *Session) observe(ctx context.Context, state *tabState) (Observation, error) {
	if state == nil || state.page == nil {
		return Observation{}, errors.New("browser page is not configured")
	}
	observation, err := state.page.Observe(ctx)
	if err != nil {
		return Observation{}, err
	}
	currentURL := strings.TrimSpace(observation.URL)
	if currentURL != "" && currentURL != "about:blank" {
		normalized, validateErr := session.validateURL(ctx, currentURL)
		if validateErr != nil {
			return Observation{}, fmt.Errorf("browser page left the admitted HTTP(S) boundary: %w", validateErr)
		}
		observation.URL = normalized
	}
	return boundedObservation(observation), nil
}

func (session *Session) result(action, tab, command string, observation *Observation, value json.RawMessage, artifact *ScreenshotArtifact) Result {
	tabs := make([]string, 0, len(session.tabs))
	for name := range session.tabs {
		tabs = append(tabs, name)
	}
	sort.Strings(tabs)
	target := tab
	if artifact != nil && strings.TrimSpace(artifact.Path) != "" {
		target = artifact.Path
	} else if observation != nil && strings.TrimSpace(observation.URL) != "" {
		target = observation.URL
	}
	return Result{
		Schema: "browser.result.v1", Status: "completed", Action: action,
		Tab: tab, Command: command, Tabs: tabs, Observation: observation,
		Value: value, Screenshot: artifact,
		Receipt: ExternalReceipt{
			Schema: "external_effect.receipt.v1", Boundary: "browser",
			Operation: firstNonEmpty(command, action), Target: target, Status: "completed",
		},
	}
}

func (session *Session) closeTabs(ctx context.Context) error {
	names := make([]string, 0, len(session.tabs))
	for name := range session.tabs {
		names = append(names, name)
	}
	sort.Strings(names)
	var closeErrors []error
	for _, name := range names {
		if err := session.closeTab(ctx, name, session.tabs[name]); err != nil {
			closeErrors = append(closeErrors, err)
		}
	}
	return errors.Join(closeErrors...)
}

func (session *Session) closeTab(ctx context.Context, name string, state *tabState) error {
	delete(session.tabs, name)
	var closeErrors []error
	if state != nil && state.page != nil {
		if err := state.page.Close(ctx); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("close browser tab %q: %w", name, err))
		}
	}
	if state != nil {
		for _, artifact := range state.artifacts {
			if err := os.Remove(artifact); err != nil && !errors.Is(err, os.ErrNotExist) {
				closeErrors = append(closeErrors, fmt.Errorf("remove browser artifact: %w", err))
			}
		}
	}
	return errors.Join(closeErrors...)
}

func (session *Session) writeScreenshot(tab string, content []byte) (*ScreenshotArtifact, error) {
	if len(content) == 0 {
		return nil, errors.New("browser returned an empty screenshot")
	}
	if len(content) > maxScreenshotBytes {
		return nil, fmt.Errorf("browser screenshot exceeds %d bytes", maxScreenshotBytes)
	}
	root := session.artifactRoot
	if root == "" {
		created, err := os.MkdirTemp("", "denova-browser-artifacts-")
		if err != nil {
			return nil, fmt.Errorf("create browser artifact directory: %w", err)
		}
		root = created
		session.artifactRoot = root
		session.artifactOwned = true
	} else if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create browser artifact directory: %w", err)
	}
	file, err := os.CreateTemp(root, tab+"-*.png")
	if err != nil {
		return nil, fmt.Errorf("create browser screenshot: %w", err)
	}
	path := file.Name()
	defer func() { _ = file.Close() }()
	if err := file.Chmod(0o600); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("protect browser screenshot: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("write browser screenshot: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("close browser screenshot: %w", err)
	}
	digest := sha256.Sum256(content)
	absolute, err := filepath.Abs(path)
	if err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("resolve browser screenshot path: %w", err)
	}
	return &ScreenshotArtifact{
		Path: absolute, MIMEType: "image/png", Bytes: len(content), SHA256: hex.EncodeToString(digest[:]),
	}, nil
}

func normalizeTabName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("browser tab name is required")
	}
	if utf8.RuneCountInString(value) > maxTabNameRunes {
		return "", fmt.Errorf("browser tab name exceeds %d characters", maxTabNameRunes)
	}
	for _, character := range value {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '-' && character != '_' && character != '.' {
			return "", errors.New("browser tab name may contain only letters, numbers, '-', '_', and '.'")
		}
	}
	return value, nil
}

func boundedRequired(field, value string, maximum int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("browser %s is required", field)
	}
	if utf8.RuneCountInString(value) > maximum {
		return "", fmt.Errorf("browser %s exceeds %d characters", field, maximum)
	}
	return value, nil
}

func normalizeSelectValues(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, errors.New("browser select requires at least one value")
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, errors.New("browser select values must not be empty")
		}
		if utf8.RuneCountInString(value) > 4096 {
			return nil, errors.New("browser select value exceeds 4096 characters")
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func boundedObservation(observation Observation) Observation {
	observation.URL, observation.Truncated = truncateRunes(observation.URL, 4096, observation.Truncated)
	observation.Title, observation.Truncated = truncateRunes(observation.Title, 1000, observation.Truncated)
	observation.Text, observation.Truncated = truncateRunes(observation.Text, maxObservationTextRunes, observation.Truncated)
	if len(observation.Elements) > maxObservedElements {
		observation.Elements = observation.Elements[:maxObservedElements]
		observation.Truncated = true
	}
	for index := range observation.Elements {
		element := &observation.Elements[index]
		element.Ref, observation.Truncated = truncateRunes(element.Ref, 64, observation.Truncated)
		element.Role, observation.Truncated = truncateRunes(element.Role, 128, observation.Truncated)
		element.Name, observation.Truncated = truncateRunes(element.Name, maxElementNameRunes, observation.Truncated)
		element.Selector, observation.Truncated = truncateRunes(element.Selector, maxSelectorRunes, observation.Truncated)
	}
	return observation
}

func truncateRunes(value string, maximum int, already bool) (string, bool) {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maximum {
		return string(runes), already
	}
	return string(runes[:maximum]), true
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
