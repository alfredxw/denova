package agents

import "testing"

func TestIsWorkspaceArtifactReadRecognizesEveryArtifactLayout(t *testing.T) {
	tests := []struct {
		name   string
		tool   string
		target string
		want   bool
	}{
		{name: "legacy relative", tool: "read", target: ".denova/artifacts/tool.txt", want: true},
		{name: "writing session", tool: "read", target: "/workspace/.denova/sessions/session.jsonl.artifacts/call.txt", want: true},
		{name: "game branch", tool: "read", target: "/workspace/.denova/artifacts/game/story/branch/call.txt", want: true},
		{name: "windows separators", tool: "read", target: `C:\\workspace\\session.jsonl.artifacts\\call.txt`, want: true},
		{name: "ordinary source", tool: "read", target: "/workspace/chapter.md", want: false},
		{name: "different tool", tool: "web_fetch", target: "/workspace/session.jsonl.artifacts/call.txt", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isWorkspaceArtifactRead(test.tool, test.target); got != test.want {
				t.Fatalf("isWorkspaceArtifactRead(%q, %q) = %t, want %t", test.tool, test.target, got, test.want)
			}
		})
	}
}
