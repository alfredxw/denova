package update

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "same with v prefix", a: "v0.1.10", b: "0.1.10", want: 0},
		{name: "patch newer", a: "0.1.9", b: "0.1.10", want: -1},
		{name: "minor newer", a: "0.2.0", b: "0.1.99", want: 1},
		{name: "missing patch equals zero", a: "1.0", b: "1.0.0", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareVersions(tt.a, tt.b)
			switch {
			case tt.want == 0 && got != 0:
				t.Fatalf("compareVersions(%q,%q)=%d want 0", tt.a, tt.b, got)
			case tt.want < 0 && got >= 0:
				t.Fatalf("compareVersions(%q,%q)=%d want <0", tt.a, tt.b, got)
			case tt.want > 0 && got <= 0:
				t.Fatalf("compareVersions(%q,%q)=%d want >0", tt.a, tt.b, got)
			}
		})
	}
}

func TestDevVersion(t *testing.T) {
	for _, v := range []string{"", "dev", "development", "0.1.10-dirty"} {
		if !isDevVersion(v) {
			t.Fatalf("%q should be treated as dev", v)
		}
	}
	if isDevVersion("0.1.10") {
		t.Fatalf("release version should not be dev")
	}
}
