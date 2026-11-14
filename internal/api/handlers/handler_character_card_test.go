package handlers

import (
	"encoding/json"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestParseOverrideReview(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []characterCardOverrideReview
		err  bool
	}{
		{"empty", "", nil, false},
		{"blank", "  ", nil, false},
		{"valid array", `[{"name":"Hero","existing_id":"id-1","existing_version":"rev-1","choice":"override"},{"name":"Lily","existing_id":"id-2","existing_version":"rev-2","choice":"keep"}]`,
			[]characterCardOverrideReview{
				{Name: "Hero", ExistingID: "id-1", ExistingVersion: "rev-1", Choice: "override"},
				{Name: "Lily", ExistingID: "id-2", ExistingVersion: "rev-2", Choice: "keep"},
			}, false},
		{"missing name", `[{"choice":"override"}]`, nil, true},
		{"invalid choice", `[{"name":"Hero","choice":"replace"}]`, nil, true},
		{"duplicate name", `[{"name":"Hero","choice":"override"},{"name":"Hero","choice":"keep"}]`, nil, true},
		{"malformed json", `[{"name":"Hero",`, nil, true},
		{"non array json", `{"Hero":"override"}`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOverrideReview(tt.raw)
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
				t.Fatalf("review = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("review = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestConfirmedReviewBinding(t *testing.T) {
	fields := characterCardImportFields{overrideReview: []characterCardOverrideReview{
		{Name: "Hero", ExistingID: "id-1", ExistingVersion: "rev-1", Choice: "override"},
		{Name: "Lily", ExistingID: "id-2", ExistingVersion: "rev-2", Choice: "keep"},
	}}
	confirm := fields.confirmedReview()
	if confirm == nil {
		t.Fatal("expected a non-nil confirm binding")
	}
	if len(confirm.Duplicates) != 2 || len(confirm.Choices) != 2 {
		t.Fatalf("expected two bound duplicates and choices, got %+v / %v", confirm.Duplicates, confirm.Choices)
	}
	first := confirm.Duplicates[0]
	if first.Name != "Hero" || first.ExistingID != "id-1" || first.ExistingVersion != "rev-1" {
		t.Fatalf("duplicate binding lost review identity: %+v", first)
	}
	if confirm.Choices["Lily"] != "keep" {
		t.Fatalf("choices = %v, want Lily=keep", confirm.Choices)
	}
	if confirm.Choices["Hero"] != "override" {
		t.Fatalf("choices = %v, want Hero=override", confirm.Choices)
	}
}

func TestConfirmedReviewEmptyChoicesStillExplicit(t *testing.T) {
	// A confirmed import without per-entry choices must still carry a binding
	// (with empty choices) so the exchange never falls back to detail-based
	// automatic overrides.
	fields := characterCardImportFields{}
	confirm := fields.confirmedReview()
	if confirm == nil || len(confirm.Duplicates) != 0 || confirm.Choices == nil {
		t.Fatalf("expected an explicit empty confirm binding, got %+v", confirm)
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

func TestReviewExpiredUsesRequestLocale(t *testing.T) {
	tests := map[string]string{
		"zh-CN": "覆盖审阅已失效",
		"en-US": "The override review is no longer valid",
	}
	for locale, want := range tests {
		t.Run(locale, func(t *testing.T) {
			c := app.NewContext(0)
			c.Request.Header.Set("X-Denova-Locale", locale)
			writeErrorKey(c, 400, "api.characterCard.reviewExpired")

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