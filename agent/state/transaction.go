package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/alfredxw/denova/agent/internal/localfs"
)

type transactionRecord struct {
	BaseRevision      string `json:"base_revision"`
	CandidateRevision string `json:"candidate_revision"`
	Stage             string `json:"stage"`
}

func (store *Store) apply(base, candidate Snapshot) (Result, error) {
	if base.Revision == candidate.Revision {
		return Result{Snapshot: base}, nil
	}
	if err := store.cacheSnapshot(base); err != nil {
		return Result{}, err
	}
	if err := store.cacheSnapshot(candidate); err != nil {
		return Result{}, err
	}
	transaction := transactionRecord{
		BaseRevision: base.Revision, CandidateRevision: candidate.Revision, Stage: "prepared",
	}
	marker := store.transactionPath()
	if err := atomicJSON(marker, transaction); err != nil {
		return Result{}, fmt.Errorf("prepare Agent state transaction: %w", err)
	}
	if err := writeSnapshot(store.root, candidate); err != nil {
		applyErr := fmt.Errorf("apply Agent state files: %w", err)
		if rollbackErr := store.rollbackPreparedTransaction(marker, base); rollbackErr != nil {
			return Result{}, errors.Join(applyErr, rollbackErr)
		}
		return Result{}, applyErr
	}
	transaction.Stage = "applied"
	if err := atomicJSON(marker, transaction); err != nil {
		confirmErr := fmt.Errorf("confirm Agent state transaction: %w", err)
		if rollbackErr := store.rollbackPreparedTransaction(marker, base); rollbackErr != nil {
			return Result{}, errors.Join(confirmErr, rollbackErr)
		}
		return Result{}, confirmErr
	}
	if err := removeDurable(marker); err != nil {
		// The candidate and the applied marker are both durable. Recovery can
		// safely retry marker cleanup; reporting a failed State mutation here
		// would invite callers to retry an operation that already committed.
		return Result{
			Snapshot: candidate, Changed: true,
			CleanupError: fmt.Errorf("finish Agent state transaction: %w", err),
		}, nil
	}
	return Result{Snapshot: candidate, Changed: true}, nil
}

func (store *Store) recoverTransaction() error {
	marker := store.transactionPath()
	var transaction transactionRecord
	if err := readJSON(marker, &transaction); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read Agent state recovery transaction: %w", err)
	}
	if !validRevision(transaction.BaseRevision) || !validRevision(transaction.CandidateRevision) {
		return errors.New("recover Agent state transaction: invalid revision")
	}
	if transaction.Stage != "prepared" && transaction.Stage != "applied" {
		return fmt.Errorf("recover Agent state transaction: invalid stage %q", transaction.Stage)
	}
	base, err := store.loadCachedSnapshot(transaction.BaseRevision)
	if err != nil {
		return fmt.Errorf("recover Agent state transaction base: %w", err)
	}
	if transaction.Stage == "applied" {
		current, err := scanFiles(store.root)
		if err == nil && snapshotFromFiles(current).Revision == transaction.CandidateRevision {
			return removeDurable(marker)
		}
	}
	return store.rollbackPreparedTransaction(marker, base)
}

// rollbackPreparedTransaction deliberately retains the recovery marker until
// the base snapshot is durable. A later Store operation can therefore retry a
// rollback that was interrupted or failed partway through.
func (store *Store) rollbackPreparedTransaction(marker string, base Snapshot) error {
	if err := writeSnapshot(store.root, base); err != nil {
		return fmt.Errorf("roll back Agent state transaction (recovery marker retained): %w", err)
	}
	if err := removeDurable(marker); err != nil {
		return fmt.Errorf("finish Agent state rollback: %w", err)
	}
	return nil
}

func (store *Store) transactionPath() string {
	return filepath.Join(store.private, "transaction.json")
}

func (store *Store) cacheSnapshot(snapshot Snapshot) error {
	root := filepath.Join(store.private, "snapshots", snapshot.Revision)
	if _, err := os.Stat(root); err == nil {
		_, loadErr := store.loadCachedSnapshot(snapshot.Revision)
		return loadErr
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	temporary := root + ".tmp"
	_ = os.RemoveAll(temporary)
	if err := copySnapshot(temporary, snapshot); err != nil {
		return err
	}
	if err := os.Rename(temporary, root); err != nil {
		if _, statErr := os.Stat(root); statErr == nil {
			_ = os.RemoveAll(temporary)
			_, loadErr := store.loadCachedSnapshot(snapshot.Revision)
			return loadErr
		}
		return err
	}
	return localfs.SyncDirectory(filepath.Dir(root))
}

func (store *Store) loadCachedSnapshot(revision string) (Snapshot, error) {
	revision = strings.TrimSpace(revision)
	if !validRevision(revision) {
		return Snapshot{}, errors.New("invalid cached Agent state revision")
	}
	root := filepath.Join(store.private, "snapshots", revision)
	files, err := scanFiles(root)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := snapshotFromFiles(files)
	if snapshot.Revision != revision {
		return Snapshot{}, fmt.Errorf("cached Agent state revision mismatch: expected=%s actual=%s", revision, snapshot.Revision)
	}
	return snapshot, nil
}
