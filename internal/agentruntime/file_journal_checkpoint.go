package agentruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	fileJournalGenerationManifestVersion = 1
	fileJournalSnapshotVersion           = 1
)

type fileJournalCheckpointStage string

const (
	checkpointSnapshotDurable  fileJournalCheckpointStage = "snapshot_durable"
	checkpointTailDurable      fileJournalCheckpointStage = "tail_durable"
	checkpointManifestDurable  fileJournalCheckpointStage = "manifest_durable"
	checkpointActivated        fileJournalCheckpointStage = "activated"
	checkpointGarbageCollected fileJournalCheckpointStage = "garbage_collected"
)

type fileJournalGeneration struct {
	Generation       uint64 `json:"generation"`
	CheckpointCursor Cursor `json:"checkpoint_cursor"`
	SnapshotFile     string `json:"snapshot_file,omitempty"`
	TailFile         string `json:"tail_file"`
}

type fileJournalGenerationManifestBody struct {
	Version   int                    `json:"version"`
	KeySHA256 string                 `json:"key_sha256"`
	Sequence  uint64                 `json:"sequence"`
	Active    fileJournalGeneration  `json:"active"`
	Previous  *fileJournalGeneration `json:"previous,omitempty"`
}

type fileJournalGenerationManifest struct {
	fileJournalGenerationManifestBody
	Checksum string `json:"checksum"`
}

type fileJournalSnapshotBody struct {
	Version    int               `json:"version"`
	Generation uint64            `json:"generation"`
	Checkpoint harnessCheckpoint `json:"checkpoint"`
}

type fileJournalSnapshot struct {
	fileJournalSnapshotBody
	Checksum string `json:"checksum"`
}

func (j *fileJournal) legacyGeneration() fileJournalGeneration {
	return fileJournalGeneration{TailFile: filepath.Base(j.path)}
}

func (j *fileJournal) generationSnapshotPath(generation uint64) string {
	return fmt.Sprintf("%s.snapshot.%020d.json", j.path, generation)
}

func (j *fileJournal) generationTailPath(generation uint64) string {
	return fmt.Sprintf("%s.tail.%020d.jsonl", j.path, generation)
}

func (j *fileJournal) generationManifestPath(sequence uint64) string {
	return fmt.Sprintf("%s.generations.%020d.json", j.path, sequence)
}

func (j *fileJournal) resolveGenerationFile(name string) (string, error) {
	if name == "" || filepath.Base(name) != name {
		return "", fmt.Errorf("file journal generation contains an invalid file name %q", name)
	}
	return filepath.Join(filepath.Dir(j.path), name), nil
}

// loadGenerationLayoutLocked scans immutable checksummed manifests newest
// first. A corrupt newest manifest is ignored in favor of the preceding one;
// the selected manifest itself retains both active and previous generations.
func (j *fileJournal) loadGenerationLayoutLocked() error {
	paths, err := filepath.Glob(j.path + ".generations.*.json")
	if err != nil {
		return fmt.Errorf("list file journal generation manifests: %w", err)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	candidates := make([]fileJournalGeneration, 0, 3)
	seen := make(map[string]struct{})
	var sequence uint64
	validManifests := 0
	for _, path := range paths {
		manifest, err := readGenerationManifest(path, j.keySHA256)
		if err != nil {
			log.Printf("[agent-runtime] ignoring invalid generation manifest file=%s error=%v", filepath.Base(path), err)
			continue
		}
		validManifests++
		if manifest.Sequence > sequence {
			sequence = manifest.Sequence
		}
		for _, generation := range generationPair(manifest.Active, manifest.Previous) {
			identity := fmt.Sprintf("%d|%s|%s", generation.Generation, generation.SnapshotFile, generation.TailFile)
			if _, duplicate := seen[identity]; duplicate {
				continue
			}
			seen[identity] = struct{}{}
			candidates = append(candidates, generation)
		}
	}
	if len(paths) > 0 && validManifests == 0 {
		return fmt.Errorf("all file journal generation manifests are invalid")
	}
	if len(candidates) == 0 {
		candidates = append(candidates, j.legacyGeneration())
	}
	tailPath, err := j.resolveGenerationFile(candidates[0].TailFile)
	if err != nil {
		return err
	}
	j.generationCandidates = candidates
	j.activeGeneration = candidates[0]
	j.manifestSequence = sequence
	j.tailPath = tailPath
	return nil
}

func generationPair(active fileJournalGeneration, previous *fileJournalGeneration) []fileJournalGeneration {
	result := []fileJournalGeneration{active}
	if previous != nil {
		result = append(result, *previous)
	}
	return result
}

func readGenerationManifest(path, keySHA256 string) (fileJournalGenerationManifestBody, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return fileJournalGenerationManifestBody{}, err
	}
	var manifest fileJournalGenerationManifest
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		return fileJournalGenerationManifestBody{}, err
	}
	bodyJSON, err := json.Marshal(manifest.fileJournalGenerationManifestBody)
	if err != nil {
		return fileJournalGenerationManifestBody{}, err
	}
	digest := sha256.Sum256(bodyJSON)
	if manifest.Version != fileJournalGenerationManifestVersion || manifest.KeySHA256 != keySHA256 ||
		manifest.Sequence == 0 || manifest.Checksum != hex.EncodeToString(digest[:]) {
		return fileJournalGenerationManifestBody{}, fmt.Errorf("generation manifest checksum, version, or identity mismatch")
	}
	if manifest.Active.TailFile == "" {
		return fileJournalGenerationManifestBody{}, fmt.Errorf("generation manifest active tail is missing")
	}
	return manifest.fileJournalGenerationManifestBody, nil
}

