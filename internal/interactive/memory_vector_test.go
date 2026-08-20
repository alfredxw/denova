package interactive

import (
	"math"
	"path/filepath"
	"testing"
)

func TestMemoryVectorRoundTrip(t *testing.T) {
	vector := []float32{0.5, -1.25, 0, 3.75}
	decoded, err := decodeMemoryVector(encodeMemoryVector(vector), len(vector))
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(decoded) != len(vector) {
		t.Fatalf("dim mismatch: got %d want %d", len(decoded), len(vector))
	}
	for i := range vector {
		if decoded[i] != vector[i] {
			t.Fatalf("value %d mismatch: got %v want %v", i, decoded[i], vector[i])
		}
	}
}

func TestDecodeMemoryVectorRejectsDimMismatch(t *testing.T) {
	if _, err := decodeMemoryVector(encodeMemoryVector([]float32{1, 2}), 3); err == nil {
		t.Fatal("expected dim mismatch error")
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b []float32
		want float64
	}{
		{name: "identical", a: []float32{1, 0}, b: []float32{1, 0}, want: 1},
		{name: "orthogonal", a: []float32{1, 0}, b: []float32{0, 1}, want: 0},
		{name: "opposite", a: []float32{1, 0}, b: []float32{-1, 0}, want: -1},
		{name: "scaled is direction only", a: []float32{1, 1}, b: []float32{5, 5}, want: 1},
		// 维度不符与零向量都返回 0,让候选自然落到向量排名之外而不是污染排序。
		{name: "dim mismatch", a: []float32{1, 0}, b: []float32{1, 0, 0}, want: 0},
		{name: "zero vector", a: []float32{0, 0}, b: []float32{1, 1}, want: 0},
		{name: "empty", a: nil, b: nil, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cosineSimilarity(tc.a, tc.b); math.Abs(got-tc.want) > 1e-6 {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestAppendAndReadMemoryVectors(t *testing.T) {
	store := &Store{root: t.TempDir()}
	storyID := "s1"

	if err := store.AppendMemoryVectors(storyID, "embed-v1", map[string][]float32{
		"r1": {1, 0},
		"r2": {0, 1},
	}); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	// 重写同一 record:读取时后写胜出,让重新抽取天然幂等。
	if err := store.AppendMemoryVectors(storyID, "embed-v1", map[string][]float32{
		"r1": {0, 2},
	}); err != nil {
		t.Fatalf("re-append failed: %v", err)
	}

	vectors := store.readMemoryVectorsLocked(storyID, "embed-v1")
	if len(vectors) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vectors))
	}
	if got := vectors["r1"]; len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Fatalf("later write should win, got %v", got)
	}

	// 换模型后旧向量整批不可见:语义空间不同,混用会得到无意义的距离。
	if other := store.readMemoryVectorsLocked(storyID, "embed-v2"); len(other) != 0 {
		t.Fatalf("expected no vectors for other model, got %d", len(other))
	}
	if none := store.readMemoryVectorsLocked(storyID, ""); len(none) != 0 {
		t.Fatalf("empty model must yield no vectors, got %d", len(none))
	}
}

func TestReadMemoryVectorsMissingSidecar(t *testing.T) {
	store := &Store{root: t.TempDir()}
	// 侧车缺失是正常状态(从未抽取过向量),不该报错也不该 panic。
	if vectors := store.readMemoryVectorsLocked("missing", "embed-v1"); len(vectors) != 0 {
		t.Fatalf("expected no vectors, got %d", len(vectors))
	}
	if _, err := filepath.Abs(store.memoryVectorPath("missing")); err != nil {
		t.Fatalf("vector path must be resolvable: %v", err)
	}
}

func TestMemoryVectorText(t *testing.T) {
	got := MemoryVectorText(NarrativeMemoryRecord{
		Subject: " 林洛 ",
		Object:  "青铜钥匙",
		Text:    "把钥匙藏进井底",
	})
	if want := "林洛 | 青铜钥匙 | 把钥匙藏进井底"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	// 缺失字段不该留下空段,否则同一记录在不同回合会嵌出不同文本。
	if got := MemoryVectorText(NarrativeMemoryRecord{Subject: "林洛", Text: "独自离开"}); got != "林洛 | 独自离开" {
		t.Fatalf("unexpected text %q", got)
	}
}
