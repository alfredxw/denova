package handlers

import (
	"encoding/json"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestParseOverrideChoices(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want map[string]string
		err  bool
	}{
		{"empty", "", map[string]string{}, false},
		{"blank", "  ", map[string]string{}, false},
		{"valid object", `{"Hero":"override","Lily":"keep"}`, map[string]string{"Hero": "override", "Lily": "keep"}, false},
		{"malformed json", `{"Hero":`, nil, true},
		{"non object json", `["override"]`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOverrideChoices(tt.raw)
			if tt.err {
				if err == nil {
					t.Fatalf("expected an error for %q", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.raw, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("choices = %v, want %v", got, tt.want)
			}
			for name, choice := range tt.want {
				if got[name] != choice {
					t.Fatalf("choices = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestInvalidOverrideChoicesUsesRequestLocale(t *testing.T) {
	tests := map[string]string{
		"zh-CN": "同名条目覆盖选择格式无效",
		"en-US": "Invalid same-name entry override choices",
	}
	for locale, want := range tests {
		t.Run(locale, func(t *testing.T) {
			c := app.NewContext(0)
			c.Request.Header.Set("X-Denova-Locale", locale)
			writeErrorKey(c, 400, "api.characterCard.invalidOverrideChoices", "detail", "invalid character")

			var body struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(c.Response.Body(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error == "" || body.Error[:len(want)] != want {
				t.Fatalf("localized error = %q, want prefix %q", body.Error, want)
			}
		})
	}
}