package handlers

import (
	"encoding/json"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestInteractiveStoryStructureBusyErrorUsesRequestLocale(t *testing.T) {
	tests := map[string]string{
		"zh-CN": "故事正在生成，请在本轮结束后再修改主角或状态结构",
		"en-US": "The story is generating. Change the protagonist or state structure after this turn finishes.",
	}
	for locale, want := range tests {
		t.Run(locale, func(t *testing.T) {
			ctx := app.NewContext(0)
			ctx.Request.Header.Set("X-Denova-Locale", locale)
			writeErrorKey(ctx, 409, "api.interactive.storyStructureBusy")

			var body map[string]string
			if err := json.Unmarshal(ctx.Response.Body(), &body); err != nil {
				t.Fatal(err)
			}
			if body["error"] != want {
				t.Fatalf("localized error = %q, want %q", body["error"], want)
			}
		})
	}
}
