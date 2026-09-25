package external

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/internal/agents/attachment"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agent "github.com/alfredxw/denova/agent"
)

func TestWritingImageCheckpointSurvivesMovedStoreAndColdReopen(t *testing.T) {
	service, request, _ := operationFixture(t)
	file := checkpointImage(t, png.NoCompression)
	url, err := agent.AttachmentDataURL(file)
	if err != nil {
		t.Fatal(err)
	}
	files, err := attachment.Materialize(request.AttachmentRoot, attachment.SessionScope(request.Session.ID), "references", []attachment.Upload{{Name: "reference.png", DataURL: url}})
	if err != nil {
		t.Fatal(err)
	}
	large := strings.Repeat("Preserve the original layout. ", 5000)
	if err := request.Session.Append(agent.UserMessageWithAttachments(large, files)); err != nil {
		t.Fatal(err)
	}
	if err := request.Session.Append(agent.UserMessageWithAttachments("Recent reference", files)); err != nil {
		t.Fatal(err)
	}
	maintenance, images, turns := 0, 0, 0
	request.Adapter = adapterFunc(func(ctx context.Context, input Input, _ Host) (Result, error) {
		if input.Mode == OperationSummarize {
			maintenance++
			for _, message := range input.History {
				for _, file := range message.Attachments {
					got, err := agent.AttachmentDataURL(file)
					if err != nil || got != url {
						t.Fatalf("summary lost original pixels: %v", err)
					}
					images++
				}
			}
			return Result{Text: "Reference: transparent canvas; retain the layout."}, nil
		}
		turns++
		retained := 0
		for _, message := range input.History {
			retained += len(message.Attachments)
			for _, file := range message.Attachments {
				if got, err := agent.AttachmentDataURL(file); err != nil || got != url {
					t.Fatalf("recent image was not restored: %v", err)
				}
			}
		}
		if retained != 1 || !strings.Contains(input.History[0].Text, "transparent canvas") {
			t.Fatal("continuation lost the summary or recent original image")
		}
		return Result{Text: "Continued."}, nil
	})
	for attempt := range 2 {
		history, err := PrepareHistory(t.Context(), request.Session)
		if err != nil {
			t.Fatal(err)
		}
		request.PreparedCursor, request.Checkpoint, request.LoadHistory = history.Cursor, history.Checkpoint, history.Messages
		if attempt == 1 {
			if history.Checkpoint == nil || history.Checkpoint.Version != checkpointVersion {
				t.Fatal("canonical checkpoint was not restored")
			}
			request.CommandID, request.Fingerprint, request.Metadata.MessageID = "second", "second", "second-input"
		}
		op, err := service.Start(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		if outcome := op.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
			t.Fatalf("continuation failed: %+v", outcome)
		}
		if attempt == 0 {
			// Copy the settled store and discard its index: neither process-local
			// provider state nor a derived index is required for recovery.
			moved := t.TempDir()
			if err := os.CopyFS(moved, os.DirFS(request.AttachmentRoot)); err != nil {
				t.Fatal(err)
			}
			indexes, err := filepath.Glob(filepath.Join(moved, "*.idx.json"))
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range indexes {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			reopened, err := session.NewStore(moved)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			request.Session, err = reopened.Get(request.Session.ID)
			if err != nil {
				t.Fatal(err)
			}
			request.AttachmentRoot, service = moved, &Service{}
		}
	}
	if maintenance == 0 || images != 1 || turns != 2 {
		t.Fatalf("cold reopen repeated/lost the image summary: maintenance=%d images=%d turns=%d", maintenance, images, turns)
	}
	canonical, err := request.Session.ReadCanonicalMessages(t.Context())
	if err != nil || canonical[0].Content != large || canonical[0].Attachments[0].RuntimePath != "" {
		t.Fatalf("canonical original changed: %v", err)
	}
}

func TestAlignedRuntimeResumeDoesNotReinspectHistoricalImages(t *testing.T) {
	file := checkpointImage(t, png.BestCompression)
	turns, loads := 0, 0
	adapter := adapterFunc(func(_ context.Context, input Input, _ Host) (Result, error) {
		turns++
		if input.Mode == OperationSummarize || (turns == 2 && (input.SessionID == "" || len(input.History) != 0)) {
			t.Fatal("aligned resume reconstructed history")
		}
		return Result{SessionID: "provider-session", Settled: true}, nil
	})
	runtime := Runtime{CacheRoot: t.TempDir(), Acquire: func(context.Context) (Adapter, func(), error) { return adapter, func() {}, nil }}
	request := SessionRequest{Key: "writing/image", Boundary: "same",
		Prepare: func(ctx context.Context, input Input, adapter Adapter) (Input, error) {
			return (HistoryPreparation{Input: input, Adapter: adapter, LoadHistory: func(context.Context) ([]Message, error) {
				loads++
				return []Message{{Role: "user", Attachments: []agent.Attachment{file}, Cursor: 1}}, nil
			}}).Prepare(ctx)
		}}
	first, err := runtime.Run(t.Context(), request, maintenanceHost{})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Session.Accept(t.Context(), "same"); err != nil {
		t.Fatal(err)
	}
	_ = first.Session.Close()
	if err := os.Remove(file.RuntimePath); err != nil {
		t.Fatal(err)
	}
	second, err := runtime.Run(t.Context(), request, maintenanceHost{})
	if err != nil {
		t.Fatal(err)
	}
	_ = second.Session.Close()
	if turns != 2 || loads != 1 {
		t.Fatalf("unexpected provider requests=%d history loads=%d", turns, loads)
	}
}

func TestManualCompactionResolvesHistoryImagesBeforeSummary(t *testing.T) {
	_, request, _ := operationFixture(t)
	file := checkpointImage(t, png.NoCompression)
	url, err := agent.AttachmentDataURL(file)
	if err != nil {
		t.Fatal(err)
	}
	files, err := attachment.Materialize(request.AttachmentRoot, attachment.SessionScope(request.Session.ID), "manual", []attachment.Upload{{Name: "reference.png", DataURL: url}})
	if err != nil {
		t.Fatal(err)
	}
	files[0].RuntimePath = ""
	loads := 0
	request.LoadHistory = func(context.Context) ([]Message, error) {
		loads++
		return []Message{{Role: "user", Text: strings.Repeat("Old context. ", 10000), Attachments: files, Cursor: 1}}, nil
	}
	images, compactions := 0, 0
	adapter := adapterFunc(func(ctx context.Context, input Input, host Host) (Result, error) {
		if input.Mode == OperationSummarize {
			for _, message := range input.History {
				for _, file := range message.Attachments {
					if got, err := agent.AttachmentDataURL(file); err != nil || got != url {
						t.Fatalf("manual summary lost pixels: %v", err)
					}
					images++
				}
			}
			return Result{Text: "Reference image details."}, nil
		}
		if input.Mode != OperationCompact {
			t.Fatal("manual compaction changed operation mode")
		}
		if compactions == 0 && (len(input.History) == 0 || !strings.Contains(input.History[0].Text, "Reference image details")) {
			t.Fatal("manual compaction lost reconstructed image context")
		}
		if compactions == 1 && (input.SessionID != "compacted" || len(input.History) != 0) {
			t.Fatal("aligned manual compaction reconstructed history")
		}
		compactions++
		return Result{SessionID: "compacted", Settled: true}, host.Emit(agentrun.Event{Type: "context_compaction", Data: map[string]any{"status": "completed"}})
	})
	request.Runtime = &Runtime{CacheRoot: t.TempDir(), Acquire: func(context.Context) (Adapter, func(), error) { return adapter, func() {}, nil }}
	for attempt := range 2 {
		result, err := CompactSession(t.Context(), request, func(ctx context.Context, preparation HistoryPreparation) (Input, error) {
			return preparation.Prepare(ctx)
		})
		if err != nil || !result.Triggered || images != 1 || compactions != attempt+1 || loads != 1 {
			t.Fatalf("manual compaction: %+v, images=%d compactions=%d loads=%d err=%v", result, images, compactions, loads, err)
		}
	}
}
