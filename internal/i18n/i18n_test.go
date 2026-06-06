package i18n

import "testing"

func TestResolveLocale(t *testing.T) {
	cases := map[string]string{
		"":      LocaleZH,
		"auto":  LocaleZH,
		"zh":    LocaleZH,
		"zh-CN": LocaleZH,
		"en":    LocaleEN,
		"en-US": LocaleEN,
		"fr-FR": LocaleZH,
	}
	for input, want := range cases {
		if got := Resolve(input); got != want {
			t.Fatalf("Resolve(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLocalizerTranslatesAndInterpolates(t *testing.T) {
	loc := New(LocaleEN)
	got := loc.T("api.workspace.switched", "workspace", "/tmp/book")
	if got != "Switched to: /tmp/book" {
		t.Fatalf("unexpected translation: %q", got)
	}
	if fallback := loc.T("missing.key"); fallback != "missing.key" {
		t.Fatalf("missing key fallback: %q", fallback)
	}
}
