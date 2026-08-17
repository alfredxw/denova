package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostEffectObligationIsExactDurableAndIndependentOfTaskCatalog(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	workspace := filepath.Join(root, "workspace")
	store := NewStore(userDir, workspace)
	effect := HostEffectObligation{
		ID: "host-effect-one", Kind: "tool_mutation_committed", Workspace: workspace,
		Payload: json.RawMessage(`{"version":1,"mutation":{"target":"chapters/one.md"}}`),
	}
	admitted, err := store.AdmitHostEffect(context.Background(), effect)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(userDir, workspace).AdmitHostEffect(context.Background(), effect); err != nil {
		t.Fatalf("exact replay: %v", err)
	}
	conflict := effect
	conflict.Payload = json.RawMessage(`{"version":1,"mutation":{"target":"chapters/two.md"}}`)
	if _, err := store.AdmitHostEffect(context.Background(), conflict); !errors.Is(err, ErrHostEffectConflict) {
		t.Fatalf("conflicting replay error = %v", err)
	}

	obligations, err := NewStore(userDir, workspace).ListHostEffects()
	if err != nil || len(obligations) != 1 || obligations[0].IntentHash != admitted.IntentHash {
		t.Fatalf("durable obligations = %#v err=%v", obligations, err)
	}
	if err := NewStore(userDir, workspace).AcknowledgeHostEffect(context.Background(), admitted); err != nil {
		t.Fatal(err)
	}
	obligations, err = store.ListHostEffects()
	if err != nil || len(obligations) != 0 {
		t.Fatalf("acknowledged obligations = %#v err=%v", obligations, err)
	}
}

func TestHostEffectAdmissionSerializesConflictingProcesses(t *testing.T) {
	root := t.TempDir()
	type process struct {
		command *exec.Cmd
		output  bytes.Buffer
	}
	processes := make([]process, 0, 2)
	for _, target := range []string{"chapters/one.md", "chapters/two.md"} {
		command := exec.Command(os.Args[0], "-test.run=^TestHostEffectAdmissionSubprocess$")
		command.Env = append(os.Environ(),
			"DENOVA_HOST_EFFECT_ADMISSION_HELPER=1",
			"DENOVA_HOST_EFFECT_ADMISSION_ROOT="+root,
			"DENOVA_HOST_EFFECT_ADMISSION_TARGET="+target,
		)
		processes = append(processes, process{command: command})
		current := &processes[len(processes)-1]
		current.command.Stdout = &current.output
		current.command.Stderr = &current.output
		if err := current.command.Start(); err != nil {
			t.Fatal(err)
		}
	}
	succeeded := 0
	conflicted := 0
	for index := range processes {
		err := processes[index].command.Wait()
		output := processes[index].output.String()
		switch {
		case err == nil:
			succeeded++
		case strings.Contains(output, ErrHostEffectConflict.Error()):
			conflicted++
		default:
			t.Fatalf("host-effect helper %d failed unexpectedly: err=%v output=%s", index, err, output)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("cross-process admission succeeded=%d conflicted=%d, want one exact winner", succeeded, conflicted)
	}
	obligations, err := NewStore(root, "").ListHostEffects()
	if err != nil || len(obligations) != 1 {
		t.Fatalf("cross-process obligation = %#v err=%v", obligations, err)
	}
}

func TestHostEffectAdmissionSubprocess(t *testing.T) {
	if os.Getenv("DENOVA_HOST_EFFECT_ADMISSION_HELPER") != "1" {
		return
	}
	root := os.Getenv("DENOVA_HOST_EFFECT_ADMISSION_ROOT")
	target := os.Getenv("DENOVA_HOST_EFFECT_ADMISSION_TARGET")
	payload, err := json.Marshal(map[string]any{"version": 1, "target": target})
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewStore(root, "").AdmitHostEffect(context.Background(), HostEffectObligation{
		ID: "cross-process-effect", Kind: "tool_mutation_committed",
		Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
}
