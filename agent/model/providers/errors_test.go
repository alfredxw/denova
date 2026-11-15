package providers

import (
	"errors"
	"testing"
)

func TestNewStreamAPIErrorClassifiesInStreamFailures(t *testing.T) {
	cause := errors.New("received error while streaming: upstream broke")
	tests := []struct {
		name        string
		payload     string
		wantCode    string
		wantKind    string
		wantStatus  int
		wantMessage string
		wantRetry   bool
	}{
		{
			name:        "server error type stays retryable",
			payload:     `{"error":{"code":"internal_server_error","message":"local error: tls: bad record MAC","param":null,"type":"server_error"}}`,
			wantCode:    "internal_server_error",
			wantKind:    "server_error",
			wantMessage: "local error: tls: bad record MAC",
			wantRetry:   true,
		},
		{
			name:        "provider request error opts out of retries",
			payload:     `{"error":{"code":"invalid_request_error","message":"bad tool schema","type":"invalid_request_error"}}`,
			wantCode:    "invalid_request_error",
			wantKind:    "invalid_request_error",
			wantMessage: "bad tool schema",
			wantRetry:   false,
		},
		{
			name:        "numeric code is read as the upstream status",
			payload:     `{"error":{"code":503,"message":"unavailable"}}`,
			wantStatus:  503,
			wantMessage: "unavailable",
			wantRetry:   true,
		},
		{
			name:        "unclassified payload remains retryable",
			payload:     `{"error":{"message":"upstream closed"}}`,
			wantKind:    streamErrorKind,
			wantMessage: "upstream closed",
			wantRetry:   true,
		},
		{
			name:        "non JSON payload keeps the SDK message",
			payload:     `upstream closed`,
			wantKind:    streamErrorKind,
			wantMessage: "received error while streaming: upstream broke",
			wantRetry:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			apiError := NewStreamAPIError(cause, []byte(test.payload))
			if apiError.Code != test.wantCode || apiError.Kind != test.wantKind {
				t.Fatalf("code=%q kind=%q, want code=%q kind=%q", apiError.Code, apiError.Kind, test.wantCode, test.wantKind)
			}
			if apiError.StatusCode != test.wantStatus {
				t.Fatalf("status = %d, want %d", apiError.StatusCode, test.wantStatus)
			}
			if apiError.Message != test.wantMessage {
				t.Fatalf("message = %q, want %q", apiError.Message, test.wantMessage)
			}
			if apiError.Retryable() != test.wantRetry {
				t.Fatalf("retryable = %t, want %t", apiError.Retryable(), test.wantRetry)
			}
			if !errors.Is(apiError, cause) {
				t.Fatalf("cause was not preserved: %v", apiError.Unwrap())
			}
		})
	}
}
