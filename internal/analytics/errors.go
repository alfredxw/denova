package analytics

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"time"

	"github.com/alfredxw/denova/agent/model/providers"
)

type ErrorCode string

const (
	ErrorUnknown        ErrorCode = "unknown"
	ErrorTimeout        ErrorCode = "timeout"
	ErrorCancelled      ErrorCode = "cancelled"
	ErrorNetwork        ErrorCode = "network"
	ErrorAuthentication ErrorCode = "authentication"
	ErrorRateLimit      ErrorCode = "rate_limit"
	ErrorProvider       ErrorCode = "provider_unavailable"
	ErrorInvalidRequest ErrorCode = "invalid_request"
	ErrorPermission     ErrorCode = "permission_denied"
	ErrorNotFound       ErrorCode = "not_found"
	ErrorHTTP           ErrorCode = "http_error"
	ErrorLocalOnly      ErrorCode = "local_only"
	ErrorConflict       ErrorCode = "conflict"
	ErrorNotConfigured  ErrorCode = "not_configured"
	ErrorPanic          ErrorCode = "panic_recovered"
	ErrorRender         ErrorCode = "react_error"
	ErrorWindow         ErrorCode = "window_error"
	ErrorRejection      ErrorCode = "unhandled_rejection"
	ErrorWhiteScreen    ErrorCode = "white_screen"
	ErrorStartup        ErrorCode = "startup"
)

type ErrorSource string

const (
	Frontend ErrorSource = "frontend"
	Backend  ErrorSource = "backend"
	API      ErrorSource = "api"
)

func classifyError(err error) ErrorCode {
	var upstream *providers.APIError
	var network net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return ErrorCancelled
	case errors.Is(err, context.DeadlineExceeded):
		return ErrorTimeout
	case errors.Is(err, fs.ErrPermission):
		return ErrorPermission
	case errors.Is(err, fs.ErrNotExist):
		return ErrorNotFound
	case errors.As(err, &upstream):
		switch {
		case upstream.StatusCode == 401 || upstream.StatusCode == 403:
			return ErrorAuthentication
		case upstream.StatusCode == 429:
			return ErrorRateLimit
		case upstream.StatusCode == 408 || upstream.StatusCode == 504:
			return ErrorTimeout
		case upstream.StatusCode >= 500:
			return ErrorProvider
		case upstream.StatusCode >= 400:
			return ErrorInvalidRequest
		}
	case errors.As(err, &network):
		if network.Timeout() {
			return ErrorTimeout
		}
		return ErrorNetwork
	}
	return ErrorUnknown
}

// ValidClientError is deliberately independent of an error's message, stack,
// filename and URL. Only these fixed browser failure classes are accepted.
func ValidClientError(code ErrorCode) bool {
	switch code {
	case ErrorRender, ErrorWindow, ErrorRejection, ErrorWhiteScreen, ErrorStartup:
		return true
	}
	return false
}

func (s *Service) ClientError(code ErrorCode) {
	if !ValidClientError(code) {
		return
	}
	s.recordError(Frontend, code, "", "", 0)
}

// APIError accepts closed server-chosen categories, never routes, response bodies
// or raw error codes. HTTP errors are separate from run results.
func (s *Service) APIError(feature Feature, operation string, code ErrorCode, status int) {
	if status < 400 || status > 599 || (feature != "" && !feature.Valid()) {
		return
	}
	switch operation {
	case "read", "create", "update", "delete", "other", "book_cover", "plugin_pages", "extension_catalog", "extension_development", "extension_runtimes", "game_instances", "game_preferences":
	default:
		return
	}
	switch code {
	case ErrorHTTP, ErrorLocalOnly, ErrorConflict, ErrorNotConfigured, ErrorAuthentication, ErrorPermission, ErrorNotFound, ErrorRateLimit, ErrorTimeout, ErrorInvalidRequest:
	default:
		return
	}
	s.recordError(API, code, feature, operation, status)
}

// RecoveredPanic is not a claim that the process crashed. Hard termination and
// failures before analytics consent may have no report at all.
func (s *Service) RecoveredPanic() { s.recordError(Backend, ErrorPanic, "", "", 0) }

func (s *Service) recordError(source ErrorSource, code ErrorCode, feature Feature, operation string, status int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.collectingLocked() {
		return
	}
	s.enqueueLocked(s.newEvent("app_error", eventData{Feature: feature, Operation: operation, ErrorSource: source, ErrorCode: code, StatusCode: status}, time.Now().UTC()))
}