func encodeGenerationManifest(body fileJournalGenerationManifestBody) ([]byte, error) {
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(bodyJSON)
	return json.Marshal(fileJournalGenerationManifest{
		fileJournalGenerationManifestBody: body,
		Checksum:                          hex.EncodeToString(digest[:]),
	})
}

func encodeFileJournalSnapshot(generation uint64, checkpoint harnessCheckpoint) ([]byte, error) {
	body := fileJournalSnapshotBody{Version: fileJournalSnapshotVersion, Generation: generation, Checkpoint: checkpoint}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(bodyJSON)
	return json.Marshal(fileJournalSnapshot{fileJournalSnapshotBody: body, Checksum: hex.EncodeToString(digest[:])})
}

func decodeFileJournalSnapshot(encoded []byte, generation fileJournalGeneration) (harnessCheckpoint, error) {
	var snapshot fileJournalSnapshot
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return harnessCheckpoint{}, err
	}
	bodyJSON, err := json.Marshal(snapshot.fileJournalSnapshotBody)
	if err != nil {
		return harnessCheckpoint{}, err
	}
	digest := sha256.Sum256(bodyJSON)
	if snapshot.Version != fileJournalSnapshotVersion || snapshot.Generation != generation.Generation ||
		snapshot.Checksum != hex.EncodeToString(digest[:]) || snapshot.Checkpoint.Cursor != generation.CheckpointCursor {
		return harnessCheckpoint{}, fmt.Errorf("snapshot checksum, version, generation, or cursor mismatch")
	}
	return snapshot.Checkpoint, nil
}

func (j *fileJournal) maybeCheckpointLocked(state *harnessState) error {
	options := j.options.normalized()
	if state == nil || !state.checkpointSafe() ||
		(j.tailBytes < options.CheckpointTailBytes && j.tailRecords < options.CheckpointTailRecords) {
		return nil
	}
	if state.cursor != j.cursor {
		return fmt.Errorf("checkpoint reducer cursor %d does not match journal cursor %d", state.cursor, j.cursor)
	}
	if err := j.ensureCommandRecordsForCheckpointLocked(); err != nil {
		return fmt.Errorf("checkpoint command idempotency fence: %w", err)
	}
	checkpoint, err := state.checkpoint()
	if err != nil {
		return err
	}
	generationNumber := j.manifestSequence + 1
	if generationNumber <= j.activeGeneration.Generation {
		generationNumber = j.activeGeneration.Generation + 1
	}
	snapshotPath := j.generationSnapshotPath(generationNumber)
	tailPath := j.generationTailPath(generationNumber)
	generation := fileJournalGeneration{
		Generation: generationNumber, CheckpointCursor: checkpoint.Cursor,
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
	manifestBody := fileJournalGenerationManifestBody{
		Version: fileJournalGenerationManifestVersion, KeySHA256: j.keySHA256,
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
	j.generationCandidates = []fileJournalGeneration{generation, previous}
	j.tailPath = tailPath
	j.tailBytes, j.tailRecords = 0, 0
	j.needsNewline = false
	j.lastTailHash = ""
	j.checkpoint = &checkpoint
	// Every command through the checkpoint cursor is now fenced by its direct
	// checksummed record. Keep only post-checkpoint commands in memory so a
	// long-lived binding cannot grow an unbounded historical command map.
	j.commandIndex = make(map[CommandID]CommandRecord)
	j.indexReady = false
	if err := j.runCheckpointHook(checkpointManifestDurable); err != nil {
		return err
	}
	if err := j.runCheckpointHook(checkpointActivated); err != nil {
		return err
	}
	if err := j.garbageCollectGenerationsLocked(generation, previous, sequence); err != nil {
		log.Printf("[agent-runtime] generation garbage collection deferred journal=%s error=%v", filepath.Base(j.path), err)
		return nil
	}
	return j.runCheckpointHook(checkpointGarbageCollected)
}

func (j *fileJournal) MaybeCheckpoint(ctx context.Context, state *harnessState) error {
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

func (j *fileJournal) runCheckpointHook(stage fileJournalCheckpointStage) error {
	if j.checkpointHook == nil {
		return nil
	}
	if err := j.checkpointHook(stage); err != nil {
		return fmt.Errorf("checkpoint interrupted after %s: %w", stage, err)
	}
	return nil
}

func (j *fileJournal) garbageCollectGenerationsLocked(active, previous fileJournalGeneration, sequence uint64) error {
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
		// The legacy base tail is no longer a fallback after two successful
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
func (j *fileJournal) ensureCommandRecordsForCheckpointLocked() error {
	return j.persistCommandRecordsLocked(j.commandIndex)
}
