package toolapproval

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"denova/config"
)

func TestEvaluateBashAllowsStaticWorkspaceTextAnalysis(t *testing.T) {
	t.Parallel()
	workspace := filepath.Join(t.TempDir(), ".nova", "示例书")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	chapter := "chapters/v00001-第一卷-青云宗篇/ch00001-第一章-穿越与觉醒.md"
	pythonCommand := fmt.Sprintf(
		`cd %q && f=%q && echo "破折号:" && grep -c "——" "$f"; echo "不是...是...:" && grep -c "不是[^。]*是" "$f"; echo "markdown头:"; grep -cE "^#|^\*\*|^- |^>" "$f"; echo "字数:"; python3 -c "
import re
s=open('$f',encoding='utf-8').read()
s=s.replace('\n','')
print('non-newline chars:',len(s))
print('CJK:',len(re.findall(r'[\u4e00-\u9fff]',s)))
"`,
		workspace, chapter,
	)

	tests := []struct {
		name     string
		command  string
		modes    []config.AgentApprovalMode
		wantRisk Risk
	}{
		{
			name: "awk character count",
			command: fmt.Sprintf(
				`cd %q && awk '{gsub(/[[:space:]]/,""); n+=length($0)} END{print n}' %q`,
				workspace, chapter,
			),
			modes:    []config.AgentApprovalMode{config.AgentApprovalAsk, config.AgentApprovalWrite},
			wantRisk: RiskLow,
		},
		{
			name:     "literal variable reads",
			command:  pythonCommand,
			modes:    []config.AgentApprovalMode{config.AgentApprovalWrite},
			wantRisk: RiskMedium,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			for _, mode := range test.modes {
				decision := evaluateShellCommand(t, workspace, "bash", mode, test.command)
				if decision.Action != ActionAllow || decision.Risk != test.wantRisk {
					t.Fatalf("mode=%s decision=%#v, want allow/%s", mode, decision, test.wantRisk)
				}
			}
		})
	}

	pythonDecision := evaluateShellCommand(t, workspace, "bash", config.AgentApprovalAsk, pythonCommand)
	if pythonDecision.Action != ActionPrompt {
		t.Fatalf("Ask-mode inline Python decision=%#v, want prompt", pythonDecision)
	}
}

func TestEvaluateBashStaticAnalysisFailsClosed(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	tests := map[string]string{
		"awk process execution":         `awk 'BEGIN { system("touch escaped") }' README.md`,
		"awk hash regex before process": `awk '/^#/ { system("touch escaped") }' README.md`,
		"awk external input":            `awk '{ print length($0) }' /etc/passwd`,
		"awk program file":              `awk -f stats.awk README.md`,
		"awk dynamic input":             `awk 'BEGIN { getline line < "/etc/passwd"; print line }'`,
		"awk output redirection":        `awk '{ print > "output.txt" }' README.md`,
		"command substitution":          `f=$(printf README.md); cat "$f"`,
		"conditional assignment":        `grep -q needle README.md && f=/etc/passwd; cat "$f"`,
		"pipeline assignment":           `f=README.md | cat; cat "$f"`,
		"external assigned path":        `f=/etc/passwd; cat "$f"`,
		"external working directory":    `cd /tmp && cat result.txt`,
		"printf variable assignment":    `printf -v path /etc/passwd; cat "$path"`,
		"printf percent n":              `printf '%n' PATH; cat README.md`,
		"unresolved inherited variable": `cat "$UNTRUSTED_PATH"`,
	}

	for name, command := range tests {
		command := command
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			decision := evaluateShellCommand(t, workspace, "bash", config.AgentApprovalWrite, command)
			if decision.Action != ActionPrompt {
				t.Fatalf("decision=%#v, want prompt", decision)
			}
		})
	}
}

func TestEvaluateBashTreatsInlinePythonAsCodeExecution(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	command := `python3 -c '__import__("json").dumps({"value": 1})'`

	writeDecision := evaluateShellCommand(t, workspace, "bash", config.AgentApprovalWrite, command)
	if writeDecision.Action != ActionAllow || writeDecision.Risk != RiskMedium {
		t.Fatalf("Write-mode decision=%#v, want allow/%s", writeDecision, RiskMedium)
	}

	askDecision := evaluateShellCommand(t, workspace, "bash", config.AgentApprovalAsk, command)
	if askDecision.Action != ActionPrompt {
		t.Fatalf("Ask-mode decision=%#v, want prompt", askDecision)
	}
}

func TestEvaluateBashRejectsWorkingDirectorySymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup differs on Windows")
	}
	t.Parallel()
	workspace := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(workspace, "outside")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	decision := evaluateShellCommand(t, workspace, "bash", config.AgentApprovalWrite, `cd outside && cat result.txt`)
	if decision.Action != ActionPrompt {
		t.Fatalf("decision=%#v, want prompt", decision)
	}
}
