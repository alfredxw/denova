package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agents/attachment"
	"denova/internal/agents/runtime/external"
	"denova/internal/hostruntime"
)

type summaryCapture struct {
	Args  []string
	Input json.RawMessage
}

// The test binary stands in for the CLI so the real launch and stream encoder
// run without account credentials or external model requests.
func TestSummaryCLIProcess(t *testing.T) {
	path := os.Getenv("DENOVA_SUMMARY_CLI_CAPTURE")
	if path == "" {
		return
	}
	body, err := bufio.NewReader(os.Stdin).ReadBytes('\n')
	if err != nil {
		os.Exit(2)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(3)
	}
	if json.NewEncoder(file).Encode(summaryCapture{Args: os.Args, Input: body}) != nil {
		os.Exit(4)
	}
	_ = file.Close()
	fmt.Println(`{"type":"system","subtype":"init","session_id":"summary-only"}`)
	fmt.Println(`{"type":"assistant","message":{"id":"summary","content":[{"type":"text","text":"Map details preserved."}]}}`)
	fmt.Println(`{"type":"result","subtype":"success","session_id":"summary-only"}`)
	os.Exit(0)
}

func TestImageSummaryLaunchesWithoutToolsAndEncodesNativePixels(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(t.TempDir(), "requests.jsonl")
	client := &Client{launch: hostruntime.ClaudeLaunch{Executable: exe, Args: []string{"-test.run=^TestSummaryCLIProcess$", "--"}},
		env: append(os.Environ(), "DENOVA_SUMMARY_CLI_CAPTURE="+capture), active: map[*exec.Cmd]context.CancelFunc{}, version: "fixture"}
	const url = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aKX0AAAAASUVORK5CYII="
	files, err := attachment.Materialize(t.TempDir(), attachment.SessionScope("summary"), "images", []attachment.Upload{{Name: "map.png", DataURL: url}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = (external.HistoryPreparation{Adapter: client, Input: external.Input{
		Selection: config.RuntimeSelection{Kind: config.RuntimeClaude, Claude: &config.ClaudeRuntimeSettings{Model: "claude-sonnet-4-6"}},
		History:   []external.Message{{Role: "user", Text: strings.Repeat("Map reference. ", 10000), Attachments: files, ToolImages: files, Cursor: 1}},
	}}).Prepare(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(capture)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader := bufio.NewScanner(file)
	reader.Buffer(make([]byte, 1024), 1<<20)
	images, calls := 0, 0
	for reader.Scan() {
		calls++
		var captured summaryCapture
		if err := json.Unmarshal(reader.Bytes(), &captured); err != nil {
			t.Fatal(err)
		}
		flags := map[string]string{}
		for i, arg := range captured.Args[:len(captured.Args)-1] {
			flags[arg] = captured.Args[i+1]
		}
		if value, present := flags["--tools"]; !present || value != "" || flags["--allowedTools"] != "mcp__denova__*" || flags["--resume"] != "" || flags["--model"] != "claude-sonnet-4-6" {
			t.Fatalf("summary inherited turn tools or session: %v", captured.Args)
		}
		var input struct {
			Message struct {
				Content []struct {
					Type   string
					Source struct{ Type, MediaType, Data string }
				}
			}
		}
		if err := json.Unmarshal(captured.Input, &input); err != nil {
			t.Fatal(err)
		}
		for _, part := range input.Message.Content {
			if part.Type == "image" {
				if part.Source.Type != "base64" || part.Source.Data != strings.SplitN(url, ",", 2)[1] {
					t.Fatal("native summary pixels changed")
				}
				images++
			}
		}
	}
	if err := reader.Err(); err != nil {
		t.Fatal(err)
	}
	if images != 2 || calls < 2 {
		t.Fatalf("summary requests=%d images=%d", calls, images)
	}
}

func TestInputEstimatorUsesResolvedAPIModel(t *testing.T) {
	client := Client{apiModel: "claude-sonnet-4-6"}
	input := external.Input{Selection: config.RuntimeSelection{Kind: config.RuntimeClaude, Claude: &config.ClaudeRuntimeSettings{ProfileID: "custom-route"}}}
	estimator := client.InputEstimator(input)
	if estimator.ImageTokens == nil || estimator.ImageTokens(256, 256) >= 32*1024 {
		t.Fatal("API route lost its resolved model policy")
	}
	input.Selection.Claude = &config.ClaudeRuntimeSettings{Model: "default"}
	if client.InputEstimator(input).ImageTokens != nil {
		t.Fatal("CLI alias did not use the conservative fallback")
	}
}
