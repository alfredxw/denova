package filejournal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	runstate "github.com/alfredxw/denova/agent/runtime"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	journalGenerationManifestVersion = 1
	journalSnapshotVersion           = 1
)

type journalCheckpointStage string

const (
	checkpointSnapshotDurable  journalCheckpointStage = "snapshot_durable"
	checkpointTailDurable      journalCheckpointStage = "tail_durable"
	checkpointManifestDurable  journalCheckpointStage = "manifest_durable"
	checkpointActivated        journalCheckpointStage = "activated"
	checkpointGarbageCollected journalCheckpointStage = "garbage_collected"
)

type journalGeneration struct {
	Generation       uint64          `json:"generation"`
	CheckpointCursor runstate.Cursor `json:"checkpoint_cursor"`
	SnapshotFile     string          `json:"snapshot_file,omitempty"`
	TailFile         string          `json:"tail_file"`
}

type journalGenerationManifestBody struct {
	Version   int                `json:"version"`
	KeySHA256 string             `json:"key_sha256"`
	Sequence  uint64             `json:"sequence"`
	Active    journalGeneration  `json:"active"`
	Previous  *journalGeneration `json:"previous,omitempty"`
}

type journalGenerationManifest struct {
	journalGenerationManifestBody
	Checksum string `json:"checksum"`
}

type journalSnapshotBody struct {
	Version    int             `json:"version"`
	Generation uint64          `json:"generation"`
	Checkpoint json.RawMessage `json:"checkpoint"`
}

type journalSnapshot struct {
	journalSnapshotBody
	Checksum string `json:"checksum"`
}

// baseGeneration is the manifest-free bootstrap layout used until the first
// checkpoint activates an immutable generation manifest.
func (j *journal) baseGeneration() journalGeneration {
	return journalGeneration{TailFile: filepath.Base(j.path)}
}

func (j *journal) generationSnapshotPath(generation uint64) string {
	return fmt.Sprintf("%s.snapshot.%020d.json", j.path, generation)
}

func (j *journal) generationTailPath(generation uint64) string {
	return fmt.Sprintf("%s.tail.%020d.jsonl", j.path, generation)
}

func (j *journal) generationManifestPath(sequence uint64) string {
	return fmt.Sprintf("%s.generations.%020d.json", j.path, sequence)
}

func (j *journal) resolveGenerationFile(name string) (string, error) {
	if name == "" || filepath.Base(name) != name {
		return "", fmt.Errorf("file journal generation contains an invalid file name %q", name)
	}
	return filepath.Join(filepath.Dir(j.path), name), nil
}

// loadGenerationLayoutLocked treats the newest immutable manifest as the
// canonical activation record. Once a manifest path exists, falling back to an
// older manifest could discard transactions durably appended to the newly
// activated tail, so complete corruption of the newest manifest is fatal.
// The previous generation named by that manifest is recovery material only.
func (j *journal) loadGenerationLayoutLocked() error {
	paths, err := filepath.Glob(j.path + ".generations.*.json")
	if err != nil {
		return fmt.Errorf("list file journal generation manifests: %w", err)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	if len(paths) == 0 {
		base := j.baseGeneration()
		j.generationCandidates = []journalGeneration{base}
		j.activeGeneration = base
		j.manifestSequence = 0
		j.tailPath = j.path
		return nil
	}
	newestPath := paths[0]
	manifest, err := readGenerationManifest(newestPath, j.keySHA256)
	if err != nil {
		return fmt.Errorf("read newest file journal generation manifest %s: %w", filepath.Base(newestPath), err)
	}
	if got, want := filepath.Base(newestPath), filepath.Base(j.generationManifestPath(manifest.Sequence)); got != want {
		return fmt.Errorf("newest file journal generation manifest sequence mismatch: file=%s manifest=%s", got, want)
	}
	candidates := generationPair(manifest.Active, manifest.Previous)
	tailPath, err := j.resolveGenerationFile(manifest.Active.TailFile)
	if err != nil {
		return err
	}
	j.generationCandidates = candidates
	j.activeGeneration = manifest.Active
	j.manifestSequence = manifest.Sequence
	j.tailPath = tailPath
	return nil
}

func generationPair(active journalGeneration, previous *journalGeneration) []journalGeneration {
	result := []journalGeneration{active}
	if previous != nil {
		result = append(result, *previous)
	}
	return result
}

func readGenerationManifest(path, keySHA256 string) (journalGenerationManifestBody, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return journalGenerationManifestBody{}, err
	}
	var manifest journalGenerationManifest
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		return journalGenerationManifestBody{}, err
	}
	bodyJSON, err := json.Marshal(manifest.journalGenerationManifestBody)
	if err != nil {
		return journalGenerationManifestBody{}, err
	}
	digest := sha256.Sum256(bodyJSON)
	if manifest.Version != journalGenerationManifestVersion || manifest.KeySHA256 != keySHA256 ||
		manifest.Sequence == 0 || manifest.Checksum != hex.EncodeToString(digest[:]) {
		return journalGenerationManifestBody{}, fmt.Errorf("generation manifest checksum, version, or identity mismatch")
	}
	if manifest.Active.TailFile == "" {
		return journalGenerationManifestBody{}, fmt.Errorf("generation manifest active tail is missing")
	}
	return manifest.journalGenerationManifestBody, nil
}

