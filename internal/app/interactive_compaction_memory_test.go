package app

import (
	"testing"

	"denova/internal/interactive"
)

func TestInteractiveCompactionCoveredTurns(t *testing.T) {
	turns := []interactive.TurnEvent{
		{ID: "t1"}, {ID: "t2"}, {ID: "t3"}, {ID: "t4"},
	}
	tests := []struct {
		name       string
		compaction *interactive.ContextCompactionEvent
		want       []string
	}{
		{
			// 首次压缩:没有前序检查点,整段历史都归本次覆盖。
			name:       "first compaction covers everything",
			compaction: nil,
			want:       []string{"t1", "t2", "t3", "t4"},
		},
		{
			// 增量压缩:只覆盖上次截止位置之后的回合,不重复补抽。
			name:       "incremental compaction resumes from the checkpoint",
			compaction: &interactive.ContextCompactionEvent{Summary: "前情", SourceTurnCount: 2},
			want:       []string{"t3", "t4"},
		},
		{
			// 空摘要的压缩事件不算有效检查点,与首次压缩同义。
			name:       "empty summary is not a checkpoint",
			compaction: &interactive.ContextCompactionEvent{SourceTurnCount: 2},
			want:       []string{"t1", "t2", "t3", "t4"},
		},
		{
			// 计数越界不得 panic:分支回滚后 SourceTurnCount 可能超过现有回合数。
			name:       "out-of-range count is clamped",
			compaction: &interactive.ContextCompactionEvent{Summary: "前情", SourceTurnCount: 99},
			want:       nil,
		},
		{
			name:       "negative count is clamped",
			compaction: &interactive.ContextCompactionEvent{Summary: "前情", SourceTurnCount: -5},
			want:       []string{"t1", "t2", "t3", "t4"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := interactiveCompactionCoveredTurns(turns, tc.compaction)
			if len(got) != len(tc.want) {
				t.Fatalf("covered %d turns, want %d: %#v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i].ID != tc.want[i] {
					t.Fatalf("covered[%d] = %q, want %q", i, got[i].ID, tc.want[i])
				}
			}
		})
	}
}

// TestInteractiveCompactionCoveredTurnsMatchesSourceRange 钉住两者同口径:
// 补抽的回合必须正好是被摘要吃掉的那些,否则会漏抽或重复抽。
func TestInteractiveCompactionCoveredTurnsMatchesSourceRange(t *testing.T) {
	turns := []interactive.TurnEvent{
		{ID: "t1", User: "a", Narrative: "A"},
		{ID: "t2", User: "b", Narrative: "B"},
		{ID: "t3", User: "c", Narrative: "C"},
	}
	compaction := &interactive.ContextCompactionEvent{Summary: "前情", SourceTurnCount: 1}

	messages, _ := interactiveCompactionSource(turns, compaction)
	covered := interactiveCompactionCoveredTurns(turns, compaction)

	// interactiveCompactionSource 每个回合产出 user + assistant 两条消息。
	if len(messages) != len(covered)*2 {
		t.Fatalf("source produced %d messages for %d covered turns", len(messages), len(covered))
	}
}
