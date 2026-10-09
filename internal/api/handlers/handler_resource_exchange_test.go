package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"denova/internal/app/resourceexchange"
	"denova/internal/platform"
	"github.com/cloudwego/hertz/pkg/app"
)

func TestExchangeErrorIncludesSafeDiagnostics(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
		code   string
		detail string
	}{
		{
			name: "invalid extension manifest",
			err: &platform.Error{Code: "INVALID_ARGUMENT", MessageKey: "platform.errors.INVALID_ARGUMENT",
				Diagnostic: "minHostVersion must be a semantic version: version string empty"},
			status: 400, code: "market.errors.operationFailed", detail: "minHostVersion",
		},
		{
			name:   "local modification",
			err:    fmt.Errorf("resource fixture: %w", resourceexchange.ErrLocalModified),
			status: 409, code: "market.errors.localModified", detail: "fixture",
		},
		{
			name:   "redacted source diagnostics",
			err:    fmt.Errorf("download https://author:password@example.com/archive.zip?token=secret failed at /Users/example/private.json"),
			status: 400, code: "market.errors.operationFailed", detail: "archive.zip",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := app.NewContext(0)
			c.Request.Header.Set("X-Denova-Locale", "zh-CN")
			exchangeError(context.Background(), c, tt.err)
			var body agentRuntimeErrorResponse
			if err := json.Unmarshal(c.Response.Body(), &body); err != nil {
				t.Fatal(err)
			}
			detail, _ := body.Details["detail"].(string)
			if c.Response.StatusCode() != tt.status || body.Code != tt.code || !strings.Contains(detail, tt.detail) {
				t.Fatalf("status=%d body=%+v", c.Response.StatusCode(), body)
			}
			for _, secret := range []string{"password", "secret", "/Users/example"} {
				if strings.Contains(detail, secret) {
					t.Fatalf("diagnostics expose %q: %s", secret, detail)
				}
			}
		})
	}
}