func encodeGenerationManifest(body journalGenerationManifestBody) ([]byte, error) {
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(bodyJSON)
	return json.Marshal(journalGenerationManifest{
		journalGenerationManifestBody: body,
		Checksum:                      hex.EncodeToString(digest[:]),
	})
}

func encodeFileJournalSnapshot(generation uint64, checkpoint json.RawMessage) ([]byte, error) {
	body := journalSnapshotBody{Version: journalSnapshotVersion, Generation: generation, Checkpoint: checkpoint}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(bodyJSON)
	return json.Marshal(journalSnapshot{journalSnapshotBody: body, Checksum: hex.EncodeToString(digest[:])})
}

func decodeFileJournalSnapshot(encoded []byte, generation journalGeneration) (json.RawMessage, error) {
	var snapshot journalSnapshot
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return nil, err
	}
	bodyJSON, err := json.Marshal(snapshot.journalSnapshotBody)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(bodyJSON)
	if snapshot.Version != journalSnapshotVersion || snapshot.Generation != generation.Generation ||
		snapshot.Checksum != hex.EncodeToString(digest[:]) || len(snapshot.Checkpoint) == 0 {
		return nil, fmt.Errorf("snapshot checksum, version, generation, or payload is invalid")
	}
	return append(json.RawMessage(nil), snapshot.Checkpoint...), nil
}

