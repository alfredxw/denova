package external

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/internal/agents/attachment"
	agentrun "denova/internal/agents/run"
	agent "github.com/alfredxw/denova/agent"
)

func TestExternalAttachmentsSurviveHistoryAndDomainReadImages(t *testing.T) {
	service, request, workspace := operationFixture(t)
	const dataURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aKX0AAAAASUVORK5CYII="
	files, err := attachment.Materialize(request.AttachmentRoot, attachment.SessionScope(request.Session.ID), request.CommandID, []attachment.Upload{{Name: "reference.png", DataURL: dataURL}})
	if err != nil {
		t.Fatal(err)
	}
	request.Input.Attachments, request.Message.Attachments = files, files
	imageBytes, _ := base64.StdEncoding.DecodeString(strings.SplitN(dataURL, ",", 2)[1])
	if err := os.WriteFile(filepath.Join(workspace, "result.png"), imageBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	request.Adapter = adapterFunc(func(ctx context.Context, input Input, host Host) (Result, error) {
		if !strings.Contains(input.Text, "immutable input copies") || len(input.Attachments) != 1 || input.Attachments[0].RuntimePath != files[0].RuntimePath {
			return Result{}, fmt.Errorf("attachment contract was not projected")
		}
		if got, err := agent.AttachmentDataURL(input.Attachments[0]); err != nil || got != dataURL {
			return Result{}, fmt.Errorf("user image bytes changed: %v", err)
		}
		result, err := host.CallTool(ctx, ToolCall{ID: "read-image", Name: "read", Arguments: json.RawMessage(`{"path":"result.png"}`)})
		if err != nil || !result.Success || len(result.Images) != 1 {
			return Result{}, fmt.Errorf("domain image read lost its image: %#v, %v", result, err)
		}
		if got, err := agent.AttachmentDataURL(result.Images[0]); err != nil || got != dataURL {
			return Result{}, fmt.Errorf("tool image bytes changed: %v", err)
		}
		return Result{Text: "I inspected both images."}, nil
	})
	op, err := service.Start(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := op.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("image operation: %#v, error: %v", outcome, outcome.Error)
	}
	history, err := PrepareHistory(t.Context(), request.Session)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := history.Messages(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	userImages, toolImages := 0, 0
	for _, message := range messages {
		userImages += len(message.Attachments)
		toolImages += len(message.ToolImages)
		for _, file := range append(append([]agent.Attachment(nil), message.Attachments...), message.ToolImages...) {
			if file.RuntimePath != "" || filepath.IsAbs(file.Path) {
				t.Fatalf("durable attachment was not portable: %+v", file)
			}
		}
	}
	if userImages != 1 || toolImages != 1 {
		t.Fatalf("history omitted images: user=%d tool=%d", userImages, toolImages)
	}
	projected, err := op.projectMedia(t.Context(), Input{History: messages})
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range projected.History {
		for _, file := range append(append([]agent.Attachment(nil), message.Attachments...), message.ToolImages...) {
			if _, err := agent.AttachmentDataURL(file); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.Chmod(files[0].RuntimePath, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files[0].RuntimePath, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := agent.AttachmentDataURL(projected.History[0].Attachments[0]); err == nil {
		t.Fatal("changed immutable input was accepted")
	}
}
