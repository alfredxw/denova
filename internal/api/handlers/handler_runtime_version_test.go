package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"denova/internal/agents/runtime/external/claude"
	"denova/internal/agents/runtime/external/codex"
	"github.com/cloudwego/hertz/pkg/app"
)

func TestRuntimeVersionErrorsIdentifyProviderAndUpgrade(t *testing.T) {
	for _, provider := range []struct {
		name, minimum, command string
		err                    error
	}{
		{"Codex CLI", codex.MinimumVersion, "codex update", codex.ErrVersionUnsupported},
		{"Claude Code", claude.MinimumVersion, "claude update", claude.ErrVersionUnsupported},
	} {
		for _, locale := range []string{"zh-CN", "en-US"} {
			for _, endpoint := range []string{"engine", "conversation"} {
				t.Run(provider.name+"/"+locale+"/"+endpoint, func(t *testing.T) {
					c := app.NewContext(0)
					c.Request.Header.Set("X-Denova-Locale", locale)
					err := fmt.Errorf("connect runtime: %w", provider.err)
					if endpoint == "engine" {
						writeEngineError(context.Background(), c, err)
					} else {
						writeConversationConfigError(c, err)
					}
					if c.Response.StatusCode() != 503 {
						t.Fatalf("status = %d", c.Response.StatusCode())
					}
					var body struct{ Error string }
					if err := json.Unmarshal(c.Response.Body(), &body); err != nil {
						t.Fatal(err)
					}
					for _, want := range []string{provider.name, provider.minimum, provider.command} {
						if !strings.Contains(body.Error, want) {
							t.Fatalf("error %q does not contain %q", body.Error, want)
						}
					}
				})
			}
		}
	}
}