func (j *journal) maybeCheckpointLocked(state runstate.JournalCheckpointState) error {
	options := j.options.normalized()
	if state == nil || !state.CheckpointSafe() ||
		(j.tailBytes < options.CheckpointTailBytes && j.tailRecords < options.CheckpointTailRecords) {
		return nil
	}
	if state.Cursor() != j.cursor {
		return fmt.Errorf("checkpoint reducer cursor %d does not match journal cursor %d", state.Cursor(), j.cursor)
	}
	if err := j.ensureCommandRecordsForCheckpointLocked(); err != nil {
		return fmt.Errorf("checkpoint command idempotency fence: %w", err)
	}
	checkpoint, err := state.MarshalCheckpoint()
	if err != nil {
		return err
	}
	generationNumber := j.manifestSequence + 1
	if generationNumber <= j.activeGeneration.Generation {
		generationNumber = j.activeGeneration.Generation + 1
	}
	snapshotPath := j.generationSnapshotPath(generationNumber)
	tailPath := j.generationTailPath(generationNumber)
	generation := journalGeneration{
		Generation: generationNumber, CheckpointCursor: state.Cursor(),
		SnapshotFile: filepath.Base(snapshotPath), TailFile: filepath.Base(tailPath),
	}
	snapshotBytes, err := encodeFileJournalSnapshot(generationNumber, checkpoint)
	if err != nil {
		return fmt.Errorf("encode file journal checkpoint: %w", err)
	}
	if err := writeAtomicFile(snapshotPath, append(snapshotBytes, '\n'), 0o600); err != nil {
		return fmt.Errorf("write file journal checkpoint: %w", err)
	}
	if err := j.runCheckpointHook(checkpointSnapshotDurable); err != nil {
		return err
	}
	if err := writeAtomicFile(tailPath, nil, 0o600); err != nil {
		return fmt.Errorf("create file journal generation tail: %w", err)
	}
	if err := j.runCheckpointHook(checkpointTailDurable); err != nil {
		return err
	}
	sequence := j.manifestSequence + 1
	previous := j.activeGeneration
	manifestBody := journalGenerationManifestBody{
		Version: journalGenerationManifestVersion, KeySHA256: j.keySHA256,
		Sequence: sequence, Active: generation, Previous: &previous,
	}
	manifestBytes, err := encodeGenerationManifest(manifestBody)
	if err != nil {
		return fmt.Errorf("encode file journal generation manifest: %w", err)
	}
	if err := writeAtomicFile(j.generationManifestPath(sequence), append(manifestBytes, '\n'), 0o600); err != nil {
		return fmt.Errorf("commit file journal generation manifest: %w", err)
	}
	j.manifestSequence = sequence
	j.activeGeneration = generation
	j.generationCandidates = []journalGeneration{generation, previous}
	j.tailPath = tailPath
	j.tailBytes, j.tailRecords = 0, 0
	j.needsNewline = false
	j.lastTailHash = ""
	// Every command through the checkpoint cursor is now fenced by its direct
	// checksummed record. Keep only post-checkpoint commands in memory so a
	// long-lived binding cannot grow an unbounded historical command map.
	j.commandIndex = make(map[runstate.CommandID]runstate.CommandRecord)
	j.indexReady = false
	if err := j.runCheckpointHook(checkpointManifestDurable); err != nil {
		return err
	}
	if err := j.runCheckpointHook(checkpointActivated); err != nil {
		return err
	}
	if err := j.garbageCollectGenerationsLocked(generation, previous, sequence); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-runtime] generation garbage collection deferred journal=%s error=%v", filepath.Base(j.path), err))
		return nil
	}
	return j.runCheckpointHook(checkpointGarbageCollected)
}

func (j *journal) MaybeCheckpoint(ctx context.Context, state runstate.JournalCheckpointState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return fmt.Errorf("file journal is closed")
	}
	return j.maybeCheckpointLocked(state)
}

func (j *journal) runCheckpointHook(stage journalCheckpointStage) error {
	if j.checkpointHook == nil {
		return nil
	}
	if err := j.checkpointHook(stage); err != nil {
		return fmt.Errorf("checkpoint interrupted after %s: %w", stage, err)
	}
	return nil
}

func (j *journal) garbageCollectGenerationsLocked(active, previous journalGeneration, sequence uint64) error {
	keep := map[string]struct{}{
		active.SnapshotFile: {}, active.TailFile: {}, previous.SnapshotFile: {}, previous.TailFile: {},
		filepath.Base(j.generationManifestPath(sequence)): {},
	}
	if sequence > 1 {
		keep[filepath.Base(j.generationManifestPath(sequence-1))] = struct{}{}
	}
	patterns := []string{j.path + ".snapshot.*.json", j.path + ".tail.*.jsonl", j.path + ".generations.*.json"}
	var joined error
	for _, pattern := range patterns {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		for _, path := range paths {
			if _, retained := keep[filepath.Base(path)]; retained {
				continue
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				joined = errors.Join(joined, err)
			}
		}
	}
	if previous.Generation != 0 && strings.TrimSpace(previous.TailFile) != filepath.Base(j.path) {
		// The bootstrap tail is no longer recovery material after two successful
		// generations. Remove it only after the new manifest is durable.
		if err := os.Remove(j.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			joined = errors.Join(joined, err)
		}
	}
	if joined != nil {
		return joined
	}
	return syncDirectory(filepath.Dir(j.path))
}

// Implemented by the command-record sidecar. It is intentionally called before
// a generation manifest can make old segments garbage-collectable.
func (j *journal) ensureCommandRecordsForCheckpointLocked() error {
	return j.persistCommandRecordsLocked(j.commandIndex)
}
